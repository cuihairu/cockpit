# Guacamole RDP 音频真机验收（D4）

> 任务：todo.md:46 D4「RDP 音频 mimetype 与 L8/L16 编解码确认，静音按钮行为联动验证」。
> 决策依据：`docs/review/2026-10-30-decisions.md` 第 12 项（分段确认音源、协商格式、传输、
> 播放、静音，才能知道该修哪一层）。
> 验收面：`docs/remote-desktop-guacamole-design.md` §音频风险（L8/L16 原始 PCM、
> RawAudioPlayer、AudioContext 静音）。
> 日期：2026-11-01。目标机：openclaw（win-worker，10.240.0.3，Windows Server 2022）。
> 结论：**PASS（附一处网关缺陷，已修复）**。

## 1. 结论摘要

| 验收点 | 结果 | 证据 |
| --- | --- | --- |
| 1. RDP 可连接（经网关） | PASS | ready 到达，Security mode Negotiate，RDPDR 登录，多轮稳定 |
| 2. 音频源可播放（会话内） | PASS | 会话 3 内 SoundPlayer 循环音，`LOOP_PROC` 确认在会话 3 |
| 3. 协商格式（mimetype） | PASS | guacd 日志「Accepted format: 16-bit PCM with 2 channels at 44100 Hz」→ **audio/L16** |
| 4. L8/L16 编解码 | 部分 | L16 全程可用；L8 仅 RawAudioPlayer 支持，guacd 1.5.5 对 16-bit PCM 源只产 L16 |
| 5. 传输（音频流到客户端） | **发现缺陷→已修复** | 网关空 `audio` 声明导致 guacd 一条 audio 指令都不发；声明 L8+L16 后音频流+5MB PCM 到达 |
| 6. 播放（L16 可播放） | PASS | 5,013,504 字节 PCM 经网关到达，RawAudioPlayer 支持 audio/L16 |
| 7. 静音按钮联动 | PASS（源码+单测） | AudioContext 单例 suspend/resume；index.test.tsx:405 覆盖 |

**关键发现**：`internal/server/api_guacamole.go` 的 guacd 握手里 `guacEncode("audio")` **空声明**
（阶段二延后遗留）是「桌面无声音 / 静音按钮无效」的根因——guacd 只在客户端声明了支持的
mimetype 时才推 `audio` 指令+blob 流；空声明下 guacd 不创建音频流，浏览器永远收不到
`audio`。这与设计文档「音频经字节管道零网关改动透传」的表述冲突：**管道确实透传，但
握手代客户端声明的能力集是空的**。已改为声明 `audio/L8,audio/L16`（与
guacamole-common-js `RawAudioPlayer.getSupportedTypes()` 一致），单测同步更新。

## 2. 环境与前提

- 网关：`.acceptance/guac/instance-audio`（server :19990 + agent `guac-acc-agent`），
  guacd 1.5.5 容器（cockpit-acc-guacd，host 网络 :4822）。
- RDP 目标：openclaw（10.240.0.3:3389），本地用户 `guactest`（Remote Desktop Users）。
- 音源：会话 3 内 `(New-Object System.Media.SoundPlayer 'C:\Windows\Media\notify.wav').PlayLooping()`，
  经 `Register-ScheduledTask -LogonType Interactive` 在交互会话运行（免密码），
  `C:\temp\audioloop.ps1`。
- 凭据：RDP 口令只存 `.acceptance/guac/rdp-guactest.pass`（gitignored），经
  `GUAC_RDP_PASS` 环境变量注入，不进日志/源码。

## 3. 分段验收（决策 12 的「才知该修哪一层」）

### 3.1 音源
会话 3 是 openclaw 唯一的 RDP 渲染会话；该会话所有状态下唯一渲染端点 = 远程音频
（Remote Audio）。循环音进程在会话 3（`LOOP_PROC pid=… sess=3`，TASK_STATE=Running）。
**注意时序**：`SoundPlayer.PlayLooping()` 在调用时刻绑定会话默认音频端点。若在 RDP
连接建立**之前**启动循环音，声音进宿主 Clouddesk 驱动而非重定向的 Remote Audio，
guacd 收不到 PCM（实测：先启音再连 RDP → audio 指令到但 blob 为 0）。**必须在 RDP
会话活跃后再启动循环音**，才会绑定 Remote Audio 端点（见 3.5 复现步骤）。

### 3.2 协商格式（mimetype）
guacd 日志（每次 RDP 会话建立时）：
```
guacd[N]: INFO:  Accepted format: 16-bit PCM with 2 channels at 44100 Hz
```
即 guacd 接受 RDP 服务端（rdpsnd）的 16-bit PCM 44100Hz 双声道。guacd 对客户端推的
audio 指令 mimetype 为 `audio/L16`。**结论：协商 mimetype = audio/L16**，非 Opus/MP3。

### 3.3 传输（关键发现）
**网关缺陷**：`internal/server/api_guacamole.go:533`（修复前在 :527）原为
```go
guacEncode("audio") // 空 = 不启用音频（阶段二）
```
Guacamole 协议中 `audio` 指令的**参数**是客户端支持的 mimetype 列表；guacd 只在参数非空时
创建音频流并推送 `audio,<idx>,<mimetype>;` + `blob,<idx>,<b64>;`。空参数 = 客户端不支持
任何音频 = guacd 不推流。

**A/B 隔离（直连 guacd，绕开网关，复刻网关握手）**：

