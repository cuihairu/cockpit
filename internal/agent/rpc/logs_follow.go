package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 日志实时尾随（见 logs-design.md F1-F3）：logs.follow.start 起
// journalctl -f / docker logs -f 跟随进程，行经 sender 推回 server
// （proxy_data，proxyId="logs:<followId>"，与 terminal 前缀模式同构）；
// 终止统一经 closer 发 proxy_close。上限防打爆 WS 与浏览器。

// execCommandContext 可注入（测试覆盖 StdoutPipe 失败的防御分支）
var execCommandContext = exec.CommandContext

const (
	// logsFollowMaxBytes 单会话累计输出上限（到达即停）
	logsFollowMaxBytes = 4 << 20
	// logsFollowTimeout 会话超时（防僵死跟随进程）
	logsFollowTimeout = 10 * time.Minute
	// logsFollowMaxActive 同时活跃的跟随会话上限
	logsFollowMaxActive = 16
	// logsFollowLineMax 单行上限（bufio.Scanner 缓冲，与 query 行边界一致）
	logsFollowLineMax = 1 << 20
	// logsFollowProxyPrefix follow 数据的 proxyId 前缀
	logsFollowProxyPrefix = "logs:"
)

// LogsSender follow 数据回推（agent 装配时注入，包装 proxy_data 发送）
type LogsSender func(proxyID string, data []byte)

// LogsCloser follow 终止通知（包装 proxy_close 发送，reason 透传）
type LogsCloser func(proxyID string, reason string)

// logsFollowSession 一个跟随会话
type logsFollowSession struct {
	id      string
	cancel  context.CancelFunc
	closeFn func(reason string) // 幂等终止：kill 进程 + 通知 server 一次
	once    sync.Once
}

// LogsProvider 的 follow 扩展字段（见 NewLogsProvider 初始化）
type followState struct {
	mu      sync.Mutex
	sender  LogsSender
	closer  LogsCloser
	follows map[string]*logsFollowSession
}

// SetSender 注入数据回推函数（不注入则 follow.start 报错）
func (p *LogsProvider) SetSender(fn LogsSender) {
	p.fs.mu.Lock()
	defer p.fs.mu.Unlock()
	p.fs.sender = fn
}

// SetCloser 注入终止通知函数
func (p *LogsProvider) SetCloser(fn LogsCloser) {
	p.fs.mu.Lock()
	defer p.fs.mu.Unlock()
	p.fs.closer = fn
}

// logsFollowParams follow.start 参数（F1 平铺：{followId, type, source, tail, grep}；
// since 无意义——-f 天然从现在起，回填由 tail 承担，故不收）
type logsFollowParams struct {
	FollowID string `json:"followId"`
	Type     string `json:"type"`
	Source   string `json:"source"`
	Tail     int    `json:"tail"`
	Grep     string `json:"grep"`
}

