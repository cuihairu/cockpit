# 远程桌面设计：Apache Guacamole 集成（guacd + guacamole-common-js）

> 2026-09-23 用户拍板立项。此前远控桌面走自研路线（grdp 纯 Go RDP 客户端 +
> 自研 canvas 渲染/扫描码输入/剪贴板），本设计定案**集成 Apache Guacamole**
> 替换之。本文档只做方案设计，实现另行确认。

## 痛点：为什么现在要换掉自研路线

Cockpit 远控桌面现状是「三套协议三套栈」，且**没有一套是可靠的**：

- **RDP**（自研 grdp 路线）：`internal/agent/rdp/session.go` 用纯 Go 的
  `github.com/nakagami/grdp` 终结 RDP 协议，位图回调 → `screen_update` 脏矩形
  → web 自研 `useCanvasRenderer` 帧缓冲合成。问题不在「能不能跑通」，而在
  **协议覆盖广度**：grdp 对 NLA/CredSSP、TLS/RDP security 协商、RemoteFX/GFX
  图形管道的支持远弱于 FreeRDP/xfreerdp 这类工业级实现。真实 Windows Server
  （尤其强制 NLA 的机器）连通性存疑，而 RDP 恰恰是「连不通就是零价值」的协议
  ——办公场景下用户要连的 Windows 机器默认强制 NLA，这不是边缘情况。
- **VNC**（noVNC 路线）：浏览器内跑 RFB 协议，agent 只做二进制 TCP 透传。
  这条路是**唯一能用**的，但仅覆盖 VNC；且 noVNC 与 grdp 是两套输入/渲染/
  剪贴板模型，UI 层无法收敛。
- **SSH**（xterm.js 路线）：刚打通（见 `internal/proxy/ssh_session.go`，
  agent 侧 `crypto/ssh` 终结协议 + PTY）。**SSH 是字符终端不是图形桌面**，
  走 xterm.js 是对的，不该被桌面方案绑架。

自研路线还有隐性成本：渲染（帧缓冲合成、脏矩形、ResizeObserver 合帧）、
输入（DOM 事件 → 扫描码 → RDP 虚拟键）、剪贴板（浏览器 Clipboard API ↔
RDP cliprdr）、分辨率协商——**每一项都是协议细节的重复造轮子**，且每加一个
协议（如将来要 SPICE）就要再抄一遍。

结论：桌面协议（RDP/VNC）交给成熟协议引擎，Cockpit 只做它擅长的——
会话、权限、审计、动态 inventory。

## 架构决策

### D1 只用 guacd + guacamole-common-js，**跳过 guacamole-web 的 Java 层**

这是本设计最核心的一刀，理由是**避免两套身份/权限/审计体系**：

Apache Guacamole 官方发行是三层：`guacd`（C 协议守护进程）+
`guacamole-common-js`（浏览器 JS 库）+ `guacamole-web`（Java servlet 容器
应用，含 JDBC 用户/连接管理）。绝大多数「Guacamole 集成」失败就失败在引了
完整的 `guacamole-web`——它自带一套用户表、连接配置表、权限表，于是 Cockpit
的 RBAC（`internal/auth` + `permAll` 权限点）和 Guacamole 的用户库变成**两套
要人工同步的真相源**，审计也裂成两半（Cockpit 的 `auditRemoteStart/End` +
Guacamole 自己的 connection history）。这与本项目一贯的「单一事实源」纪律
（inventory-first、DB 放索引文件放内容）直接冲突。

拆开看三层各自的角色就清楚该留谁：

- **guacd**：纯协议翻译守护进程。它**没有用户概念**——给它一条
  `RDP → host:3389` 的指令，它就去连并把 RDP 翻译成 Guacamole 指令流。
  连接凭据（用户名/密码）是每条连接传入的，不在 guacd 里存。这正是我们要的
  「协议引擎」边界：它替我们终结 RDP/VNC 协议，但不碰我们的业务模型。
