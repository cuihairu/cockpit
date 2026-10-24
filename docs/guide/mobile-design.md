# Mobile Plan: Flutter (iOS + Android)


> 2026-09-22. The user specified Flutter as the tech stack, covering both platforms. This document defines the rationale for the choice, the boundary with the existing server, the M1 minimum viable scope, and subsequent milestones.

## Background

The Cockpit web UI is desktop-first (antd data-dense tables, multi-column workbench), and the mobile browser experience is poor. Yet the most typical mobile scenarios for a personal infrastructure console are exactly:

- **Checking status while away**: is the agent online, are there alerts, are certificates/domains about to expire;
- **Receiving alerts**: disk SMART anomalies, agent offline, backup failures must be pushable to the phone;
- **A few critical operations**: restarting a container/service, acknowledging an alert — not full ops work.

The mobile feature surface is naturally a narrow subset of the web; the design revolves around "thin client + reuse all existing infrastructure".

## Decisions

- **D1 Tech choice: Flutter** (user-specified). Alternatives compared: PWA/responsive retrofit of web has the lowest cost, but antd tables perform poorly on small screens, there is no push channel, and no native feel — not up to standard as a mobile solution; React Native shares the TS ecosystem with web, but the cost of maintaining a second frontend for a personal project remains, and native components like the terminal emulator (xterm.dart) are more mature on the Flutter side. The price of Flutter is introducing a third language stack (Go / TS / Dart), traded against the narrow M1 feature surface (mostly read-only + a few operations) for a controllable long-term maintenance surface.

- **D2 No separate BFF, direct connection to existing `/api/*`**: the web's `services/api.ts` has already converged 196 endpoint calls; authentication (JWT + TOTP two-step), agents, alerts, Docker, and audit are all ready-made. The mobile side is just a thin client; the server side only adds endpoints when there is a genuine aggregation need (M1 expects zero additions). API alignment discipline is the same as web: **fields are authoritative per the Go model json tags**, not from memory or copying each other.

- **D3 Alert push reuses the ntfy channel, no FCM/APNs**: server notifications already have four channels — herald / ntfy / webhook / telegram (config.go) — and the official ntfy Android/iOS app is a ready-made push endpoint: the server configures one ntfy channel and gets phone push, saving the accounts, signing, and backend pipeline of FCM/APNs. Inside the app, alert-list polling serves as a fallback. Self-hosted ntfy has limitations on instant iOS delivery (background polling interval); documented, not a blocker.

