# Windows Agent Packaging Design (setup.exe + service integration + full icon chain)

> Language: [中文](./windows-agent-design.md) ｜ **English**

> Status: landed. The artifact `cockpit-agent-setup-<version>.exe` goes into the nightly release (including `.sha256`); the real-Windows walkthrough happens in the nightly **Build Agent Installer** job.
> Requirement origin: user urging (B1, see the repo's `BUGS.md`).

## Goals and shape

The agent delivery on Windows must satisfy three things — without any one of them it's a "half-finished product":

1. **A double-click-to-use installer**: install directory, Start menu, desktop shortcut, uninstaller
2. **Ready on install (auto-start on boot)**: register as a Windows service, Automatic + auto-restart on crash
3. **Has an icon**: the exe resource section, shortcuts, installer, and Control Panel uninstall entry all use the same icon

Inno Setup was chosen (over MSI): scriptable, parameterized silent installs, custom install page (Server address input), and the Windows service task handled by the agent's own subcommand. The build script is at
[`packaging/windows/agent.iss`](https://github.com/cuihairu/cockpit/blob/main/packaging/windows/agent.iss).

## Full icon chain (SVG logo → ico → exe resource)

One `scripts/gen-windows-icon.sh` command regenerates the whole chain; all artifacts are committed to the repo (the build does not depend on a rasterization toolchain inside the CI):

```text
web/public/logo.svg
  └─ magick 7 multi-size rasterization (16/24/32/48/64/128/256) → packaging/windows/agent.ico
       ├─ Inno SetupIconFile / UninstallDisplayIcon → installer + uninstall entry icon
       ├─ [Icons] shortcut icon (references the icon embedded in the exe)
       └─ rsrc → cmd/cockpit-agent/rsrc_windows_{amd64,arm64}.syso
            └─ go build windows/amd64|arm64 auto-links it into the PE .rsrc section
```

Key constraints:

- A syso must be named `<name>_<GOOS>_<GOARCH>.syso` for `go build` to pick it up, and it **only affects the windows target** — linux/darwin builds ignore it entirely, so CI needs no extra switch.
- The ico is a multi-size PNG container (ImageMagick assembles PNGs directly, no BMP encoding, lossless transparent edges).
- Locally, `debug/pe` can assert the RT_ICON/RT_GROUP_ICON directories in the `.rsrc` section; visual-level verification (`ExtractAssociatedIcon`) is left to the nightly real-Windows walkthrough.

## Service integration (agent's built-in SCM support)

Service registration, start, stop, and removal are all handled by the agent's own subcommands:

```text
cockpit-agent service install [-server ...]   # register/upgrade (Automatic + failure actions) + start
cockpit-agent service uninstall              # stop and unregister
cockpit-agent service start|stop|status
cockpit-agent service run                     # SCM invocation entry (do not run manually)
```

Service name/display name/description/failure restart policy (5s/10s/20s, count reset after 24h) are byte-for-byte identical to the existing `install.ps1` registration — the two install paths recognize each other and overwrite each other on upgrade.

### Why we must implement the service state ourselves

Registering a **console program** directly as a service on Windows (with `New-Service` pointing at `cockpit-agent start`) is guaranteed to break: the process never calls `StartServiceCtrlDispatcher`, so the SCM waits 30 seconds, judges it unresponsive, and kills it — the service can never start. This is the pre-existing shape of `install.ps1`, and this design fixes it as well:

- `handleStart` does SCM-context detection at the top (`svc.IsWindowsService`): in a service context it automatically switches into service mode, so existing directly-attached services self-heal after their next restart with no user intervention.
- In console mode that detection is always false, and behavior is completely unchanged (existing signal-path semantics untouched).

### Exit semantics: in-process supervision, not SCM restart allowance

When the agent cannot reach the Server, `Start()` returns an error. If we followed the conventional approach of "exit the process and let the SCM restart it", the SCM's failure-restart allowance is only 3 times (5s/10s/20s), and network not ready / DNS not effective / the server temporarily offline at boot would completely exhaust the service. So the service mode reconnects **in-process**:

- Connection failure → log, retry after 15s, service stays `Running` (the state is real: the process is indeed alive)
- Crash/hard kill → still handled by the failure actions registered at registration time
- SCM `Stop`/`Shutdown` → close the stop channel, and `StartCmd.RunService` triggers `a.Stop()` for a graceful exit (connection disconnect; registry residue cleanup handled internally by the agent)

The difference between `StartCmd.RunService(stop)` and `Run()` lies only in where the exit signal comes from: the systemd/container paths still go through the signal branch (`Run`), and the service path goes through the stop-channel branch.

## Installer structure

| Section | Content |
|---|---|
| `[Setup]` | Fixed AppId (reinstall = upgrade merge), `{autopf}\Cockpit Agent`, `PrivilegesRequired=admin`, `x64compatible` 64-bit mode, `{param:server}` (`/SERVER=` silent switch, an Inno native constant), `CloseApplications`+`RestartApplications` (stop first, then start, on upgrade) |
| `[Tasks]` | `service` (register service + auto-start on boot, checked by default), `desktopicon` (desktop shortcut) |
| `[Code]` | Custom input page collecting the Server WebSocket address; skipped when the service task is unchecked; silent install takes it from `/SERVER=`; if both are empty, fail explicitly (do not let `service install` fail after validating an empty `-server`) |
| `[Files]` / `[Icons]` | Payload is the CI-staged `cockpit-agent.exe`; the Start menu includes an uninstall entry |
| `[Run]` | `service install -server ...` (`Tasks: service`) |
| `[UninstallRun]` | Run `service uninstall` before files are deleted (stop + remove) |
| `[UninstallDelete]` | Clean up `%ProgramData%\CockpitAgent` (service logs + `config.env`, not inside `{app}`) |

## Relationship with `install.ps1`

`install.ps1` (no GUI / fleet installs) aligns its capabilities with the installer; service registration is changed to call `cockpit-agent service install`: it no longer assembles `New-Service` + `sc failure` itself, nor rebuilds after `Remove-Service` (the agent internally follows the upgrade path: stop service → refresh `BinaryPathName` → restart). The upgrade script and the upgrade installer can overwrite each other's service entry.

## Acceptance (nightly real-Windows walkthrough)

On `windows-latest`, the `Build Agent Installer` job: silent install → assert the service is `Automatic` + `Running` → extract the icon with `ExtractAssociatedIcon` for evidence → Start menu/desktop shortcuts exist → desktop screenshot archived → silent uninstall → assert the exe/service/shortcuts are all cleaned up, printing `WALKTHROUGH_OK`. The icon PNG and desktop screenshot are uploaded as artifacts (`walkthrough-icon.png` / `walkthrough-desktop.png`), and the installer binary itself plus `.sha256` go into the release assets.

The part coverable on linux from the code side: the non-Windows platform-guidance branch of the `service` subcommand (`cmd/cockpit-agent/cov_service_test.go`), and the parameter-validation passthrough of `RunService` plus the "exit gracefully when stop precedes start" semantics (`internal/agent/cov_runservice_test.go`); the SCM state machine proper can only be covered by the real-Windows walkthrough.