- **guacamole-common-js**：浏览器端 JS 库，实现 Guacamole 协议的
  `Guacamole.Tunnel`（传输）、`Guacamole.Display`（canvas 渲染）、
  `Guacamole.Keyboard`/`Guacamole.Mouse`（输入）、`Guacamole.Client`（会话
  状态机）。它**也不含用户/连接管理**——参数（protocol/host/port/凭据）
  是调用方给的。这层替我们干掉自研渲染/输入/剪贴板三座大山。
- **guacamole-web**：Java servlet + JDBC，负责「谁可以连哪台机器」。
  **这层 Cockpit 已经有了**：RBAC 权限点（`terminal:write` 等）+
  `matchRemoteEgress` 出口策略 + 动态 inventory（agent 上报的
  `remote-services` capability）+ 一次性票据（`internal/server/ticket.go`）。
  引它就是引第二套，且它的「静态连接配置」模型与 Cockpit 的
  动态 inventory 理念相反（见 `reference-projects.md` 的 Guacamole 缺点
  记录）。

许可证上同理成立：Cockpit 是 **Apache License 2.0**，guacd 与
guacamole-common-js 同为 Apache 2.0，同许可证集成零传染风险；guacamole-web
虽也是 Apache 2.0（不构成障碍），但**跳过它的理由是架构重复，不是许可证**。

### D2 隧道走 Go WS 网关反代 guacd，不暴露 guacd 端口

guacd 只讲 Guacamole 协议，默认监听 TCP `4822`。浏览器侧
`guacamole-common-js` 的 `Guacamole.Tunnel` 有两个实现：
`Guacamole.HTTPTunnel`（HTTP 长轮询，走 `guacamole-web` 的 servlet）和
`Guacamole.WebSocketTunnel`（WebSocket，URL 指向服务端 WS 端点）。**没有
「浏览器直连 guacd」这条路**——guacd 不是 HTTP 服务，且内网端口不该暴露。

所以需要一个 WS 网关：浏览器 WS ↔ Go 后端 ↔ guacd TCP。这个位置正好是
Cockpit server 已经坐着的（现有 `/api/remote/terminal`、`/api/remote/desktop`
都是「浏览器 WS ↔ server ↔ agent WS」的双跳管道，server 本就是远控网关）。

放 Go 而不是放 guacamole-web 的理由同 D1：网关这里要做的不只是字节转发，
还有**会话生命周期、票据校验、权限判定、审计落库**——全是 Cockpit 已有的
能力。guacamole-web 的 `GuacamoleWebSocketTunnelEndpoint` 做的是同样的事，
但它用 Java 的 session + 它的权限表，又回到两套体系。

反代的具体形态：Go 后端与 guacd 之间**用 Guacamole 原生 TCP 协议**（不用
guacd 的 WebSocket 模式），因为 guacd 的协议本就是「指令流 + base64 chunk」
的文本协议，TCP 转发最直接；Go 侧把 WS 的文本帧 ↔ guacd 的 TCP 流做双向
管道，**不解析指令内容**（除了握手阶段的 `select`/`size`/`audio`/`video`
参数指令需要 Go 构造）。这样协议演进（Guacamole 1.5.x 新指令）由
guacamole-common-js 与 guacd 自己对齐，Go 网关不跟随。

### D3 复用现有票据与 401 递归刷新链路

现有票据语义（`internal/server/ticket.go`）：`POST /api/remote/tickets`
→ 一次性票据、5 分钟有效、`ValidateTicket` 即消费（`Consumed` 标记）。
这是为「WS 握手凭证」设计的——握手一次即作废，泄漏窗口 5 分钟。

Guacamole 隧道**复用同一条签发票据的端点与审计**，票据仍是 WS 握手凭证
（一次性语义不变），差异只在「握手成功后 server 侧持有 guacd TCP 连接直到
会话结束」而非「转发到 agent」。这样对前端是一致的体验：`createRemoteTicket`
→ 拿 ticket → 连 WS；对后端审计是一致的：`auditRemoteStart` 记
`Protocol: "rdp"`/`"vnc"`，会话结束 `auditRemoteEnd` 记 duration。