// FollowStart 处理 logs.follow.start（F1/F2/F3）：校验 → 上限检查 →
// 起跟随进程 → goroutine 行扫描推送。start 返回成功即开始推数据。
func (p *LogsProvider) FollowStart(params map[string]interface{}) (interface{}, error) {
	p.fs.mu.Lock()
	sender, closer := p.fs.sender, p.fs.closer
	p.fs.mu.Unlock()
	if sender == nil || closer == nil {
		return nil, fmt.Errorf("log follow not available")
	}

	b, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("bad follow params: %w", err)
	}
	var fp logsFollowParams
	if err := json.Unmarshal(b, &fp); err != nil {
		return nil, fmt.Errorf("bad follow params: %w", err)
	}
	// 复用 query 校验（tail/grep/source 同规则）
	q := &LogsQuery{Type: fp.Type, Source: fp.Source, Tail: fp.Tail, Grep: fp.Grep}
	if err := q.validate(); err != nil {
		return nil, err
	}
	followID := fp.FollowID
	if followID == "" || len(followID) > 128 || strings.ContainsAny(followID, " \t\r\n") {
		return nil, fmt.Errorf("followId required")
	}

	p.fs.mu.Lock()
	if len(p.fs.follows) >= logsFollowMaxActive {
		p.fs.mu.Unlock()
		return nil, fmt.Errorf("too many active log follows (max %d)", logsFollowMaxActive)
	}
	// 同 followId 重复 start：覆盖旧会话（先杀旧进程防泄漏）。
	// closeFn 内部要拿 p.fs.mu，必须在锁外调（Mutex 不可重入）
	var old *logsFollowSession
	if prev, exists := p.fs.follows[followID]; exists {
		old = prev
	}
	p.fs.mu.Unlock()
	if old != nil {
		old.closeFn("replaced")
	}

	ctx, cancel := context.WithTimeout(context.Background(), logsFollowTimeout)
	cmd, outPipe, err := p.followCmdFn(ctx, q)
	if err != nil {
		cancel()
		return nil, err
	}

	sess := &logsFollowSession{id: followID, cancel: cancel}
	sess.closeFn = func(reason string) {
		sess.once.Do(func() {
			cancel() // kill 进程（CommandContext）
			p.fs.mu.Lock()
			// 只删自己：同 followId 被新会话覆盖后，旧会话的迟到 close 不误删新会话
			if p.fs.follows[followID] == sess {
				delete(p.fs.follows, followID)
			}
			p.fs.mu.Unlock()
			closer(logsFollowProxyPrefix+followID, reason)
		})
	}
	p.fs.mu.Lock()
	p.fs.follows[followID] = sess
	p.fs.mu.Unlock()

	go p.pumpFollow(sess, cmd, outPipe, q.Grep, sender)
	return map[string]interface{}{"followId": followID}, nil
}

// startFollowCmd 构造并启动跟随命令（F2）：journalctl -f / docker logs -f，
// 回填 tail 行由命令自带参数完成
func startFollowCmd(ctx context.Context, q *LogsQuery) (*exec.Cmd, *bufio.Reader, error) {
	var cmd *exec.Cmd
	if q.Type == "systemd" {
		cmd = execCommandContext(ctx, "journalctl",
			"-f", "-n", strconv.Itoa(q.Tail), "-o", "short-iso", "-q", "-u", q.Source)
	} else {
		cmd = execCommandContext(ctx, "docker",
			"logs", "-f", "--tail", strconv.Itoa(q.Tail), "--timestamps", q.Source)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = nil // 错误走进程退出 + close 通知，不混入数据流
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start %s follow: %w", q.Type, err)
	}
	return cmd, bufio.NewReaderSize(stdout, 64*1024), nil
}

// pumpFollow 行扫描 → grep 过滤 → 上限检查 → 推送；任何退出路径统一 closeFn
func (p *LogsProvider) pumpFollow(sess *logsFollowSession, cmd *exec.Cmd, out *bufio.Reader, grep string, sender LogsSender) {
	defer func() {
		_ = cmd.Wait()
	}()
	proxyID := logsFollowProxyPrefix + sess.id
	var total int
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), logsFollowLineMax)
	for scanner.Scan() {
		line := scanner.Text()
		if grep != "" && !strings.Contains(line, grep) {
			continue
		}
		total += len(line) + 1
		if total > logsFollowMaxBytes {
			sess.closeFn("limit")
			return
		}
		sender(proxyID, []byte(line+"\n"))
	}
	// 进程退出（容器停止 / journal 结束）/ 超时 / stop kill 都落到这里；
	// stop 路径 closeFn 已发过 close（once 幂等），其余路径在此补发
	sess.closeFn("exited")
}

// FollowStop 处理 logs.follow.stop：杀进程 + 通知（幂等）
func (p *LogsProvider) FollowStop(params map[string]interface{}) (interface{}, error) {
	followID, _ := params["followId"].(string)
	if followID == "" {
		return nil, fmt.Errorf("followId required")
	}
	p.fs.mu.Lock()
	sess := p.fs.follows[followID]
	p.fs.mu.Unlock()
	if sess != nil {
		sess.closeFn("stopped")
	}
	// 不存在的 followId 幂等成功（stop 与进程自身退出竞态时先到）
	return map[string]interface{}{"stopped": true}, nil
}
