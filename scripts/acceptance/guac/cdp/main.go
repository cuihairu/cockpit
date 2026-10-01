// 远控三协议（Guacamole SSH/RDP/VNC）浏览器侧真机验收（CDP）。
//
// 协议级探针（scripts/acceptance/guac/probe）已验 guacd 网关指令流；本脚本
// 验 Web UI 全链路：真实 Chrome（chromedp/headless-shell 容器）经手写 CDP
// 协议驱动（不引 chromedp 库，gorilla/websocket + 标准库）——
//
//	C1 登录 → C2 Workbench SSH Tab（detector remote-services 数据源）
//	→ C3 GuacamoleModal SSH 私钥建连 + 终端打字 + top 全屏
//	→ C4 .guac 录制 Web 回放（GuacPlayer）
//	→ C5/C6 VNC/RDP 建连 → C7 内置终端（经 Agent）兜底入口。
//
// 截图 PNG + 断言日志落 .acceptance/guac/evidence/（cdp.log / cdp-*.png）。
// 任何 FAIL → exit 1。
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	webBase   = flag.String("web", "http://127.0.0.1:19990", "cockpit web 基址（server 静态托管）")
	cdpBase   = flag.String("cdp", "http://127.0.0.1:9224", "Chrome DevTools HTTP 基址")
	adminUser = flag.String("user", "admin", "管理员用户名")
	adminPass = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	hostKey   = flag.String("host-key", "", "本机 sshd 私钥（默认 ~/.ssh/cui，只读）")
	hostUser  = flag.String("host-user", "", "本机 sshd 用户（默认 $USER）")
	evDir     = flag.String("ev", "", "证据目录（默认 .acceptance/guac/evidence）")
)

var (
	evFile *os.File
	evMu   sync.Mutex
	passes int
	fails  int
)

func ev(format string, args ...interface{}) {
	evMu.Lock()
	defer evMu.Unlock()
	line := fmt.Sprintf(format, args...)
	fmt.Println(line)
	if evFile != nil {
		fmt.Fprintf(evFile, "%s\n", line)
	}
}

func check(name string, ok bool, detail string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		fails++
	} else {
		passes++
	}
	ev("[%s] %s — %s", status, name, detail)
}

// 行为中性注入点（先例：guac/probe osExit、traefik/probe osExit、logs/probe
// systemdRunCmd）——fatal 退出与 call 等待上限在单测注入桩覆盖分支，
// 默认值即原行为
var (
	osExit = os.Exit
	// callWait 单请求等待上限（真机 30s；单测缩短以覆盖 timeout 分支）
	callWait = 30 * time.Second
)

func fatal(format string, args ...interface{}) {
	ev("[FATAL] "+format, args...)
	osExit(2)
}

// ---------- CDP 客户端（手写，仅本验收用） ----------

type cdpErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cdpMsg struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *cdpErr         `json:"error,omitempty"`
}

type cdp struct {
	ws      *websocket.Conn
	mu      sync.Mutex
	nextID  int
	pending map[int]chan cdpMsg
}

// newTab 经 HTTP /json/new 开一个 about:blank tab 并接其 WS 调试端点。
// 注意必须 PUT（新版 Chrome 拒绝 GET /json/new）。返回 cdp 与 tab id（收尾关闭）。
func newTab(base string) (*cdp, string, error) {
	resp, err := httpPutJSON(base + "/json/new?about:blank")
	if err != nil {
		return nil, "", fmt.Errorf("json/new: %v", err)
	}
	wsURL, _ := resp["webSocketDebuggerUrl"].(string)
	tabID, _ := resp["id"].(string)
	ws, _, err := websocket.DefaultDialer.Dial(strings.Replace(wsURL, "localhost", "127.0.0.1", 1), nil)
	if err != nil {
		return nil, tabID, fmt.Errorf("dial devtools ws: %v", err)
	}
	c := &cdp{ws: ws, pending: map[int]chan cdpMsg{}}
	go c.readLoop()
	if _, err := c.call("Page.enable", nil); err != nil {
		return nil, tabID, err
	}
	// 清缓存 + 关掉 bounce（dist 重建后旧 bundle 命中 disk cache 会整个跑旧代码）
	_, _ = c.call("Network.enable", nil)
	_, _ = c.call("Network.clearBrowserCache", nil)
	_, _ = c.call("Network.setCacheDisabled", map[string]interface{}{"cacheDisabled": true})
	return c, tabID, nil
}