**为什么票据保持一次性**：Guacamole 隧道虽然长连接，但「建立隧道」这个动作
本身就是一次性握手。连接断了要重连就重新申请票据——这与现有 terminal/desktop
的重连行为一致（`TerminalModal` 的「重连」按钮就是重新 `createRemoteTicket`）。
把票据改成会话级长效凭证只会扩大泄漏窗口，收益为零。

401 递归刷新链路照搬现有实现，两侧都有先例：

- web `services/api.ts:124` 的 401 拦截 + `refreshToken()`（`/auth/refresh`），
  且 mobile `api/client.dart:83-91` 明确记录了**递归刷新防护**：
  `extra['skipRefresh'] == true` 的请求（即刷新请求自身）收到 401 不再触发
  刷新，直接交给 `onUnauthorized`——否则刷新失败会无限递归。
- Guacamole 隧道的票据申请走同一个 `remoteClient`（已有 Bearer 拦截器），
  401 行为完全一致；WS 握手本身不带 JWT（票据即凭证），故 WS 路径不需要
  刷新，重连时经 `createRemoteTicket` 自然走一遍 401 刷新。

这条「复用」的意义在于**不发明第二套鉴权失败语义**：用户 token 过期后的行为
（无感刷新 → 失败登出回登录页）在普通 API、终端、桌面、Guacamole 隧道上
是同一个。

### D4 会话/权限/审计放 Go 后端，guacd 无状态

guacd 是**无状态协议翻译器**：一次 `connect` 指令建一条 TCP 到目标，
翻译指令流，`disconnect` 即结束。它不知道「谁」在连——用户名/密码是
`connect` 指令的参数。这个特性决定了职责划分：

- **Go 后端**：票据签发（含 `matchRemoteEgress` 出口策略校验）、
  `terminal:write` 权限点判定、`auditRemoteStart/End/Failure` 落库、
  `RemoteSessionManager` 会话登记（pending/connected/closed/failed）、
  会话录制开关与保留策略。
- **guacd**：只翻译协议。凭据经 Go 网关透传到 `connect` 指令，**不落
  guacd 日志、不进 guacd 配置**（guacd 的 `GUACD_LOG_LEVEL` 建议保持
  `info` 以下，避免指令内容进日志）。
- **guacamole-common-js**：只渲染与输入。它对「谁能连」零感知。

这与现有 `remote_audit.go` 的纪律一致：审计详情**不含口令**
（`auditRemoteStart` 注释明确「不写 password」），Guacamole 路径同样遵守。

### D5 guacd 用 docker-compose 部署

guacd 是独立 C 守护进程，官方镜像 `apache/guacamole`（含 guacd）。
用 docker-compose 部署而不是要求宿主机装 guacd，理由：

- 与 Cockpit 的 stacks/Docker 能力一致（用户本就有 Docker 主机）；
- guacd 依赖 FreeRDP/VNC 客户端库（libguac-client-rdp 等），源码编译
  依赖链长，镜像已经把这些编好；
- 版本升级（Guacamole 1.5.x → 1.6.x）只动 compose 的 image tag；
- guacd 无持久化状态，容器无状态可随意重建。

代价是多一个容器要管——但这正是 stacks 功能的价值场景（见
`stack-deploy-design.md`），用 Cockpit 自己部署自己的 guacd 是自洽的。

## 阶段 1 验收范围

阶段 1 目标是**「连得上、操作得了」**，不做音频、不做文件传输、不做剪贴板
高级格式（纯文本剪贴板属基础输入输出，包含在内）：

1. **浏览器 RDP 连 Windows**
   - 目标：Windows 专业版/企业版/Server 的远程桌面服务（`termsrv`），
     支持 NLA（CredSSP）认证。
   - **Win11 Home 无 RDP server**：Windows 家庭版不提供远程桌面**服务端**
     （只能作为客户端连别人）。这是微软的产品线划分，不是本方案缺陷。
     因此 Win11 Home 机器的验收走 **VNC 兜底**——装 TightVNC/Vino 等 VNC
     server，Guacamole 的 VNC 协议引擎连之。阶段 1 验收项「RDP 连 Windows」
     的目标机器是专业版/Server，家庭版机器另列「VNC 兜底」子项。
