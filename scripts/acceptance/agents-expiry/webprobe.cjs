#!/usr/bin/env node
/**
 * 过期 agent 验收 Web 侧探针（D-2026-10-08-3，agents-expiry/probe.sh 调用）。
 *
 * 手写 CDP 客户端（同 firewall/webprobe.cjs 形状：Node ≥22 内置 WebSocket，
 * 零依赖；/json/new 必须 PUT；两汉字按钮 autoInsertSpace 去空白匹配）。
 *
 * 前置：probe.sh 已完成 E1-E4（server :19989，AGENT_EXPIRE_MINUTES=1，
 * 幽灵 agent 行已过期待隐藏，Chrome 容器 CDP :9226）。
 *
 *   E5a /agents 页：角标「过期阈值」+ Alert「已隐藏 1 台过期 agent」+
 *       过期行不在表格里 → exp-01-hidden.png
 *   E5b 点「显示」→ 行回来且标「离线」→ exp-02-shown.png
 *   E6  点「清理离线 agent」→ 弹窗「确认清理」→ toast + 行物理消失
 *       → exp-03-cleaned.png
 *
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

const webBase = args.web || 'http://127.0.0.1:19989'
const cdpBase = args.cdp || 'http://127.0.0.1:9226'
const adminUser = args.user || 'admin'
const adminPass = args.pass || ''
const evDir = args.ev || '.acceptance/agents-expiry'

let passes = 0
let fails = 0
function check(name, ok, detail) {
  const status = ok ? 'PASS' : 'FAIL'
  if (ok) passes++
  else fails++
  console.log(`[${status}] ${name} — ${detail}`)
}

// ---------- CDP 客户端（同 firewall/webprobe.cjs） ----------

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
    ws.onopen = () => { clearTimeout(timer); resolve() }
    ws.onerror = () => { clearTimeout(timer); reject(new Error('ws error')) }
    ws.onmessage = (ev) => {
      let m
      try { m = JSON.parse(ev.data) } catch { return }
      if (m.id && pending.has(m.id)) {
        const { resolve: res, reject: rej } = pending.get(m.id)
        pending.delete(m.id)
        if (m.error) rej(new Error(`${m.error.message}`))
        else res(m.result)
      }
    }
  })
}

function call(method, params) {
  return new Promise((resolve, reject) => {
    const id = ++nextID
    const timer = setTimeout(() => {
      if (pending.has(id)) { pending.delete(id); reject(new Error(`${method}: timeout`)) }
    }, 30000)
    pending.set(id, {
      resolve: (v) => { clearTimeout(timer); resolve(v) },
      reject: (e) => { clearTimeout(timer); reject(e) },
    })
    ws.send(JSON.stringify({ id, method, params: params || {} }))
  })
}

async function evalJS(expr) {
  const res = await call('Runtime.evaluate', {
    expression: expr, returnByValue: true, awaitPromise: true,
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
    } catch { /* 跳转途中容忍 */ }
    await new Promise((r) => setTimeout(r, 400))
  }
  return false
}

async function navigate(url) {
  await call('Page.navigate', { url })
  await waitJS("document.readyState==='complete'", 15000)
  await new Promise((r) => setTimeout(r, 900))
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

function jsStr(s) { return JSON.stringify(s) }

async function fill(selector, value) {
  const expr = `(()=>{const el=document.querySelector(${jsStr(selector)});if(!el)return 'NOEL';
const proto=el.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;
Object.getOwnPropertyDescriptor(proto,'value').set.call(el,${jsStr(value)});
el.dispatchEvent(new Event('input',{bubbles:true}));return 'OK'})()`
  return (await evalJS(expr)) === 'OK'
}

function bodyText() { return evalJS('document.body.innerText') }

async function waitBodyIncludes(text, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      if ((await bodyText()).includes(text)) return true
    } catch { /* 轮询容忍 */ }
    await new Promise((r) => setTimeout(r, 400))
  }
  return false
}

// clickText 点含指定文本的按钮（双侧去空白：antd 两汉字按钮 autoInsertSpace
// 插空格；带空格按钮文本如「清理离线 agent」同样归一）
async function clickText(text) {
  const expr = `(()=>{const t=${jsStr(text)}.replace(/\\s+/g,'');
const b=[...document.querySelectorAll('button')].find(b=>b.textContent.replace(/\\s+/g,'')===t);
if(!b)return 'NOBTN';b.click();return 'OK'})()`
  return (await evalJS(expr)) === 'OK'
}