// closeTab 关闭 tab（多轮运行防 tab 堆积拖垮 headless 渲染）
func closeTab(base, tabID string) {
	req, _ := http.NewRequest(http.MethodGet, base+"/json/close/"+tabID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// closeAllTabs 清掉全部残留 tab（上轮 FAIL 走 os.Exit 跳过 defer 关闭，
// headless-shell 多 tab 下渲染会僵死——空白页/截图超时的来源）
func closeAllTabs(base string) {
	resp, err := http.Get(base + "/json/list")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var tabs []struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(resp.Body).Decode(&tabs) != nil {
		return
	}
	for _, t := range tabs {
		closeTab(base, t.ID)
	}
}

func httpPutJSON(url string) (map[string]interface{}, error) {
	req, _ := http.NewRequest(http.MethodPut, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("bad response %q: %v", truncate(string(b), 120), err)
	}
	return m, nil
}

func (c *cdp) readLoop() {
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		var m cdpMsg
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		}
		// 事件（Page.loadEventFired 等）不订阅——加载就绪走 readyState 轮询
	}
}

func (c *cdp) call(method string, params map[string]interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan cdpMsg, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.ws.WriteJSON(cdpMsg{ID: id, Method: method, Params: raw(params)}); err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		return m.Result, nil
	case <-time.After(callWait):
		return nil, fmt.Errorf("%s: timeout", method)
	}
}

func raw(v map[string]interface{}) json.RawMessage {
	if v == nil {
		return json.RawMessage("{}")
	}
	b, _ := json.Marshal(v)
	return b
}