| 客户端 audio 声明 | guacd 行为 |
| --- | --- |
| 空（原网关） | `ready` 后**无** audio 指令，无音频流（指令流仅 ready/img/sync） |
| `audio/L16` | `audio [1 audio/L16]` 指令到达，img/copy/sync 正常 |

隔离探针 `.acceptance/guac/directprobe/main.go`（一次性，不随仓提交）复刻
`guacConnectArgs` 的逐位对齐（含 `VERSION_1_5_0` 回显）；此前因把 VERSION 当 opcode
拆掉导致 connect 参数少一位被 guacd 拒收（"Client did not return the expected number
of arguments"），修正后连接成功。

**修复**：网关握手改为
```go
guacEncode("audio", "audio/L8", "audio/L16")
```
与 guacamole-common-js `RawAudioPlayer.getSupportedTypes()` 一致。单测
`api_guacamole_test.go:452` 断言同步更新为 `5.audio,8.audio/L8,9.audio/L16;`。

### 3.4 传输修复后（经网关全链路）
修复后经网关重测（RDP 连接活跃后再启循环音）：
```
== audio 指令: idx=1 mimetype=1 | audio/L16 ==
== 音频流 1 blob 字节=5013504（全部流 blob 合计=5072563）==
== 指令流统计: copy=964 ... img=41 blob=963 end=41 ... audio=1 sync=762 ==
== 结论：mimetype=audio/L16 dataBytes=5013504 可播放=true ==
```
**5,013,504 字节 PCM 经网关到达客户端**（音频流 idx=1）。按 44100Hz×2ch×2B ≈
176.4KB/s 折算，5.01MB ≈ 28 秒连续音频，与 30s 采集窗口吻合——是连续实流而非零星
噪声。指令流统计 blob=963 = 音频流 chunk（约 900+，guacd 默认 ~6KB/chunk）+ 图像流
chunk（img=41）。同轮 guacd 日志确认 `Accepted format: 16-bit PCM with 2 channels at
44100 Hz`。

### 3.5 L8/L16 编解码行为
- **L16**：全程可用。16-bit PCM 44100Hz 2ch → guacd 推 `audio/L16`，RawAudioPlayer 播放。
- **L8**：RawAudioPlayer（`isSupportedType`）声明支持 `audio/L8`；但 guacd 1.5.5 的 rdpsnd
  对 16-bit PCM 源**只输出 audio/L16**，不把 16-bit 下采样为 L8。直连 A/B：仅声明
  `audio/L8` → guacd `Accepted format` 照旧（说明音频已捕获），但**不推 audio 指令**
  （客户端没声明它能产的 L16）。结论：真实 Windows RDP 源的可用编解码为 **L16**；L8
  是浏览器侧支持面，当前 guacd 不会选用。

### 3.6 播放
`rawPlayable()`（mimetype 去 `;rate=,channels=` 后为 `audio/L8|audio/L16`）判定通过；
`guacamole-common-js` `RawAudioPlayer` 支持 `audio/L16;rate=44100,channels=2`。播放链
（AudioPlayer.getInstance → RawAudioPlayer → AudioContextFactory 单例）与客户端
`GuacamoleModal` 一致。

### 3.7 静音按钮联动
`web/src/components/GuacamoleModal/index.tsx:378-391`：
```ts
const ctx = Guacamole.AudioContextFactory?.getAudioContext()
if (muted) { void ctx.resume() } else { void ctx.suspend() }
```
即静音按钮切换 → 全局 AudioContext 单例 suspend/resume（RawAudioPlayer 内部直连该
单例），一次性全局静音/恢复。单测 `index.test.tsx:405` 覆盖 suspend→resume 切换，
`:547` 覆盖 AudioContext 不可用（jsdom/旧浏览器）时静音不抛错。

## 4. 遗留 / 待办

- **浏览器真实点击静音按钮**未做 CDP 级验证（无头浏览器对 AudioContext 无实际输出设备，
  行为已由单测+源码闭环）。若需端到端 UI 点击验证，可用 `scripts/acceptance/guac/cdp/`。
- 循环音 `SoundPlayer.PlayLooping()` 的端点绑定时序（3.1）是**测试技巧**而非产品缺陷；
  真实桌面用户的声音天然从 RDP 重定向端点流出。
- 网关声明 `audio/L8,audio/L16` 是**常开**：所有 RDP 会话都会带音频能力声明。当前
  guacd 仅产 L16，无额外开销；若未来 guacd 支持 Opus/MP3，可在此按需扩展。

## 5. 复现步骤（供再验）

1. 起实例网关+agent+guacd 容器（scripts/acceptance/guac/run-server.sh + setup-env.sh）。
2. 建立一次 RDP 连接（让会话 3 活跃），随后在会话 3 启动循环音
   （`Start-ScheduledTask GuacAudioLoop`，脚本 `C:\temp\audioloop.ps1`）。
3. 用 `.acceptance/guac/audioprobe` 建票据连 WS：等 `audio` 指令 → 采集 blob 窗口 →
   期望 `mimetype=audio/L16`、`dataBytes>0`、`可播放=true`。

## 6. 涉及文件

- `internal/server/api_guacamole.go:533` — audio 声明修复（本次；原 :527）
- `internal/server/api_guacamole_test.go:452` — 断言同步（本次）
- `docs/remote-desktop-guacamole-design.md:286-294` — 验收面（L8/L16、字节管道）
- `.acceptance/guac/audioprobe/main.go`、`.acceptance/guac/directprobe/main.go` — 探针（一次性，不随仓提交）
- `docs/review/2026-10-30-decisions.md:221-236` — 决策 12
