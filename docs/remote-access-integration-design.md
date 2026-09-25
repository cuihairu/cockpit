# 远控三协议第三方集成设计：VNC / SSH / RDP 统一 Guacamole 栈

> 2026-09-25 立项。方向对齐另一仓库：VNC / SSH / RDP 三协议的远控统一集成
> 第三方方案，不再各自维护自研协议链路。RDP / VNC 已在
> [remote-desktop-guacamole-design](./remote-desktop-guacamole-design.md)
> 落地（Guacamole 路线）；本文补齐 **SSH 接入同一栈**，并给出三协议的
> 选型总览。旧设计「不做：SSH/telnet 走 Guacamole」一条自本文起**废止**
> （见 D7），其余决策不受影响。

## 现状（2026-09-25 复核）

| 协议 | 前端入口 | 通道 | 协议终结方（第三方库） | 状态 |
| --- | --- | --- | --- | --- |
| RDP | GuacamoleModal | `/api/remote/guacamole` → guacd | guacd（FreeRDP）+ guacamole-common-js | ✅ 已集成 |
| VNC | GuacamoleModal | 同上（protocol=vnc） | guacd（libvncclient）+ guacamole-common-js | ✅ 已集成 |
| SSH | TerminalModal（xterm.js） | `/api/remote/terminal` → agent | agent 内 `x/crypto/ssh`（Go 库，非远控专用栈） | ⚠️ 自研链路 |
| Telnet | TerminalModal | 同上 | agent raw TCP | 维持 |
| （兜底） | VNCModal（noVNC）/ DesktopModal（grdp） | agent 通道 | noVNC / grdp | 无引用，按 2026-09-24 拍板保留并存期 |

缺口只有一项：**SSH 未走第三方远控栈**。VNC/RDP 的痛点论述（自研协议
链路的维护成本、渲染/输入/剪贴板自担）见旧设计文档「痛点」章节，SSH
同理——且 SSH 是三协议中唯一还留在自研链路上的。

## 选型

### 三协议客户端选型表

| 协议 | 选型 | 底层引擎 | 接入点 |
| --- | --- | --- | --- |
| RDP | guacd 的 rdp 插件 | FreeRDP | `/api/remote/guacamole`（protocol=rdp） |
| VNC | guacd 的 vnc 插件 | libvncclient | `/api/remote/guacamole`（protocol=vnc） |
| SSH | guacd 的 ssh 插件 | libssh2 | `/api/remote/guacamole`（protocol=ssh）🆕 |
| 浏览器渲染/输入 | guacamole-common-js | canvas / keyboard / clipboard | GuacamoleModal（三协议复用同一组件） |

**一句话理由**：guacd 一个守护进程覆盖三协议（外加 telnet 可选），浏览器
侧 common-js 一个客户端全包；会话/票据/审计/出口策略仍在 Go 后端
（旧设计 D4 的边界不变），guacd 无状态、无用户概念。

### 备选方案为什么不用

| 方案 | 覆盖 | 不用理由 |
| --- | --- | --- |
| **noVNC**（现 VNCModal） | 仅 VNC（RFB） | 只覆盖 VNC，RDP/SSH 无着落；与 RDP 栈无法共享渲染/输入/录制代码。保留为兜底，不再扩展。 |
| **x/crypto/ssh + xterm.js**（现状 agent 通道） | 仅 SSH | 这是「Go 库 + 终端模拟器」的自组链路而非远控方案：host key（TOFU）、保活、录制、协议演进全部自担。作为 agent 侧兜底保留，不作为主入口。 |
| **WebSSH / sshwifty / ttyd** 等独立终端网关 | 仅 SSH | 多部署一个服务、多一套账号体系；票据/审计/出口策略要二次打通。guacd 已在部署内。 |
| **server 直连 crypto/ssh 自研网关** | 仅 SSH | 等于把 agent 通道平移到 server，仍要自研录制与协议维护；与「集成第三方方案」方向相反。 |
| **Teleport** | SSH/K8s/DB | 企业级全家桶（自带用户库/证书体系/Proxy），个人 homelab 场景过重。 |
| **Apache Guacamole 完整版**（含 Java web 层） | RDP/VNC/SSH/Telnet | 旧设计 D1 已否决：Java 层与 Cockpit 的 Go 后端职责重叠（认证/会话/审计都在 Go 侧）。只取 guacd + common-js。 |
| **Myrtille / SparkView** | RDP 为主 | 商业许可或活跃度不足，协议覆盖不全。 |

## 协议与数据流（SSH 新增段）