2. **VNC 连 Linux**：Linux 桌面的 VNC server（TigerVNC/TightVNC/x11vnc），
     支持 VNC 密码认证。
3. **iPad Safari 触控可用**：iPadOS Safari 的触控事件映射到 Guacamole
   鼠标指令（`Guacamole.Mouse` 已处理 touch → mouse），单指移动/点击、
   双指滚动可操作；屏幕旋转后 `size` 指令重新协商分辨率。

**不在阶段 1**：音频（见风险章节）、文件传输（`Guacamole.Filesystem`）、
剪贴板富格式（RTF/HTML）、SFTP 文件传输叠加（Guacamole 的 RDP 驱动器
重定向）。

## 方案对比

| 方案 | 覆盖协议 | 许可证 | 关键取舍 |
|------|---------|--------|---------|
| **noVNC** | **仅 VNC**（RFB） | MPL 2.0 | 轻量、浏览器内跑 RFB，Cockpit 已用。但**只覆盖 VNC**，RDP 还得另找一套；与 RDP 栈无法共享渲染/输入/剪贴板代码。若确定只做 VNC 它够用，但阶段 1 明确要求 RDP。 |
| **RustDesk Web** | RustDesk 自有协议（不是 RDP/VNC） | **AGPL 3.0** | 性能好（自研编解码）、开箱即用。**排除理由是许可证传染**：AGPL 要求分发衍生作品时整个作品开源，Cockpit 是 Apache 2.0，集成 AGPL 组件会使整个组合件受 AGPL 约束，与项目许可证选择冲突。且 RustDesk 协议是自有协议，**连不了标准 RDP/VNC 服务器**——它要两端都装 RustDesk，与「连用户已有 Windows 远程桌面」的需求不符。 |
| **xterm.js** | 字符终端（SSH/telnet） | Apache 2.0 | 已用于 SSH（`TerminalModal`），**但这不是桌面方案**。xterm.js 是终端模拟器，渲染的是字符网格不是像素；RDP/VNC 的图形流无处安放。**SSH 单走 xterm.js 是对的**，不该被桌面方案绑架——Guacamole 也支持 SSH，但我们已自研打通（agent 侧 `crypto/ssh` + PTY），替换收益低于迁移成本。 |
| **自研（grdp + canvas）** | RDP（部分） | MIT（grdp） | 现状。**否**：协议覆盖弱于 FreeRDP（NLA/CredSSP/TLS 协商/RemoteFX/GFX），真实 Windows 连通性存疑；渲染/输入/剪贴板三块全要自己维护；每加一个协议重抄一遍。沉没成本不构成继续投入的理由。 |
| **Guacamole（guacd + common-js）** | **RDP / VNC / SSH / Telnet** | Apache 2.0 | **取**：协议引擎是 FreeRDP/VNC 客户端库（工业级覆盖），渲染/输入/剪贴板由 common-js 承担，与 Cockpit 同许可证零传染；guacd 无状态、无用户概念，与「业务在 Go 后端」的边界干净。代价是多一个 guacd 进程 + Guacamole 协议隧道网关要自己写（但网关正是 Cockpit 已在坐的位置）。 |

**为什么不用 Teleport**（顺带记）：Teleport 是身份为中心的访问平台，
包含证书短时效、审批流、会话录制，能力比 Guacamole 全。但它**自带一套
用户/角色/资源模型**（与 guacamole-web 同样的「第二套体系」问题），且桌面
协议支持（RDP）需 Teleport 的 desktop access，部署形态是整套集群。Cockpit
是个人/小团队控制台，引入 Teleport 是拿大炮打蚊子且模型冲突。Guacamole
的「只做协议翻译」边界更贴合。