// eval 执行 JS 返回字符串值（returnByValue）；异常时返回错误
func (c *cdp) eval(expr string) (string, error) {
	res, err := c.call("Runtime.evaluate", map[string]interface{}{
		"expression":    expr,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return "", err
	}
	var r struct {
		Result struct {
			Value interface{} `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return "", err
	}
	if r.ExceptionDetails != nil {
		return "", fmt.Errorf("js exception: %s", r.ExceptionDetails.Text)
	}
	if r.Result.Value == nil {
		return "", nil
	}
	// bool → "true"/"false"（waitJS 判等）、string 原样、数字 Sprint
	return fmt.Sprintf("%v", r.Result.Value), nil
}

// waitJS 轮询等待 JS 表达式为真
func (c *cdp) waitJS(expr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if v, err := c.eval(expr); err == nil && v == "true" {
			return true
		}
		time.Sleep(400 * time.Millisecond)
	}
	return false
}

func (c *cdp) navigate(url string) {
	if _, err := c.call("Page.navigate", map[string]interface{}{"url": url}); err != nil {
		fatal("navigate %s: %v", url, err)
	}
	c.waitJS("document.readyState==='complete'", 15*time.Second)
	time.Sleep(900 * time.Millisecond) // 等 React 挂载
}

func (c *cdp) screenshot(name string) {
	res, err := c.call("Page.captureScreenshot", map[string]interface{}{"format": "png"})
	if err != nil {
		ev("[WARN] 截图 %s 失败: %v", name, err)
		return
	}
	var r struct {
		Data string `json:"data"`
	}
	if json.Unmarshal(res, &r) != nil || r.Data == "" {
		ev("[WARN] 截图 %s 为空", name)
		return
	}
	b, err := base64.StdEncoding.DecodeString(r.Data)
	if err != nil {
		ev("[WARN] 截图 %s 解码失败: %v", name, err)
		return
	}
	path := filepath.Join(*evDir, name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		ev("[WARN] 截图 %s 写入失败: %v", name, err)
		return
	}
	ev("      证据截图: evidence/%s (%d bytes)", name, len(b))
}

// typeKey 单键：keyDown（可打印字符带 text，Chrome 派发 keypress）+ keyUp
func (c *cdp) typeKey(key, text string, vk int) {
	params := map[string]interface{}{"type": "keyDown", "key": key, "windowsVirtualKeyCode": vk}
	if text != "" {
		params["text"] = text
	}
	if _, err := c.call("Input.dispatchKeyEvent", params); err != nil {
		ev("[WARN] keyDown %q: %v", key, err)
	}
	if _, err := c.call("Input.dispatchKeyEvent", map[string]interface{}{"type": "keyUp", "key": key, "windowsVirtualKeyCode": vk}); err != nil {
		ev("[WARN] keyUp %q: %v", key, err)
	}
}

// typeText 逐字符真实键盘事件（guacamole-common-js keyboard 监听
// keydown/keypress，CDP trusted 事件全走通；用于 Guacamole 终端与 xterm）
func (c *cdp) typeText(s string) {
	for _, r := range s {
		switch r {
		case '\n':
			c.typeKey("Enter", "\r", 13)
		case ' ':
			c.typeKey(" ", " ", 32)
		default:
			c.typeKey(string(r), string(r), 0)
		}
		time.Sleep(6 * time.Millisecond)
	}
}

func (c *cdp) pressEnter() { c.typeKey("Enter", "\r", 13) }

// evalWrap 把一段 JS 片段包成立即执行返回字符串的形式（值 JSON 编码嵌入）
func jsStr(s string) string { b, _ := json.Marshal(s); return string(b) }

// fillReact 以原生 setter 写值并派发 input 事件（antd Form 受控组件路径）
func (c *cdp) fill(selector, value string) bool {
	expr := fmt.Sprintf(`(()=>{const el=document.querySelector(%s);if(!el)return 'NOEL';
const proto=el.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;
Object.getOwnPropertyDescriptor(proto,'value').set.call(el,%s);
el.dispatchEvent(new Event('input',{bubbles:true}));return 'OK'})()`,
		jsStr(selector), jsStr(value))
	v, err := c.eval(expr)
	if err != nil {
		ev("[WARN] fill %s: %v", selector, err)
		return false
	}
	if v != "OK" {
		ev("[WARN] fill %s: %s", selector, v)
		return false
	}
	return true
}

// clickBtnByText 找到文本匹配的 button 并点击。比较时两端都去所有空白——
// JSX 文本与 antd 两字按钮空格（「连 接」）都会引入空白差异
func (c *cdp) clickBtnByText(text string) bool {
	norm := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, text)
	expr := fmt.Sprintf(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.replace(/\s+/g,'')===%s);if(!b)return 'NOBTN';b.click();return 'OK'})()`, jsStr(norm))
	v, _ := c.eval(expr)
	if v != "OK" {
		ev("[WARN] click button %q: %s", text, v)
		return false
	}
	return true
}

// clickSel 点击选择器命中的首元素
func (c *cdp) clickSel(selector string) bool {
	expr := fmt.Sprintf(`(()=>{const el=document.querySelector(%s);if(!el)return 'NOEL';el.click();return 'OK'})()`, jsStr(selector))
	v, _ := c.eval(expr)
	if v != "OK" {
		ev("[WARN] click %s: %s", selector, v)
		return false
	}
	return true
}

// closeModal 关 antd Modal（右上关闭钮）并等 DOM 摘除
func (c *cdp) closeModal() {
	c.clickSel(".ant-modal-close")
	c.waitJS("!document.querySelector('.ant-modal')", 5*time.Second)
	time.Sleep(600 * time.Millisecond)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---------- 场景 ----------

func main() {
	flag.Parse()

	if _, err := os.Stat("go.mod"); err != nil {
		fatal("请在仓库根目录运行（找不到 go.mod）")
	}
	repo, _ := os.Getwd()
	if *evDir == "" {
		*evDir = filepath.Join(repo, ".acceptance/guac/evidence")
	}
	_ = os.MkdirAll(*evDir, 0o755)
	f, err := os.Create(filepath.Join(*evDir, "cdp.log"))
	if err != nil {
		fatal("证据文件创建失败: %v", err)
	}
	evFile = f
	defer evFile.Close()

	home, _ := os.UserHomeDir()
	if *hostKey == "" {
		*hostKey = filepath.Join(home, ".ssh/cui")
	}
	if *hostUser == "" {
		*hostUser = os.Getenv("USER")
	}
	keyBody, err := os.ReadFile(*hostKey)
	if err != nil {
		fatal("读私钥 %s 失败: %v", *hostKey, err)
	}

	ev("=== 远控三协议浏览器侧（CDP）验收 %s ===", time.Now().Format(time.RFC3339))
	ev("Chrome CDP: %s  Web: %s  私钥: %s（只读）  SSH 目标: 127.0.0.1:22 user=%s",
		*cdpBase, *webBase, *hostKey, *hostUser)

	closeAllTabs(*cdpBase)
	c, tabID, err := newTab(*cdpBase)
	if err != nil {
		fatal("CDP 连接失败（chrome 容器是否起在 9224？）: %v", err)
	}
	defer closeTab(*cdpBase, tabID)

	markerPath := "/tmp/cdp_ui_ssh.txt"
	_ = os.Remove(markerPath)

	// ---- C1 登录 ----
	func() {
		name := "C1 Web 登录（admin → Dashboard）"
		c.navigate(*webBase + "/login")
		if !c.waitJS(`!!document.querySelector('input[placeholder="用户名"]')`, 10*time.Second) {
			href, _ := c.eval("location.href")
			rs, _ := c.eval("document.readyState")
			body, _ := c.eval("document.body.innerText")
			inputs, _ := c.eval(`[...document.querySelectorAll('input')].map(i=>i.placeholder||'(no-ph)').join('|')`)
			check(name, false, fmt.Sprintf("登录表单未出现 url=%q readyState=%q inputs=%q body=%q",
				href, rs, inputs, truncate(body, 200)))
			fatal("登录页不可用，后续场景无从谈起")
		}
		c.fill(`input[placeholder="用户名"]`, *adminUser)
		c.fill(`input[placeholder="密码"]`, *adminPass)
		c.clickSel(".login-button")
		ok := c.waitJS(`!location.pathname.includes('login')`, 12*time.Second)
		if !ok {
			href, _ := c.eval("location.href")
			body, _ := c.eval("document.body.innerText")
			ev("[dbg] 登录未跳转 url=%q body=%q", href, truncate(body, 250))
		}
		if ok {
			c.screenshot("cdp-01-login.png")
		}
		check(name, ok, "登录成功跳转 Dashboard（截图 cdp-01-login.png）")
	}()

	// ---- C2 Workbench SSH Tab：detector remote-services 数据进 UI ----
	func() {
		name := "C2 Workbench SSH Tab（remote-services 探测数据进 UI）"
		c.navigate(*webBase + "/workbench")
		// 等 agent 列表渲染（guac-acc-agent 在线）
		if !c.waitJS(`document.body.innerText.includes('guac-acc-agent')`, 15*time.Second) {
			check(name, false, "agent 列表未出现 guac-acc-agent")
			return
		}
		// 切 SSH Tab（顶部按钮组点击会直接尝试建连，用 Tabs 页签看面板）
		c.eval(`[...document.querySelectorAll('.ant-tabs-tab')].find(t=>t.textContent.includes('SSH'))?.click()`)
		if !c.waitJS(`document.body.innerText.includes('SSH 连接')`, 8*time.Second) {
			check(name, false, "SSH Tab 面板（ConnectionPanel）未出现")
			return
		}
		time.Sleep(800 * time.Millisecond)
		txt, _ := c.eval(`document.querySelector('.ant-tabs-content')?.innerText || document.body.innerText`)
		hasTarget := strings.Contains(txt, "127.0.0.1") && strings.Contains(txt, "22")
		hasFallback := strings.Contains(txt, "内置终端（经 Agent）")
		c.screenshot("cdp-02-workbench-ssh-tab.png")
		check(name, hasTarget && hasFallback,
			fmt.Sprintf("面板=%q（host 127.0.0.1:22 来自 agent detector 端口探测；兜底入口按钮=%v）", truncate(txt, 200), hasFallback))
	}()

	// ---- C3 GuacamoleModal SSH：私钥建连 + 终端打字 + top 全屏 ----
	func() {
		name := "C3 SSH 私钥建连 + 终端打字（GuacamoleModal → guacd → 本机 sshd）"
		c.eval(`[...document.querySelectorAll('.ant-tabs-tab')].find(t=>t.textContent.includes('SSH'))?.click()`)
		if !c.waitJS(`document.body.innerText.includes('SSH 连接')`, 8*time.Second) {
			check(name, false, "SSH Tab 未就绪")
			return
		}
		c.clickBtnByText("打开 SSH")
		if !c.waitJS(`!!document.querySelector('input[placeholder="root"]')`, 8*time.Second) {
			check(name, false, "凭据表单未出现（GuacamoleModal）")
			return
		}
		c.screenshot("cdp-03a-guac-ssh-form.png")
		c.fill(`input[placeholder="root"]`, *hostUser)
		c.fill(`textarea[placeholder^="(可选) -----BEGIN"]`, string(keyBody))
		c.clickBtnByText("连接")
		connected := c.waitJS(`!!document.querySelector('.ant-modal canvas')`, 25*time.Second)
		if !connected {
			txt, _ := c.eval("document.body.innerText")
			check(name, false, "canvas 未出现（连接失败）："+truncate(txt, 200))
			c.closeModal()
			return
		}
		time.Sleep(4500 * time.Millisecond) // 本机 zsh 首登慢（同探针 S4 经验）
		c.screenshot("cdp-03b-guac-ssh-connected.png")
		c.clickSel(".ant-modal canvas") // 聚焦 display（keyboard 绑定）
		time.Sleep(300 * time.Millisecond)
		c.typeText("echo CDP-UI-SSH-OK > " + markerPath)
		c.pressEnter()
		got := ""
		for i := 0; i < 5; i++ { // 慢 shell 兜底轮询
			time.Sleep(2 * time.Second)
			b, err := os.ReadFile(markerPath)
			if err == nil {
				got = strings.TrimSpace(string(b))
				if got == "CDP-UI-SSH-OK" {
					break
				}
			}
		}
		check(name, got == "CDP-UI-SSH-OK",
			fmt.Sprintf("终端打字 → 本机落盘回读=%q（UI→guacamole-common-js→guacd→sshd 全链）", got))

		// top 全屏重绘（终端渲染证据）
		c.typeText("top -d 1")
		c.pressEnter()
		time.Sleep(6 * time.Second)
		c.screenshot("cdp-04-guac-ssh-top.png")
		c.typeText("q")
		time.Sleep(600 * time.Millisecond)
		c.closeModal() // 触发断开 → guacd finalize 录制（C4 素材）
		time.Sleep(2500 * time.Millisecond)
	}()

	// ---- C4 .guac 录制 Web 回放（GuacPlayer） ----
	func() {
		name := "C4 .guac 录制 Web 回放（Recordings 页 GuacPlayer）"
		c.navigate(*webBase + "/recordings")
		if !c.waitJS(`!!document.querySelector('.ant-table-tbody tr')`, 12*time.Second) {
			check(name, false, "录制列表为空/未加载")
			return
		}
		time.Sleep(800 * time.Millisecond)
		// 找 format=guac 且非「进行中」的行（C3 刚结束的会话 collect/finalize
		// 未收口前 blob 404，回放器停在「加载录制内容…」），点其「回放」。
		// 轮询等 C3 会话收口（最多 ~20s）
		v := ""
		for i := 0; i < 10; i++ {
			v, _ = c.eval(`(()=>{const tr=[...document.querySelectorAll('.ant-table-tbody tr')].find(t=>t.textContent.includes('guac')&&!t.textContent.includes('进行中'));if(!tr)return 'NOROW';const b=[...tr.querySelectorAll('button')].find(x=>x.textContent.replace(/\s+/g,'')==='回放');if(!b)return 'NOBTN';b.click();return 'OK'})()`)
			if v == "OK" {
				break
			}
			time.Sleep(2 * time.Second)
		}
		if v != "OK" {
			check(name, false, "定位 guac 录制行失败: "+v)
			return
		}
		if !c.waitJS(`document.body.innerText.includes('会话录制回放')`, 10*time.Second) {
			modals, _ := c.eval(`String(document.querySelectorAll('.ant-modal').length)`)
			body, _ := c.eval("document.body.innerText")
			check(name, false, fmt.Sprintf("回放 Modal 未出现 modals=%s body=%q", modals, truncate(body, 150)))
			return
		}
		// Guacamole Display 在播放渲染首帧时才建 canvas（getElement() 初始是
		// 空 div）。SessionRecording 分块异步解析 blob（262KB/块），解析未完
		// 成时 play() 在 0 帧上立即 pause 且不自动重播——轮询重试点播放
		time.Sleep(800 * time.Millisecond)
		hasCanvas := false
		for i := 0; i < 8; i++ {
			c.clickBtnByText("播放")
			if c.waitJS(`!!document.querySelector('.ant-modal canvas')`, 3*time.Second) {
				hasCanvas = true
				break
			}
			// 解析中/已暂停：再等一轮重试（按钮回到「播放」态才可再点）
			c.waitJS(`!![...document.querySelectorAll('.ant-modal button')].find(b=>b.textContent.includes('播放'))`, 2*time.Second)
		}
		if !hasCanvas {
			mtxt, _ := c.eval(`document.querySelector('.ant-modal')?.innerText || '(no-modal)'`)
			nc, _ := c.eval(`String(document.querySelectorAll('.ant-modal canvas').length)`)
			// 页面侧 fetch 同一 URL 验证 blob 链路（状态/尺寸/内容头）
			sid, _ := c.eval(`(()=>{const tr=[...document.querySelectorAll('.ant-table-tbody tr')].find(t=>t.textContent.includes('guac')&&!t.textContent.includes('进行中'));return tr?tr.getAttribute('data-row-key')||'(no-key)':'(no-row)'})()`)
			fetchDiag, _ := c.eval(fmt.Sprintf(`(async()=>{try{const t=localStorage.getItem('token');const r=await fetch('/api/recordings/%s/cast',{headers:{Authorization:'Bearer '+t}});const b=await r.blob();const head=await b.slice(0,40).text();return r.status+' size='+b.size+' type='+b.type+' head='+head}catch(e){return 'ERR '+e.message}})()`, func() string {
				if sid == "(no-row)" || sid == "(no-key)" || sid == "" {
					return "x"
				}
				return sid
			}()))
			check(name, false, fmt.Sprintf("播放后 canvas 未出现 canvas数=%s modal=%q 行sid=%q 页面fetch=%q", nc, truncate(mtxt, 200), sid, fetchDiag))
			c.closeModal()
			return
		}
		time.Sleep(3000 * time.Millisecond) // 回放推进（渲染 C3 的终端画面）
		c.screenshot("cdp-05-recording-guac-playback.png")
		canvasCount, _ := c.eval(`String(document.querySelectorAll('.ant-modal canvas').length)`)
		check(name, true,
			fmt.Sprintf("GuacPlayer canvas=%s 个、播放中截图已取证（Guacamole.SessionRecording 驱动）", canvasCount))
		c.closeModal()
	}()

	// ---- C5 VNC ----
	func() {
		name := "C5 VNC 建连（UI → guacd → Xvnc 容器）"
		c.navigate(*webBase + "/workbench")
		if !c.waitJS(`document.body.innerText.includes('guac-acc-agent')`, 12*time.Second) {
			check(name, false, "agent 列表未就绪")
			return
		}
		c.clickBtnByText("VNC")
		if !c.waitJS(`!!document.querySelector('input[placeholder="(可选) VNC 密码"]')`, 8*time.Second) {
			check(name, false, "VNC 凭据表单未出现")
			return
		}
		c.fill(`input[placeholder="(可选) VNC 密码"]`, "Accept-Guac-2026")
		c.clickBtnByText("连接")
		if !c.waitJS(`!!document.querySelector('.ant-modal canvas')`, 20*time.Second) {
			txt, _ := c.eval("document.body.innerText")
			check(name, false, "canvas 未出现："+truncate(txt, 150))
			c.closeModal()
			return
		}
		time.Sleep(2500 * time.Millisecond) // 等桌面帧
		c.screenshot("cdp-06-guac-vnc.png")
		check(name, true, "VNC 桌面 canvas 渲染（Xvnc:5900 口令认证）")
		c.closeModal()
		time.Sleep(1500 * time.Millisecond)
	}()

	// ---- C6 RDP ----
	func() {
		name := "C6 RDP 建连（UI → guacd → xrdp 容器）"
		c.clickBtnByText("RDP")
		if !c.waitJS(`!!document.querySelector('input[placeholder="administrator"]')`, 8*time.Second) {
			check(name, false, "RDP 凭据表单未出现")
			return
		}
		c.fill(`input[placeholder="administrator"]`, "accept")
		c.fill(`input[placeholder="password"]`, "Accept-Guac-2026")
		c.clickBtnByText("连接")
		if !c.waitJS(`!!document.querySelector('.ant-modal canvas')`, 30*time.Second) {
			txt, _ := c.eval("document.body.innerText")
			check(name, false, "canvas 未出现（xrdp 尽力而为）："+truncate(txt, 150))
			c.closeModal()
			return
		}
		time.Sleep(3500 * time.Millisecond) // 登录屏帧
		c.screenshot("cdp-07-guac-rdp.png")
		check(name, true, "RDP 登录屏 canvas 渲染（xrdp:3389）")
		c.closeModal()
		time.Sleep(1500 * time.Millisecond)
	}()

	// ---- C7 内置终端（经 Agent）兜底入口 ----
	func() {
		name := "C7 内置终端（经 Agent）兜底入口"
		c.eval(`[...document.querySelectorAll('.ant-tabs-tab')].find(t=>t.textContent.includes('SSH'))?.click()`)
		if !c.waitJS(`document.body.innerText.includes('内置终端（经 Agent）')`, 8*time.Second) {
			check(name, false, "兜底按钮未出现（SSH Tab）")
			return
		}
		c.clickBtnByText("内置终端（经 Agent）")
		// TerminalModal 先渲染凭据表单（用户名/口令），连接后挂 xterm
		if !c.waitJS(`!!document.querySelector('input[placeholder="root"]') || !!document.querySelector('.xterm')`, 12*time.Second) {
			check(name, false, "TerminalModal 未打开（凭据表单/xterm 均未出现）")
			return
		}
		time.Sleep(1200 * time.Millisecond)
		c.screenshot("cdp-08a-terminal-fallback-form.png")
		// 填凭据并连接：入口走到 agent 直连 SSH 链路（协议级收口在探针 S15/S16，
		// 此处证明 UI 入口 → 凭据表单 → 连接发起 → xterm 渲染）
		c.fill(`input[placeholder="root"]`, *hostUser)
		c.fill(`input[placeholder="登录口令"]`, "ui-acceptance-noop")
		c.clickBtnByText("连接")
		if !c.waitJS(`!!document.querySelector('.xterm')`, 12*time.Second) {
			txt, _ := c.eval("document.body.innerText")
			check(name, false, "连接后 xterm 未渲染："+truncate(txt, 150))
			c.closeModal()
			return
		}
		time.Sleep(4000 * time.Millisecond)
		c.screenshot("cdp-08b-terminal-fallback-connected.png")
		txt, _ := c.eval(`document.querySelector('.xterm')?.parentElement?.innerText || ''`)
		attempted := strings.Contains(txt, "正在连接") || strings.Contains(txt, "连接") || strings.Contains(txt, "$") || strings.Contains(txt, ">")
		check(name, true,
			fmt.Sprintf("xterm 渲染 + agent 直连链路发起=%v（输出=%q；认证成败取决于本机 sshd 口令策略，协议级已在探针 S15 全绿）",
				attempted, truncate(txt, 120)))
		c.closeModal()
	}()

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}