```
浏览器                    Go 网关                         guacd                  目标
   │  WSS /api/remote/guacamole?ticket=…                     │                    │
   │  (Sec-WebSocket-Protocol 兜底)                          │                    │
   │ ────────────────────────────────────────────────────────>│  TCP select "ssh"  │
   │                          参数名列表响应 <──────────────── │                    │
   │                          size 1280x800 + connect ──────> │ ── libssh2 ──────> │ :22
   │                          (username/password/private-key) │    认证 + PTY      │
   │ <────────────── 指令流（终端图块/剪贴板/sync）─────────── │ <───────────────── │
   │ ─────────────── 键盘/鼠标/剪贴板指令（key/mouse/clip）──> │ ────────────────> │
```

- **浏览器 ↔ server 段**：WebSocket 文本帧承载 Guacamole 指令流，与
  RDP/VNC 完全同一管道；网关不解析指令内容（握手指令构造除外）。
- **server ↔ guacd 段**：`select ssh` → 参数列表 → `size` + `connect`。
  `size` 的像素宽高由 guacd 的 ssh 插件按默认字体度量换算成终端列/行
  （connect 参数不传 width/height——官方 ssh 参数集无此项）。
- **guacd ↔ 目标段**：libssh2 拨 `hostname:port`，认证用 connect 指令
  携带的凭据；终端输出编码为 Guacamole 指令流（图块）推回。

### connect 参数映射（D2）

| ticket 参数 | guacd connect 参数 | 说明 |
| --- | --- | --- |
| `username` | `username` | 直传（上游已统一处理） |
| `password` | `password` | 直传；口令认证 |
| `private_key` | `private-key` | **PEM 原文 → base64 编码**（guacd 约定：base64 编码的私钥内容）；私钥认证，优先于口令 |
| `domain` | 不传 | SSH 无域概念（RDP 专用） |
| `width`/`height` | 不传 | 字符终端；尺寸经隧道层 `size` 指令（guacd 自行换算列/行） |

## 与现有远控链路衔接

全部复用，零新增基础设施：

1. **票据**：`POST /api/remote/tickets` 的 `private_key` 字段早已存在
   （agent 通道 SSH 私钥用），Guacamole 路线直接复用；票据仍为 5 分钟
   一次性、内存态，凭据不进 URL/日志/审计。
2. **出口策略**：`matchRemoteEgress`（allow-list / CIDR / per-agent），
   agent_id 作为策略锚点与审计归属——与 RDP/VNC 语义一致（guacd 从自身
   网络位置拨目标，agent 不在数据路径上，旧设计 D4）。
3. **审计**：`auditRemoteStart/End` 全协议同款；SSH 经 guacd 的会话同样
   记 `Protocol: ssh`，凭据（口令/私钥）绝进审计详情。
4. **录制**：guacd 的 ssh 插件同样支持 `recording-path/recording-name`
   → `.guac` 收集链路（`collectGuacRecording`，Format=guac）原样生效，
   SSH 会话录制与 RDP/VNC 同一 `/recordings` 页回放。
5. **权限**：`terminal:write`（RBAC `remote` 资源域），入口按钮的
   PermGuard 不因通道切换而变。

## 决策

- **D1 协议白名单扩 `ssh`**：`handleGuacamoleWebSocket` 放行 rdp/vnc/ssh；
  **telnet 不进**（方向只提三协议；telnet 无加密，维持 agent 通道现状）。
- **D2 connect 参数映射**：见上表；私钥 base64 编码在 Go 网关侧完成
  （ticket 存 PEM 原文，编码属 guacd 协议细节，不该散到前端）。
- **D3 前端入口切换**：web 默认 SSH 入口从 TerminalModal 切到
  GuacamoleModal（Workbench SSH Tab 与 Agents 页统一分流），与 RDP/VNC
  一致；SSH Tab 保留「内置终端（经 Agent）」次入口作兜底。
- **D4 agent 通道降级为兜底、不删**：与 grdp 路线同一「并存期」纪律——
  TerminalModal 保留（telnet 仍走它；guacd 不可达时 SSH 有退路；移动端
  继续用它）。
- **D5 录制/审计/出口策略复用**：见上节，零新增配置键。
- **D6 移动端不动**：mobile 的 SSH 终端走 `/api/remote/terminal` +
  xterm.dart 独立链路，本次不改（Flutter 端无 guacamole 客户端，后续
  单独评估）。
- **D7 旧决策废止**：remote-desktop-guacamole-design.md「不做：SSH/telnet
  走 Guacamole」一条废止（telnet 维持不做，SSH 接入）；其余「不做」
  （音频 UI、文件传输、SFTP 等）不变。

### 体验差异（如实记录）