## 骨架代码

### 1. guacd docker-compose

```yaml
# deploy/guacd/docker-compose.yml
services:
  guacd:
    image: guacamole/guacd:1.5.5
    container_name: cockpit-guacd
    restart: unless-stopped
    ports:
      # 仅绑定内网/回环：Go 后端反代连接此端口，不对外暴露
      - "127.0.0.1:4822:4822"
    volumes:
      - guacd-recordings:/var/lib/guacamole:rw   # session recording 落盘
      - guacd-drive:/var/lib/guacamole/drive:rw  # 阶段二文件传输用
    environment:
      # 日志不含指令内容；勿开 debug（会打出 connect 参数含口令）
      GUACD_LOG_LEVEL: info
      TZ: Asia/Shanghai
    # guacd 无状态，可随重建；recording 卷是唯一持久化面
volumes:
  guacd-recordings:
  guacd-drive:
```

要点：端口绑 `127.0.0.1`（Go 网关同机反代；跨机部署时绑内网地址并靠
防火墙收口）；`/var/lib/guacamole` 是 guacd 默认 recording 目录（见风险
章节「session recording 白捡」）；`GUACD_LOG_LEVEL` 不开 debug（guacd
debug 会打印 `connect` 指令参数，含口令）。

### 2. Go 会话接口（WS 网关反代 guacd）

```go
// internal/server/api_guacamole.go
package server

import (
	"bufio"
	"net"
	"strings"

	"github.com/gorilla/websocket"
)

// GuacamoleSession 一条 Guacamole 桌面会话：浏览器 WS ↔ guacd TCP 双跳管道。
// 职责：票据校验（握手时）+ 出口策略 + 审计 + 双向字节管道。
// 不解析 Guacamole 指令内容（握手阶段构造 connect/size 除外）。
type GuacamoleSession struct {
	ID       string
	UserID   string
	Username string
	Protocol string // rdp / vnc
	AgentID  string
	Host     string
	Port     int
	ClientWS *websocket.Conn
	guacd    net.Conn
	done     chan struct{}
}

// handleGuacamoleWebSocket 浏览器 ↔ server 段：WS 文本帧即 Guacamole 协议指令。
// ticket 经 Sec-WebSocket-Protocol 传递（与 terminal/desktop 同款）。
func (s *Server) handleGuacamoleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 1. 票据：Sec-WebSocket-Protocol[0] → ValidateTicket（一次性消费）
	// 2. 权限：terminal:write + matchRemoteEgress(agentID, host, port)
	// 3. 拨 guacd TCP（127.0.0.1:4822，超时 10s）
	// 4. 升级 WS（Subprotocols 回显 ticketID）
	// 5. 构造握手指令发 guacd：size / audio / video / connect
	//    connect 参数：protocol=rdp|vnc, hostname, port, username, password…
	//    （凭据来自 ticket.Params，同 handleTerminalWebSocket 的 proxy_new 下发）
	// 6. auditRemoteStart（Protocol: rdp|vnc, Session: sessionID）
	// 7. 双向管道：
	//      WS → guacd：conn.Write(ws 文本帧内容)
	//      guacd → WS：bufio.Scanner 按 Guacamole 指令边界切分 → ws.WriteMessage
	// 8. 任一侧断开 → sendGuacamoleClose → auditRemoteEnd（含 duration）
}

// Guacamole 指令是「逗号分隔长度前缀 + 分号结尾」的文本协议，
// 例如：4.size,1.1,4.1024,3.768;
// 管道转发用 bufio.Reader 流式透传即可，无需解析。
```

**为什么 Go 侧不解析指令内容**（除握手构造外）：Guacamole 协议是
guacamole-common-js 与 guacd 之间的私有协议，版本随 Guacamole 发行版演进。
Go 网关如果解析指令（如为了「审计键入内容」），就要跟着协议版本改，且会
重新引入「输入含密码」的泄漏面（见 `recording-design.md` D4）。**网关只做
字节管道**，协议演进由 Guacamole 两端自己对齐。

