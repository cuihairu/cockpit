#!/usr/bin/env node
/**
 * Cockpit 防火墙 M1 浏览器侧验收探针（docs/guide/firewall-design.md）。
 *
 * 真实 Chrome（chromedp/headless-shell 容器，CDP 9224）经 Node 内置
 * WebSocket/fetch 手写 CDP 客户端驱动（Node ≥22 全局 WebSocket；零依赖，
 * 不引 chromedp/puppeteer，同 guac/cdp 手写纪律）。踩坑同 guac/cdp：
 *   - /json/new 必须 PUT；截图/Page 域先 enable 无需——直接 capture
 *   - dist 重建后旧 bundle 命中 disk cache → clearBrowserCache + setCacheDisabled
 *   - antd 两汉字按钮自动插空格 → 文本匹配去空白
 *   - os.Exit 跳 defer → 开场 closeAllTabs 清残留 tab（多 tab 渲染僵死）
 *
 * 场景（-phase empty 起步、-phase full 收尾，由 probe-web.sh 分两段调用——
 * 空态需要「仅有无工具主机在线」，先于防火墙 agent 启动）：
 *   W1 登录（admin → Dashboard）
 *   W2 空态：仅有无防火墙工具主机 → 「暂无带防火墙工具的在线主机」
 *   W3 总览表：legacy 行（后端 iptables+legacy、默认策略 input accept）、
 *      非 root 行（不可读）、超大规则集行（部分读取 + 页顶 4MB 截断告警）
 *   W4 legacy 面板：展开 → 链/规则明细 + cockpit 名下规则标注
 *   W5 非 root 面板：展开 → 「防火墙规则集不可读」说明态
 *   W6 超大规则集面板：展开 → meta-only 错误说明（而非误报「规则集为空」）
 *
 * 证据：-ev 目录下 fw-*.png 截图；断言日志随 stdout 交 bash 落盘。
 * 退出码：0 全过；1 任一 FAIL。
 */
'use strict'

const args = {}
for (let i = 2; i < process.argv.length; i++) {
  const a = process.argv[i]
  if (a.startsWith('--')) {
    if (a.includes('=')) {
      const [k, ...v] = a.slice(2).split('=')
      args[k] = v.join('=')
    } else {
      args[a.slice(2)] = process.argv[++i]
    }
  }
}

const webBase = args.web || 'http://127.0.0.1:19999'
const cdpBase = args.cdp || 'http://127.0.0.1:9225'
const adminUser = args.user || 'admin'
const adminPass = args.pass || ''
const phase = args.phase || 'full'
const evDir = args.ev || '.acceptance/firewall'
const hosts = {}
for (const pair of (args.hosts || '').split(',')) {
  const [k, v] = pair.split('=')
  if (k && v) hosts[k] = v
}

let passes = 0
let fails = 0
function check(name, ok, detail) {
  const status = ok ? 'PASS' : 'FAIL'
  if (ok) passes++
  else fails++
  console.log(`[${status}] ${name} — ${detail}`)
}
function fatal(msg) {
  console.error(`[FATAL] ${msg}`)
  cleanup()
  process.exit(1)
}

// ---------- CDP 客户端（手写，同 guac/cdp 形状） ----------

let ws = null
let nextID = 0
const pending = new Map()

function putJSON(url) {
  return fetch(url, { method: 'PUT' }).then((r) => r.json())
}

async function connect(url) {
  return new Promise((resolve, reject) => {
    ws = new WebSocket(url.replace('localhost', '127.0.0.1'))
    const timer = setTimeout(() => reject(new Error('ws dial timeout')), 10000)
    ws.onopen = () => {
      clearTimeout(timer)
      resolve()
    }
    ws.onerror = (e) => {
      clearTimeout(timer)
      reject(new Error('ws error'))
    }
    ws.onmessage = (ev) => {
      let m
      try {
        m = JSON.parse(ev.data)
      } catch {
        return
      }
      if (m.id && pending.has(m.id)) {
        const { resolve: res, reject: rej } = pending.get(m.id)
        pending.delete(m.id)
        if (m.error) rej(new Error(`${m.error.message}`))
        else res(m.result)
      }
      // 事件不订阅——加载就绪走 readyState 轮询
    }
  })
}

function call(method, params) {
  return new Promise((resolve, reject) => {
    const id = ++nextID
    pending.set(id, { resolve, reject })
    const timer = setTimeout(() => {
      if (pending.has(id)) {
        pending.delete(id)
        reject(new Error(`${method}: timeout`))
      }
    }, 30000)
    ws.send(
      JSON.stringify({ id, method, params: params || {} }),
    )
    // 包一层：成功/失败都清 timer
    pending.set(id, {
      resolve: (v) => {
        clearTimeout(timer)
        resolve(v)
      },
      reject: (e) => {
        clearTimeout(timer)
        reject(e)
      },
    })
  })
}