async function login() {
  await navigate(webBase + '/login')
  if (!(await waitJS(`!!document.querySelector('input[placeholder="用户名"]')`, 10000))) {
    const inputs = await evalJS(
      `[...document.querySelectorAll('input')].map(i=>i.placeholder||'(no-ph)').join('|')`)
    const state = await evalJS('location.href + " | " + document.body.innerText.slice(0, 200)')
    check('E5 登录', false, `登录表单未出现 inputs=${JSON.stringify(inputs)} state=${state}`)
    return false
  }
  await fill('input[placeholder="用户名"]', adminUser)
  await fill('input[placeholder="密码"]', adminPass)
  await clickText('登录')
  const ok = await waitJS(`!location.pathname.includes('login')`, 12000)
  check('E5 登录', ok, ok ? 'admin → 控制台' : '登录未跳转')
  return ok
}

async function main() {
  if (!adminPass) { console.error('[FATAL] 缺少 --pass'); process.exit(1) }

  // 清残留 tab（多 tab 渲染僵死，同 firewall 探针纪律）
  try {
    const list = await fetch(cdpBase + '/json/list').then((r) => r.json())
    for (const t of Array.isArray(list) ? list : []) {
      await fetch(cdpBase + '/json/close/' + t.id).catch(() => {})
    }
  } catch { /* chrome 未起时由 newTab 报错 */ }

  const tab = await putJSON(cdpBase + '/json/new?about:blank')
  if (!tab || !tab.webSocketDebuggerUrl) {
    console.error('[FATAL] json/new 失败（chrome 是否起在 ' + cdpBase + '？）')
    process.exit(1)
  }
  await connect(tab.webSocketDebuggerUrl)
  await call('Page.enable', {})
  await call('Network.enable', {})
  await call('Network.clearBrowserCache', {})
  await call('Network.setCacheDisabled', { cacheDisabled: true })

  if (!(await login())) process.exit(1)

  let ok = true

  // E5a：过期行默认隐藏 + 阈值旁注 + Alert
  await navigate(webBase + '/agents')
  const okThreshold = await waitBodyIncludes('过期阈值：1 分钟', 10000)
  check('E5a 阈值配置可见（过期阈值：1 分钟）', okThreshold,
    okThreshold ? 'Card 角标在' : '未渲染')
  const okAlert = await waitBodyIncludes('已隐藏 1 台过期 agent', 8000)
  check('E5a Alert「已隐藏 1 台过期 agent」', okAlert, okAlert ? '隐藏提示在' : '未渲染')
  const rowGone = await evalJS(
    `document.body.innerText.includes('agex-acc-a1') ? 'VISIBLE' : 'HIDDEN'`)
  check('E5a 过期行默认不在列表', rowGone === 'HIDDEN', `行=${rowGone}`)
  if (okThreshold && okAlert && rowGone === 'HIDDEN') {
    await screenshot('exp-01-hidden.png')
  } else {
    await screenshot('exp-01-hidden-FAIL.png')
    ok = false
  }

  // E5b：显示切换 → 行回来标「离线」
  if (!(await clickText('显示'))) {
    check('E5b 显示切换', false, '未找到「显示」按钮')
    ok = false
  } else {
    const shown = await waitBodyIncludes('agex-acc-a1', 8000)
    // 行内状态徽标「离线」
    const offlineBadge = await evalJS(
      `[...document.querySelectorAll('tr.ant-table-row')]
        .find(tr=>tr.textContent.includes('agex-acc-a1'))?.innerText.includes('离线')`)
    check('E5b 显示后行在且标「离线」', shown && offlineBadge === 'true',
      `shown=${shown} badge=${offlineBadge}`)
    if (shown && offlineBadge === 'true') await screenshot('exp-02-shown.png')
    else { await screenshot('exp-02-shown-FAIL.png'); ok = false }
  }

  // E6：手动清理（阈值留空 = 全部离线）
  if (!(await clickText('清理离线 agent'))) {
    check('E6 手动清理', false, '未找到清理按钮')
    ok = false
  } else {
    const modal = await waitJS(`!!document.querySelector('.ant-modal')`, 8000)
    if (!modal) { check('E6 清理弹窗', false, 'Modal 未出现'); ok = false }
    else {
      await screenshot('exp-03a-confirm-modal.png')
      if (!(await clickText('确认清理'))) {
        check('E6 确认清理', false, '未找到「确认清理」按钮')
        ok = false
      } else {
        // toast（message.success 已清理 N 台）或行消失，二者其一即过
        const toast = await waitBodyIncludes('已清理 1 台离线 Agent', 8000)
        const rowGoneAgain = await waitJS(
          `!document.body.innerText.includes('agex-acc-a1')`, 8000)
        check('E6 清理生效（toast/行消失）', toast || rowGoneAgain,
          `toast=${toast} rowGone=${rowGoneAgain}`)
        await screenshot('exp-03-cleaned.png')
        if (!(toast || rowGoneAgain)) ok = false
      }
    }
  }

  console.log(`\n=== webprobe: ${passes} PASS / ${fails} FAIL ===`)
  process.exit(fails > 0 ? 1 : 0)
}

main().catch((e) => {
  console.error('[FATAL]', e.message)
  process.exit(1)
})