### 3. 前端 guacamole-common-js 接入骨架

```ts
// web/src/components/GuacamoleModal/index.tsx
import Guacamole from 'guacamole-common-js'

/**
 * Guacamole 桌面 Modal：guacamole-common-js 承担渲染/输入/剪贴板，
 * 本组件只负责「申请票据 → 建隧道 → 生命周期」。
 */
const GuacamoleModal: React.FC<Props> = ({ visible, onClose, agentId, host, port, protocol, username, password }) => {
  const clientRef = useRef<Guacamole.Client | null>(null)
  const displayRef = useRef<HTMLDivElement>(null)

  const connect = useCallback(async () => {
    // 1. 申请票据（复用 createRemoteTicket；401 刷新链路已内置在 remoteClient）
    const { ticket } = await createRemoteTicket({
      agentId, host, port,
      protocol: protocol as RemoteProtocol,
      username, password,
    })

    // 2. 隧道：WS URL + 票据作子协议（与 TerminalModal 同款握手）
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const tunnel = new Guacamole.WebSocketTunnel(
      `${wsProtocol}//${window.location.host}/api/remote/guacamole`,
    )
    // guacamole-common-js 的 WebSocketTunnel 自己处理 Guacamole 协议
    // 的 chunk/base64 编解码——不要在这里自由发挥（见风险章节）
    ;(tunnel as unknown as { wsProtocols?: string[] }).wsProtocols = [ticket]

    // 3. 客户端：Display/Keyboard/Mouse 全由 common-js 承担
    const client = new Guacamole.Client(tunnel)
    clientRef.current = client

    const display = client.getDisplay()
    displayRef.current?.appendChild(display.getElement())

    // 触控 → 鼠标：common-js 的 Guacamole.Mouse 已处理 touch 事件
    const mouse = new Guacamole.Mouse(display.getElement())
    mouse.onmousedown = mouse.onmouseup = mouse.onmousemove = (state) => {
      client.sendMouseState(state)
    }
    const keyboard = new Guacamole.Keyboard(document)
    keyboard.onkeydown = (keysym) => client.sendKeyEvent(1, keysym)
    keyboard.onkeyup = (keysym) => client.sendKeyEvent(0, keysym)

    // 4. 剪贴板（纯文本）
    client.onclipboard = (streams, mimetype) => {
      if (mimetype !== 'text/plain') return
      const reader = new Guacamole.StringReader(streams[0])
      let text = ''
      reader.ondata = (chunk) => { text += chunk }
      reader.onend = () => navigator.clipboard.writeText(text)
    }

    client.connect() // 参数已含在服务端 connect 指令里（ticket 携带）
  }, [agentId, host, port, protocol, username, password])

  useEffect(() => () => clientRef.current?.disconnect(), [])
  // ...
}
```

**为什么参数（凭据）由服务端放进 `connect` 指令而非前端**：
guacamole-common-js 的 `client.connect(data)` 可传参数字符串，服务端也可
在 `connect` 指令里带参数。**选服务端**是因为凭据已在 ticket.Params 里
（`handleTicketCreate` 存储），前端不需要经手——减少一次凭据在浏览器
内存/网络上的暴露面，且与 `handleTerminalWebSocket` 的
`proxy_new` 下发凭据同款（前端传 ticket，agent 拿到的是已解析的凭据）。

## 风险与边界

### H.264/帧率：按办公场景验收，不追求媒体级

Guacamole 1.5.x 的 RDP 驱动支持 GFX/AVC444（H.264）硬件编码路径，
VNC 侧无 H.264（RFB 协议本身是像素/伪像素编码）。**验收标准定为办公
场景**：文本编辑器滚动流畅、窗口拖动不明显掉帧、输入延迟可接受（主观
「不难受」），**不以帧率数值/码率为目标**。

理由：Cockpit 的远控场景是「远程运维/应急处理」（改配置、看日志、重启
服务），不是看视频/3D。媒体级验收（如 30fps 稳定、<100ms 输入延迟）会把
工作量引向调优 guacd 的 FreeRDP 编解码参数，而这些参数的收益在办公场景
下感知弱。阶段 1 若办公场景流畅即可收；若特定目标机器卡顿，先查
guacd 的 `disable-audio`/`color-depth`/`resize-method` 参数，再考虑
是否为网络问题。

**H.264 是否启用不由我们配**：FreeRDP 的 AVC/H.264 需要目标 Windows 支持
（RemoteFX/GFX 或 AVC444），guacd 会自动协商。我们不强制开启也不禁止
——协商结果即事实。

### 音频放阶段二

Guacamole 支持音频（RDP 的 `rdpsnd`、VNC 无音频），但：

1. 浏览器音频播放需用户手势解锁（iOS Safari 尤其严格），静音/音量控制
   要自己接 UI；
2. guacd 音频编解码（如 RDP 的 Opus/MP3）与浏览器 `Guacamole.Audio`
   的播放链路要对齐；
3. 运维场景下桌面音频价值低（不像远程影音）。

**收益/成本比低**，故明确推迟。架构上不留死角：`connect` 指令的 `audio`
参数本就是可选列表，阶段二加上即可，不影响阶段一的协议路径。

### 隧道二进制帧 chunk/base64 规则照抄官方 Tunnel 实现

**这是最容易自作聪明翻车的一处**，故单独强调：

Guacamole 协议的指令流里，二进制数据（位图、音频帧、剪贴板文件）不是直接
塞进指令的，而是走「**流**」机制：`blob` 指令携带 base64 chunk（带流索引），
`end` 指令收尾。`guacamole-common-js` 的 `Guacamole.Tunnel` 实现里有
`sendMessage` 的 opcode 组装（长度前缀格式 `length.opcode,arg1,arg2...;`）
和 base64 chunk 的切分规则。

**规则**：

1. Go 网关**不重新实现** chunk/base64 编解码——网关只做 WS 文本帧 ↔
   guacd TCP 流的字节管道，两侧的编解码各自由 `guacamole-common-js`
   与 guacd 完成；
2. 若将来必须在 Go 侧构造指令（如握手阶段的 `connect`），**照抄**
   [guacamole-common-js 的 `Guacamole.Tunnel.sendMessage`](https://github.com/apache/guacamole-client-commonjs)
   的长度前缀格式与 [`guac-hawk-bit`](https://github.com/apache/guacamole-server) 的
   指令解析，**不要自由发挥**——协议是私有的，格式错一处就是静默断流，
   调试成本极高（指令流不透明）；
3. 版本对齐：guacamole-common-js 的版本与 guacd 的版本要匹配到同一
   Guacamole 发行版（1.5.x 配 1.5.x）。协议是私有的，跨版本不保证兼容。

这条风险的根因是「私有协议 + 无 schema」：Guacamole 协议没有公开的 IDL/
schema，格式约定只存在于两端实现代码里。照抄比自己写快且对。

### guacd session recording：白捡的审计回放

**这是集成 Guacamole 的意外红利**，值得单独说：

guacd 原生支持把会话录成 **`.guac` 格式**（Guacamole session recording），
在 `connect` 指令里传 `recording-path` + `recording-name` 参数即自动录制，
无需我们解析/转发任何像素数据。录出来的 `.guac` 文件可以用官方的
`guacplay`（guacamole-server 自带工具）或 `guacenc` 回放/转码。

对照 Cockpit 现状：终端侧已有 asciinema v2 录制（`recording-design.md`，
M1 落地：`recording.go` + `TerminalRecording` 表 + `/recordings` 回放页），
但**桌面侧（RDP/VNC）录制明确列为「不做」**（见 recording-design.md
D1/不做节：「VNC/RDP 像素流录制（WebM/图片序列，体积大回放重）」）。
当时否掉的原因是**自研录制成本高**——像素流录制要自己写格式、自己写
回放器、体积大。

Guacamole 路线下这个成本**几乎为零**：guacd 写 `.guac` 文件，我们只需：

1. `connect` 指令加 `recording-path`（指向 guacd 容器卷）+
   `recording-name`（会话 ID）两个参数；
2. 会话结束后把 `.guac` 文件从 guacd 卷收集到 Cockpit 的
   `data/recordings/` 目录（与 asciinema `.cast` 同目录，`recording.go`
   的 retention/清理逻辑直接复用）；
3. 回放：阶段二做 Web 回放器（解析 `.guac` 格式），或先用官方 `guacplay`
   离线回放（CLI 工具）。

**这直接补强了 `recording-design.md` 的「不做」缺口**，且与既有
`recording.enabled` / `recording.retention_days` 设置项语义对齐。阶段一
先落「录制落盘」（guacd 侧白捡），Web 回放器放阶段二。

代价：`.guac` 是 Guacamole 私有格式（同风险「私有协议」），脱离 Guacamole
生态不能回放——但 asciinema `.cast` 是开放格式可脱离 Cockpit 回放，两者
互补（终端开放格式、桌面 Guacamole 格式）。

## 不做（后续版本）

- **音频**（见风险章节，阶段二）；
- **文件传输**：Guacamole 的 `Guacamole.Filesystem` + RDP 驱动器重定向 /
  VNC 的 SFTP 叠加。与 Cockpit 的 `file-manager-design.md` 路线重叠，
  先用文件管理器（agent 侧 RPC）覆盖文件进出，桌面内文件传输按需再议；
- **剪贴板富格式**（RTF/HTML）：阶段一纯文本；
- **Web `.guac` 回放器**：阶段一用官方 `guacplay` 离线回放，Web 回放器
  阶段二（可复用 `/recordings` 页面的播放器骨架）；
- **SSH/telnet 走 Guacamole**：SSH 已自研打通（`internal/proxy/ssh_session.go`，
  agent 侧 `crypto/ssh` + PTY + TOFU host key），xterm.js 体验优于
  Guacamole 的终端仿真；telnet 同理。**桌面协议（RDP/VNC）走 Guacamole，
  字符终端（SSH/telnet）走 xterm.js**——两者不互相绑架；
- **替换 agent 侧 RDP（grdp）**：阶段一 Guacamole 路线**新增**到 server 侧，
  agent 侧 grdp 保留为「无 guacd 部署时的降级路径」或按阶段一验收结果
  决定废弃。两套并存期不宜过长，阶段一验收后定夺。

## 里程碑

**阶段 1：连得上、操作得了**（本设计的验收范围）

1. guacd docker-compose 落地（`deploy/guacd/docker-compose.yml`）；
2. Go WS 网关反代（`internal/server/api_guacamole.go`）+ 票据/审计/出口策略；
3. web `GuacamoleModal`（guacamole-common-js）替换 `DesktopModal` 的 RDP/
   VNC 入口（或并列入口灰度）；
4. 验收：浏览器 RDP 连 Windows 专业版/Server（NLA）、VNC 连 Linux、
   Win11 Home VNC 兜底、iPad Safari 触控可用。

**阶段 2：补强**

- 音频；
- `.guac` Web 回放器 + 与 `/recordings` 页面整合；
- 文件传输（按需）；
- grdp 路线去留定夺。

## 参考

- [Apache Guacamole 架构](https://guacamole.apache.org/doc/gug/architecture.html)
- [guacamole-common-js API](https://guacamole.apache.org/doc/gug/guacamole-common-js.html)
- [Guacamole 协议（指令格式）](https://guacamole.apache.org/doc/gug/protocol-reference.html)
- [guacd 参数（recording-path 等）](https://guacamole.apache.org/doc/gug/configuring-guacamole.html)
- 项目内：`docs/guide/recording-design.md`（终端录制，桌面录制缺口）、
  `docs/guide/reference-projects.md`（Guacamole 参考记录）、
  `internal/server/ticket.go`（票据语义）、`internal/server/remote_audit.go`
  （远控审计）、`web/src/services/api.ts`（401 刷新）