async function evalJS(expr) {
  const res = await call('Runtime.evaluate', {
    expression: expr,
    returnByValue: true,
    awaitPromise: true,
  })
  if (res.exceptionDetails) throw new Error('js exception: ' + res.exceptionDetails.text)
  const v = res.result && res.result.value
  if (v === undefined || v === null) return ''
  return typeof v === 'string' ? v : String(v)
}

async function waitJS(expr, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      if ((await evalJS(expr)) === 'true') return true
    } catch {
      // 页面跳转途中 evaluate 可能抛错——轮询容忍
    }
    await new Promise((r) => setTimeout(r, 400))
  }
  return false
}

async function navigate(url) {
  await call('Page.navigate', { url })
  await waitJS("document.readyState==='complete'", 15000)
  await new Promise((r) => setTimeout(r, 900)) // 等 React 挂载
}

async function screenshot(name) {
  try {
    const res = await call('Page.captureScreenshot', { format: 'png' })
    if (!res || !res.data) return
    require('fs').writeFileSync(
      require('path').join(evDir, name),
      Buffer.from(res.data, 'base64'),
    )
    console.log(`      证据截图: ${evDir}/${name}`)
  } catch (e) {
    console.log(`[WARN] 截图 ${name} 失败: ${e.message}`)
  }
}

function jsStr(s) {
  return JSON.stringify(s)
}

// fillReact 原生 setter 写值 + input 事件（antd Form 受控组件路径，guac/cdp 同款）
async function fill(selector, value) {
  const expr = `(()=>{const el=document.querySelector(${jsStr(selector)});if(!el)return 'NOEL';
const proto=el.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;
Object.getOwnPropertyDescriptor(proto,'value').set.call(el,${jsStr(value)});
el.dispatchEvent(new Event('input',{bubbles:true}));return 'OK'})()`
  const v = await evalJS(expr)
  return v === 'OK'
}

// clickHeader 点含指定文本的 Collapse 面板头（面板子项懒挂载，展开才能断言）
async function clickHeader(text) {
  const expr = `(()=>{const h=[...document.querySelectorAll('.ant-collapse-header')].find(h=>h.textContent.includes(${jsStr(text)}));if(!h)return 'NOHDR';h.click();return 'OK'})()`
  const v = await evalJS(expr)
  return v === 'OK'
}

function bodyText() {
  return evalJS('document.body.innerText')
}

async function waitBodyIncludes(text, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      if ((await bodyText()).includes(text)) return true
    } catch {
      // 轮询容忍
    }
    await new Promise((r) => setTimeout(r, 400))
  }
  return false
}

// ---------- 场景 ----------

async function login() {
  await navigate(webBase + '/login')
  if (
    !(await waitJS(`!!document.querySelector('input[placeholder="用户名"]')`, 10000))
  ) {
    const inputs = await evalJS(
      `[...document.querySelectorAll('input')].map(i=>i.placeholder||'(no-ph)').join('|')`,
    )
    check('W1 Web 登录', false, `登录表单未出现 inputs=${JSON.stringify(inputs)}`)
    return false
  }
  await fill('input[placeholder="用户名"]', adminUser)
  await fill('input[placeholder="密码"]', adminPass)
  const expr = `(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.replace(/\\s+/g,'')==='登录');if(!b)return 'NOBTN';b.click();return 'OK'})()`
  await evalJS(expr)
  const ok = await waitJS(`!location.pathname.includes('login')`, 12000)
  check('W1 Web 登录', ok, ok ? 'admin → 控制台' : '登录未跳转')
  if (ok) await screenshot('fw-01-login.png')
  return ok
}

// 空态：仅有无防火墙工具的主机在线
async function scenarioEmpty() {
  await navigate(webBase + '/firewall')
  const ok = await waitBodyIncludes('暂无带防火墙工具的在线主机', 10000)
  check('W2 空态（无防火墙工具主机在线）', ok, ok ? 'Empty 文案已渲染' : '未出现空态文案')
  if (ok) await screenshot('fw-02-empty.png')
  return ok
}