guacd 的 ssh 终端是**服务端渲染的图块流**（guacamole 指令编码的位图），
不是 xterm.js 的字符网格：带宽占用更高、字体渲染由 guacd 决定。统一栈
（同一组件、同一录制、同一部署）的收益大于该差异；对终端体验有极致
要求的场景用「内置终端（经 Agent）」兜底入口。此差异是 D3/D4 并存的
根本原因，不是过渡态。

### D8：SSH 尺寸跟窗口走、剪贴板只对终端开反向

「接入同一栈」不等于「三协议行为一致」——两处按协议分岔，都是语义决定的
结果而非实现偷懒：

| | RDP / VNC（桌面） | SSH（终端） |
| --- | --- | --- |
| `size` 指令语义 | 「切远端桌面分辨率」 | 「终端占多大像素 → guacd 换算列/行」 |
| 谁来发 | 工具栏分辨率下拉（用户显式选） | `ResizeObserver` 监听 display 容器 + 150ms 去抖，自动跟窗口/全屏变形 |
| 工具栏分辨率下拉 | 显示 | **隐藏**（尺寸跟窗口走，固定档位会与之打架） |
| 剪贴板反向（浏览器 → 远端） | 不接（维持既有的仅正向，桌面路径零行为变更） | 接：终端区域 Ctrl+V（`paste` 事件，自带数据不需 Clipboard 授权）+ 工具栏「粘贴到远程」按钮（`navigator.clipboard.readText()`，读失败提示走 Ctrl+V） |

桌面侧不自动发 `size` 的理由：那会让「窗口多大桌面就多大」，全屏时把
用户精心选定的分辨率改掉——与既有行为不符。终端侧必须自动发的理由：
`vim` / `top` 这类全屏程序按列/行重绘，不跟随就会停在建连时的旧列行数、
右侧留黑边。

反向剪贴板用 `Client.createClipboardStream('text/plain')` +
`Guacamole.StringWriter`（common-js 官方写法）；Go 网关是字节管道，
`clipboard` 指令原样透传到 guacd，**后端零改动**。纯文本单层
（富格式见「不做」）。

### 实现补记

- `GuacamoleModal` 里 `useConnectionTimeout` 的 `clear` 曾解构成
  `clearTimeout`，**遮蔽了全局 `clearTimeout`**——尺寸同步去抖第一版就
  被它坑了（连续两次 resize 发了两条 `size`）。已改名 `clearConnTimeout`。
  组件内还有真实定时器，别再把 hook 的 clear 叫 `clearTimeout`。
- `RemoteToolbar` 的「粘贴到远程」按钮补了 `aria-label`（此前 icon-only
  无可访问名，测试也只能靠按钮序号定位）。

## 测试与验收

**单测（本次落地）**：

- Go：`guacConnectArgs` ssh 分支（username/password 直传、private-key
  base64、无 width/height/domain）；WS 白名单 ssh 放行、telnet 拒绝；
  全链路（covGuacdServer）ssh 握手指令断言。
- web：GuacamoleModal ssh 表单（用户名必填、口令/私钥可选、无私钥时
  不传 private_key）；Workbench SSH 分流到 GuacamoleModal、内置终端
  次入口仍开 TerminalModal。
- web（D8 补齐）：ssh 尺寸同步（容器 resize 去抖后发 `sendSize` 末次
  尺寸、0 尺寸跳过、RDP 不挂 ResizeObserver、卸载时 effect cleanup）；
  剪贴板反向（终端 paste / 工具栏按钮、readText 失败兜底）；SSH 隐藏
  分辨率下拉而 RDP 保留；Agents 页 ssh 分流断言随 D3 更新。
- `web/src/test/setup.ts` 加 ResizeObserver 可观测桩（jsdom 无实现），
  触发器 `window.__triggerResize(w, h)`。

**真机验收（挂起待验，入 acceptance-checklist）**：

- 真 guacd + 真 sshd：口令认证 / 私钥认证各一次；
- 终端渲染（vim/top 全屏程序）、`size` 变更换算列/行；
- SSH 会话 `.guac` 录制收集与 `/recordings` 回放；
- 剪贴板双向（浏览器 ↔ sshd）。

## 不做（后续版本）

- **telnet 经 guacd**：方向只提三协议；telnet 维持 agent 通道。
- **SFTP / Guacamole Filesystem drive**：与 file-manager 路线重叠，
  维持旧设计「不做」。
- **删除 agent SSH 通道 / TerminalModal / VNCModal / DesktopModal**：
  并存期纪律，等 Guacamole 方案真机验收后统一定夺。
- **移动端 SSH 切 guacamole**：见 D6。
- **guacd ssh 专属参数**（font-name/color-scheme/timezone/backspace 等）：
  最小参数集起步，有需求再加（参数是 connect 指令追加项，零迁移）。