- **D4 Authentication fully reused**: `POST /api/auth/login` → when `requires_totp=true`, use `tmp_token` with `/api/auth/totp/verify` to exchange for the formal JWT (the same two-step flow as the web's `LoginResponse`); tokens are stored in `flutter_secure_storage` (iOS Keychain / Android Keystore), never in SharedPreferences; `/api/auth/refresh` renewal is done automatically by a dio interceptor (401 → refresh → replay, on failure log out back to the login page).

- **D5 Self-signed HTTPS explicit switch**: the server is often deployed on an intranet or behind a self-signed certificate. Strict validation by default; the settings page provides an "allow self-signed certificates" switch (`badCertificateCallback` lets it through), with a red-text warning about man-in-the-middle risk when enabled. No certificate import (over-engineering for a personal scenario).

- **D6 State management Riverpod + networking dio**: Riverpod is compile-time safe and mainstream in the community; dio's interceptor mechanism directly carries D4's refresh-replay and D5's certificate switch. Data polling is the mainstay (M1 does not connect the WebSocket metrics stream); polling is battery-friendly and simple to implement for dashboard scenarios.

- **D7 Terminal / desktop deferred**: terminal in M2 with xterm.dart + the existing remote ticket WebSocket; VNC/desktop viewing has a weak ecosystem on the Flutter side (no noVNC equivalent), listed as an exploration item without commitment (closed as "won't do" on 2026-10-07; evaluation basis in the M3 section).

- **D8 CI as a standalone `mobile.yml`**: triggered by `paths: mobile/**`, with `flutter analyze` + `flutter test` + APK build (`subosito/flutter-action`). iOS builds do not enter CI (requires a macOS runner + signing account); to be revisited at the local/real-device stage. (2026-10-07 update: per the third-edition policy, add a `build-ios` job — the macOS runner runs `flutter build ios --no-codesign --debug` to verify "it compiles" without a signing account; signing and real-device installation remain outside CI scope.)

## Milestones

**M1 minimum viable** (core loop for phone scenarios):

| Page | Reused endpoints (cf. `web/src/services/api.ts`) |
| --- | --- |
| Onboarding: server URL config + connection test | `GET /health` |
| Login (incl. TOTP two-step) | `/api/auth/login`, `/api/auth/totp/verify` |
| Dashboard: agent online/offline, unread alerts, certificate/domain expiry | `/api/agents`, `/api/alerts`, resource lists |
| Agent list + detail metrics | `/api/agents`, metrics endpoints |
| Docker container list + start/stop/restart | Docker container endpoints |
| Alert list + mark all read | `/api/alerts`, `PUT /api/alerts/read-all` |
| Audit list | `/api/admin/audit/logs` |
| Settings: server URL, self-signed switch, logout | — |

Dark theme follows the system; portrait single-column layout on phones.

**M2** (completed 2026-09-22): resources page with three tabs (domain/certificate expiry, backup task status and manual trigger), Cron read-only view (external entries shown verbatim, write operations left to desktop), file browsing (read-only: directory navigation + entry details), SSH terminal (tickets + WS subprotocol + xterm.dart), biometric lock (local_auth startup gate).

Deviations from the original plan: the reverse proxy (nginx/traefik) view, originally deferred to M3 evaluation, was closed in two batches via todo M4 (read-only three states, 2026-09-23) and M5 (create/edit/delete write operations, a9c095f4); the backup view landed as the third tab of the resources page rather than a standalone page; Cron and files are explicitly **read-only** on mobile — add/delete/edit operations stay on desktop, consistent with the "view on phone, change on desktop" positioning.

**M3** (closed 2026-10-07): Stacks deployment operations have landed (2026-09-29, todo M8) — stack list + up/down/restart/pull async operations (taskId polled every 2s) + compose/.env read-only preview; deviation: compose editing, stack deletion, and deployment history stay on desktop. The SMART/NAS/networking observation read-only pages have landed (2026-10-01, b5d15a6, closed with 20 widget tests) — per-agent action single entry (not in the bottom tab, same as reverse proxy/Stacks), `smart_page` (health three states and anomalous-disk-pinned ordering aligned with the web Disk page), `nas_page` (pool/mount/share three sections + over-threshold highlighting), `overlay_page` (runtime-state snapshots of the four tools); inspection config and write operations stay on desktop.
**VNC/desktop viewing exploration item closed (2026-10-07, decision = won't do)**: when D7 was initiated it was already noted that "the Flutter-side ecosystem is weak, no commitment"; closing evaluation — (1) route a "webview embedding noVNC" requires exposing a bare VNC port, conflicting with cockpit's existing architecture of "VNC uniformly via guacd + /api/remote/guacamole tunnel"; (2) route b "Dart implementation of a Guacamole protocol client" (instruction parsing + CustomPainter rendering + keyboard/mouse event encoding) amounts to rebuilding guacamole-js, violating the "don't rebuild the underlying layer" boundary discipline; (3) positioning judgment: emergency operations are already covered by the M2 SSH terminal, and watching a desktop on a small phone screen + touch mapping is low value. Trigger conditions (re-initiate if either is met): a clear mobile desktop-viewing need emerges, or a actively-maintained noVNC equivalent appears on the Flutter side. Desktop viewing is handled by the web-side Guacamole three protocols (RDP/VNC/SSH); the mobile side does not duplicate it.

## Directory structure

```
mobile/                  # Flutter project, parallel to web/
  lib/
    main.dart
    api/                 # dio client, interceptors, endpoint wrappers
    models/              # hand-written models aligned with Go json tags
    state/               # Riverpod providers (auth, agents, alerts…)
    pages/               # one directory per page, named in alignment with web pages
    widgets/
  test/                  # unit tests: model alignment, interceptors, page widget tests
```

## Prerequisites and acceptance

- The Flutter SDK (stable) must be installed locally. This machine is Linux: **Android APK can be built locally; iOS cannot be built locally** (requires macOS + Xcode) — iOS compile verification is closed out to the CI macOS runner (2026-10-07, `mobile.yml` build-ios unsigned debug build); iOS real-device installation and App Store signing still require your own Mac/account, so the real-device acceptance item is on hold.
- Automated acceptance: `flutter analyze` with 0 issues, `flutter test` all green, `mobile.yml` CI green.
- Real-device acceptance (to be added to the mobile section of `docs/guide/acceptance-checklist.md`):
  - Install the APK on a real Android device: login → TOTP → dashboard data consistent with the server;
  - Error presentation for offline/wrong server URL; token expiry auto-renewal without user awareness;
  - ntfy channel push delivered to the phone (trigger an alert on the server side);
  - With the self-signed switch off, a self-signed server connection is rejected; on, it works.