// 总览表三行形态 + 页顶截断告警
async function scenarioOverview() {
  await navigate(webBase + '/firewall')
  // legacy 行：后端 iptables + variant legacy + input accept 徽标
  const okLegacy = await waitBodyIncludes('legacy', 12000)
  let text = ''
  try {
    text = await bodyText()
  } catch {
    /* fallthrough */
  }
  const hasInputAccept = /input\s*accept/.test(text)
  check(
    'W3a legacy 行（后端 iptables + variant 标注）',
    okLegacy && text.includes('iptables'),
    `variant=${okLegacy} iptables=${text.includes('iptables')}`,
  )
  check('W3b 默认策略徽标 input accept', hasInputAccept, hasInputAccept ? 'accept 警示色徽标' : '未渲染')

  // 非 root 行：不可读
  const norootHost = hosts.noroot || ''
  const okNoroot = norootHost ? await waitBodyIncludes('不可读', 10000) : false
  check('W3c 非 root 行状态列不可读', okNoroot, okNoroot ? '不可读 + 原因透传' : '未出现')

  // 超大规则集行：部分读取 + 页顶 4MB 截断告警
  const okTruncAlert = await waitBodyIncludes('规则集过大，仅显示前 4MB', 10000)
  const okPartial = text.includes('部分读取')
  check(
    'W3d 超大规则集行部分读取 + 页顶截断告警',
    okTruncAlert,
    `alert=${okTruncAlert} partial=${okPartial}`,
  )
  if (okLegacy) await screenshot('fw-03-overview.png')
  return okLegacy && hasInputAccept && okNoroot && okTruncAlert
}

async function expandPanel(host, waitText, name, shot) {
  if (!host) {
    check(name, false, '未提供主机名')
    return false
  }
  const clicked = await clickHeader(host)
  if (!clicked) {
    check(name, false, `未找到面板头 ${host}`)
    return false
  }
  const ok = await waitBodyIncludes(waitText, 8000)
  check(name, ok, ok ? `展开后出现「${waitText}」` : `展开后未见「${waitText}」`)
  if (ok) await screenshot(shot)
  return ok
}

async function main() {
  if (!adminPass) fatal('缺少 --pass')

  // 清残留 tab（上轮 FAIL 跳过清理会拖僵 headless 渲染）
  try {
    const list = await fetch(cdpBase + '/json/list').then((r) => r.json())
    for (const t of Array.isArray(list) ? list : []) {
      await fetch(cdpBase + '/json/close/' + t.id).catch(() => {})
    }
  } catch {
    /* chrome 未起时由 newTab 报错 */
  }

  const tab = await putJSON(cdpBase + '/json/new?about:blank')
  if (!tab || !tab.webSocketDebuggerUrl) fatal('json/new 失败（chrome 是否起在 ' + cdpBase + '？）')
  await connect(tab.webSocketDebuggerUrl)
  await call('Network.enable', {})
  await call('Network.clearBrowserCache', {})
  await call('Network.setCacheDisabled', { cacheDisabled: true })

  if (!(await login())) process.exit(1)

  let ok = true
  if (phase === 'empty') {
    ok = await scenarioEmpty()
  } else {
    ok = (await scenarioOverview()) && ok
    // W4 legacy 面板：规则明细 + cockpit 标注
    ok =
      (await expandPanel(
        hosts.legacy || '',
        'cockpit',
        'W4 legacy 面板（规则明细 + cockpit 标注）',
        'fw-04-panel-legacy.png',
      )) &&
      ok
    // W5 非 root 面板：说明态
    ok =
      (await expandPanel(
        hosts.noroot || '',
        '防火墙规则集不可读',
        'W5 非 root 面板说明态',
        'fw-05-panel-noroot.png',
      )) &&
      ok
    // W6 超大规则集面板：meta-only 错误说明（非「规则集为空」误报）
    const bigHost = hosts.big || ''
    if (bigHost) {
      const clicked = await clickHeader(bigHost)
      const okW6 = clicked && (await waitBodyIncludes('exceeds', 8000))
      const text = okW6 ? await bodyText() : ''
      const notMisleading = !text.includes('规则集为空（未配置任何规则）')
      check(
        'W6 超大规则集面板 meta-only 说明',
        okW6 && notMisleading,
        okW6 ? (notMisleading ? '错误说明渲染，无误报' : '误报「规则集为空」') : '未见 exceeds 说明',
      )
      if (okW6) await screenshot('fw-06-panel-big.png')
      ok = okW6 && notMisleading && ok
    }
  }

  cleanup()
  process.exit(ok && fails === 0 ? 0 : 1)
}

function cleanup() {
  try {
    if (ws) ws.close()
  } catch {
    /* ignore */
  }
}

main().catch((e) => {
  console.error('[FATAL]', e.message)
  cleanup()
  process.exit(1)
})
