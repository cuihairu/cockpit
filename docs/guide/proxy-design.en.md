# P1 Design: Reverse Proxy Management (visualised delivery of Nginx sites)

> Language: [中文](./proxy-design.md) ｜ **English**

> 2026-09-15. First-phase design for P1 "reverse proxy / routing management". The original todo.md wording: "visualised configuration and delivery of Nginx/Caddy/Traefik, promoting the 'gateway' resource from a record to a manageable object". This document only makes design decisions and interface definitions.

## Current state and pain points

| Pain point | Current state | Consequence |
|------|------|------|
| Adding a site requires SSH + hand-writing a server block | copy an old config and change the domain, `nginx -t` + reload | one mistake in format/escaping and the whole site 502s |
| There's nowhere to look up the current site inventory | the domain→upstream mapping exists only in the operator's head | six months later nobody knows which port runs which service |
| A bad config edit has a large blast radius | editing production conf directly, panicking to roll back when it breaks | users report outages while the reload is failing |
| The Gateway resource is only a record | inventory's Gateway is a network device profile, unrelated to the Nginx reverse proxy | "manageable" never gets off the ground |

## Architecture overview

Same channel model as the file manager: **synchronous RPC + the server purely forwards**. An Nginx config
snippet is KB-scale text, `nginx -t` + reload completes in seconds, and a 30s RPC timeout is ample — there is
no need for an async task model.

```
┌─ server ─────────────────────┐        ┌─ agent (the host nginx runs on) ────────┐
│ REST /api/agents/{id}/proxy  │ ─RPC─▶ │ nginx provider (new)                    │
│ site CRUD + audit            │ ◀─RPC─ │ render → nginx -t → write file → reload│
│ no persistence: files are state│        │ only touches cockpit-*.conf, zero contact│
└──────────────────────────────┘        └─────────────────────────────────┘
```

## Key decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | M1 backend scope | **Nginx only** (bare host install / systemd); Caddy/Traefik come later as separate extensions | Most universal in personal clouds; `nginx -t` validation and graceful reload are mature; containerised nginx has a complex config mount layout, added later |
| D2 | Management boundary | cockpit only manages its own `/etc/nginx/conf.d/cockpit-*.conf` snippets, **with zero contact with existing user config** | It doesn't parse arbitrary nginx syntax (regex-parsing server blocks is notoriously fragile); it's structurally impossible to break an existing site |
| D3 | Site metadata | Embedded in a file header `# cockpit:meta {json}`, and only files cockpit itself wrote are parsed on read | Reading back its own rendered output needs no real nginx parsing, so the roundtrip is reliable |
| D4 | Apply flow | render → `nginx -t` → write file → reload; **a failed `nginx -t` never reaches disk**; a failed reload automatically rolls the file back and retries once | Syntax errors never land on disk; a strong guarantee of "if apply fails, the site stays as it was" |
| D5 | reload method | `systemctl reload nginx` first, falling back to `nginx -s reload` | Under systemd the service semantics are correct; bare installs/containers without systemd use the signal |
| D6 | Execution model | Fully synchronous RPC | Config snippets are small and commands take seconds, so an async task model is over-engineering |
| D7 | Site model | `{name, serverNames[], upstream, scheme(http/https), tlsCert, tlsKey, websocket, extra}` | Covers 90% of personal-cloud scenarios (reverse proxy + WS + certificates); unconventional directives pass straight through `extra`, with `nginx -t` as the backstop |
| D8 | capability | `nginx-proxy`, detected via `exec.LookPath("nginx")` + `nginx -v`; only agents that pass detection register the provider and report the capability | Hosts without nginx don't show a fake capability |
| D9 | REST mount | `/api/agents/{id}/proxy/...`, reusing the file manager's serveAPI `/agents/` dispatch pattern | Scoped per agent, one nginx per machine |
| D10 | Persistence | **Not persisted** — site state has the agent-side file as its single source of truth, with the server purely forwarding + auditing | No server/agent state sync problem; auditing apply/delete is enough for traceability |

## Agent-side design

### Detection and registration

- The `detector` is untouched — the probe function in `internal/agent/agent.go`: after `exec.LookPath("nginx")`
  passes, run `nginx -v 2>&1` to get the version, and on success append the `nginx-proxy` capability
  (metadata: `{version, confDir}`);
- `providers.go`: register `NewNginxProvider` based on the detected `nginx-proxy` capability;
  the conf directory can be overridden with `COCKPIT_NGINX_CONF_DIR` (default `/etc/nginx/conf.d`;
  distributions with a sites-enabled style configure their own).

### RPC methods

| Method | Parameters | Returns | Notes |
|------|------|------|------|
| `proxy.status` | `{}` | `{installed, version, confDir, siteCount, reloadMode}` | overview; reloadMode = systemctl / signal |
| `proxy.sites` | `{}` | `{sites: [{name, serverNames, upstream, scheme, websocket, updatedAt}]}` | list sites under cockpit's name (parsing meta) |
| `proxy.site.get` | `{name}` | `{name, meta, content}` | view the full rendered config |
| `proxy.site.apply` | `{site}` | `{name, file}` | validate → `nginx -t` → write → reload (roll back on failure) |
| `proxy.site.delete` | `{name}` | `{}` | delete the snippet file → reload; restore the file if reload fails |

### Site parameters and validation (same rules on both ends)

| Field | Rule |
|------|------|
| `name` | `^[a-z0-9][a-z0-9_-]{0,63}$` (same as backupNameRe), part of the filename `cockpit-site-<name>.conf` |
| `serverNames` | 1–16 entries, each matching `^[A-Za-z0-9*.\-]{1,253}$` (domain/wildcard) |
| `upstream` | `^[A-Za-z0-9.:\-]{1,253}$`, of the form `127.0.0.1:3000` |
| `scheme` | `http` / `https` |
| `tlsCert` / `tlsKey` | mandatory when scheme=https, absolute paths; **certificate upload goes through the file manager** (file.write) so responsibilities don't overlap |
| `websocket` | bool; when true, renders Upgrade/Connection headers + `proxy_http_version 1.1` |
| `extra` | 0–4KB of free text, inserted verbatim at the end of the server block (a straight-through for advanced users) |

### Render and apply flow

```
render(site) → snippet text (first line is the meta comment)
  ↓
nginx -t                      failure → return an stderr summary, nothing lands on disk
  ↓
write cockpit-site-<name>.conf  the old file content is read into memory first
  ↓
reload                        failure → roll back the old content (or delete the new file) → reload again → return the error
```

- `nginx -t`: `exec.CommandContext(ctx, "nginx", "-t")`, 10s timeout, with the full stderr truncated
  to 2KB and returned to the UI;
- reload: `systemctl reload nginx` (5s timeout) failing → `nginx -s reload`; if both fail treat it
  as a reload failure (roll back);
- meta comment: single-line compressed JSON, `# cockpit:meta {...}`; contains no sensitive information (certificates are path references).

> Interception semantics note (worded per real-machine probe evidence from 2026-10-07): **syntax/certificate
> errors in a new snippet are intercepted synchronously by the parse preceding reload** — the `nginx -s reload`
> -s process parses the full config itself before sending SIGHUP, so a bad snippet reports emerg in that
> process and exits non-zero → it takes the rollback path above; the pre-write `nginx -t` actually only
> guards the existing config (a health check before apply). **A bind conflict is the only asynchronous blind
> spot**: the -s process doesn't bind sockets, so a master-side bind failure (e.g. the port is taken) lands in
> error.log, apply returns 200 and the snippet stays on disk, but the old config keeps serving — the data
> plane is safe ("if apply fails, the site stays as it was" still holds), but the reporting layer cannot
> perceive it synchronously; the systemctl mode is isomorphic (debian's nginx.service ExecReload is
> `nginx -s reload`). If synchronous error summaries need to be closed, the candidate approach is sniffing
> error.log after reload (heuristic), left for M2 evaluation.

### Render template (http site example)

```nginx
# cockpit:meta {"name":"blog","serverNames":["blog.example.com"],...}
server {
    listen 80;
    server_name blog.example.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

- https: `listen 443 ssl` + `ssl_certificate/-key`, plus a second server block rendering
  the 80 → https 301 redirect;
- websocket: appends `proxy_http_version 1.1` + Upgrade/Connection headers inside the location;
- `extra` is appended as-is after the location block, inside the server block.

## Server-side design

### REST API (server/api_proxy_sites.go, mounted on the serveAPI `/agents/` branch, JWT)

| Method | Path | Notes |
|------|------|------|
| GET  | `/api/agents/{id}/proxy/status` | forwards proxy.status |
| GET  | `/api/agents/{id}/proxy/sites` | forwards proxy.sites |
| GET  | `/api/agents/{id}/proxy/sites/{name}` | forwards proxy.site.get (with the full config) |
| PUT  | `/api/agents/{id}/proxy/sites/{name}` | body is the site params → proxy.site.apply + audit proxy_apply |
| DELETE | `/api/agents/{id}/proxy/sites/{name}` | proxy.site.delete + audit proxy_delete |

- Parameter validation on the server side follows the same rules as the agent (defence on both ends); the `nginx -t` stderr summary
  is returned to the UI as-is (this is the key information for the user to correct their input);
- Audit: `ResourceProxySite = "proxy_site"`, actions proxy_apply / proxy_delete;
  the detail records name/domain/upstream and **no certificate content** (paths only).

## Web UI design

- A new `/proxy` "Reverse Proxy" page (menu, ArrowRightOutlined):
  - **Agent selection**: a dropdown annotating the nginx version (agents without the capability are disabled with a hint);
  - **Status card**: nginx version / config directory / site count / reload method;
  - **Site list** Table: name / domain (Tag) / upstream / scheme (http / https Tag) /
    WebSocket / updated time / actions (view config, edit, delete Popconfirm);
  - **Create/edit Modal**: name, domains (tags multi-value), upstream, scheme Radio,
    certificate path/private key path (shown for https, with a hint that "certificate files can be uploaded via Workbench - Files"),
    WebSocket toggle, advanced directives TextArea; a failed save shows the nginx -t error summary;
  - **Config view Modal**: a read-only pre showing the complete rendered snippet.

## M2: Traefik backend (design, 2026-09-18)

The backend extension promised by D1. **M2 is Traefik only, and only via the file provider's dynamic
directory** — each YAML file in the directory pointed at by `providers.file.directory` is an independent
dynamic config, isomorphic to nginx's `conf.d` snippet pattern, so D2's "only its own snippets, zero contact
with user config" holds unchanged. Caddy is deferred: a Caddyfile is whole-file semantics, so splitting it per
site means editing the user's main file to add `import` lines, which conflicts with the zero-contact principle
(see the not-doing list).

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D11 | Backend scope | Traefik file provider only; rendering/validation/apply abstracted per backend (`SiteRenderer`/`SiteApplier`), RPC method names unchanged, the provider holds the backend implementation dispatch | The method table doesn't move server or web routing; nginx behaviour is unchanged |
| D12 | Directory detection and capability | Dynamic directory value: `COCKPIT_TRAEFIK_DIR` override → parse `providers.file.directory` from the static config `/etc/traefik/traefik.yml` (or `.yaml`) → default `/etc/traefik/dynamic`; the capability `traefik-proxy` = the directory exists (the directory goes into metadata for provider init). **Does not depend on LookPath("traefik")** | Traefik mostly runs containerised and the host often has no binary; the directory (including the host-side path of a docker mount) is the source of truth; the env override follows the same convention as `COCKPIT_NGINX_CONF_DIR`; a failed version probe just leaves version empty |
| D13 | Validation and apply | No `nginx -t`/reload equivalent: after rendering, `yaml.Unmarshal` self-check + router→service reference consistency validation, **and nothing lands on disk if validation fails**; the file provider hot-reloads (watches). Bad-file semantics (corrected per real-machine measurement on traefik:v3.5.6, not locally isolated): any bad file in the directory (YAML/Go template syntax errors, including non-cockpit user files) → the watcher callback fails as a whole and **the whole directory's hot update freezes** — existing sites keep serving their last valid config, new changes (including those delivered by the panel) land on disk successfully but are refused; after the bad file is deleted/fixed (including being overwritten by a panel re-delivery), the next watch event **thaws it automatically** and the changes queued during the freeze all take effect | Weaker than nginx in having no global pre-check and a blast radius of the whole directory (but only new changes freeze, existing traffic is undisturbed); stronger in having no reload-failure rollback branch; the apply flow degrades to render→self-check→atomic write; the self-check guarantees cockpit itself never produces a bad file — bad files can only come from manual edits/third-party tools, which is exactly the exposure surface of the drift `drifted` state, and a panel re-delivery cures it while also unfreezing |
| D14 | Render mapping | Same site model (D7 fields unchanged): `serverNames` → multiple values in the router rule's `Host(...)`; `upstream` → the service's `loadBalancer.servers[].url`; https → a 443 router with `tls=true` + file-level `tls.certificates` (certFile/keyFile path references, ACME-pushed files referenced directly) + the 80 router hung on a `redirectScheme` middleware for a permanent redirect; **`websocket` is a no-op** (Traefik passes WS natively, the field is kept for panel compatibility); **`extra` is unsupported** — when non-empty, validation rejects it outright | Redirects use two routers per site rather than static entrypoint config (not touching static config = zero contact); rejecting extra avoids the injection surface of rendering arbitrary YAML fragments |
| D15 | Metadata and drift | The first-line YAML comment `# cockpit:meta {json}` is isomorphic with nginx; the drift kind whitelist extends to `traefik` (`dynamicDir/cockpit-site-<name>.yml`, reading the file is the current state), and the drift page's KIND_LABEL gains a label | The BaselineRecorder/diff chain is generic, only the snapshot branch and whitelist are extended |
| D16 | server / web | The server purely forwards and is unaware of the backend, zero changes; the web status card shows the backend name and version ("-" when there is no binary), the edit Modal's extra control is disabled on the traefik backend with an "unsupported" hint, everything else is reused | Backend differences collapse into the agent render layer and two UI hints |

Rendered snippet example (an https site):

```yaml
# cockpit:meta {"name":"blog","serverNames":["blog.example.com"],...}
http:
  routers:
    cockpit-blog-web:
      rule: "Host(`blog.example.com`)"
      entryPoints: ["web"]
      middlewares: ["cockpit-blog-redirect"]
      service: cockpit-blog
    cockpit-blog-websecure:
      rule: "Host(`blog.example.com`)"
      entryPoints: ["websecure"]
      service: cockpit-blog
      tls: {}
  middlewares:
    cockpit-blog-redirect:
      redirectScheme:
        scheme: https
        permanent: true
  services:
    cockpit-blog:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:3000"
tls:
  certificates:
    - certFile: /path/fullchain.pem
      keyFile: /path/privkey.pem
```

Router/service naming is `cockpit-<site>` (the collision domain is Traefik's global namespace, so a
prefix avoids clashing with the user's dynamic files).

### M2 checklist (implemented 2026-09-18)

- [x] agent: `traefik_provider.go` (detection/render/yaml self-check/atomic write) + capability
      append; the drift whitelist extended to traefik. Implementation delta: D11's wording "RPC method names
      unchanged `proxy.*`" doesn't match the code reality — M1's method prefix is actually `nginx.*`, so the
      implementation followed reality with `traefik.*`, and the server's `proxyRPCPrefix` dispatches by agent
      capability (nginx first, old agents fall back to `nginx.*`); D11's intent (the server doesn't perceive
      backend differences when forwarding) is unaffected
- [x] server: forwarding logic unchanged, only the method prefix dispatched by capability + traefik added to the drift kind
      whitelist (the existing 5 endpoints have test coverage for traefik agent pass-through)
- [x] web: a "backend" row on the status card + hot-reload wording + snippet suffix/hints switching per backend
      + extra disabled; the drift page's KIND_LABEL/COLOR gained traefik
- [x] tests: both render forms (http / https+redirect+certificate reference), yaml self-check rejection
      (missing service reference / no http section), meta roundtrip, extra rejection without writing to disk,
      lifecycle (apply→get→delete + baseline registration/clearing), directory detection (env override /
      static config parsing), drift four states + diff + target validation, server prefix dispatch (nginx only/
      traefik only/dual backend/old agent fallback + traefik audit)
- [x] Real-machine acceptance (listed in todo.md, completed 2026-09-30): a Traefik container
      (traefik:v3.5.6) mounting a host directory as the file provider dynamic directory, probe
      `scripts/acceptance/traefik/` 10/10 — add/modify/delete hot reload, meta read-back,
      https 301 redirect + certificate file reference loading (verified by handshake with our own CA), extra rejection,
      drift four states + record, audit trail; **bad-file semantics measured, correcting D13** (see above)

## Not doing (later items)

- Caddy (Caddyfile rendering): a Caddyfile is whole-file semantics, so per-site splitting means editing the user's main file to add
  import lines, which conflicts with D2's zero contact; deferred unless there's a real need and a one-time main-file collaboration is acceptable;
- Traffic statistics / access log analysis: needs a log collection pipeline, a separate item;
- Certificate issuance and renewal linkage: do it together once the P1 certificate resource and acme integration land;
- upstream health checks / multi-backend load balancing: a single upstream is enough for personal scenarios first;
- Containerised nginx: config mount path discovery is complex, wait for a real need.

## M1 checklist

- [x] agent: `internal/agent/rpc/nginx_provider.go` (detection/render/nginx -t/write/reload/rollback)
      + capability append + registration by capability
- [x] server: `api_proxy_sites.go` (5 endpoints + parameter validation on both ends + audit) + serveAPI integration
      + audit constants
- [x] web: the `/proxy` page (agent selection/status card/site list/edit Modal/config view) + routing menu
- [x] tests: render template forms (http/https+redirect/ws/extra), parameter validation, meta roundtrip,
      nginx -t failure not landing on disk, reload failure rollback, server forwarding and audit
- [x] documentation wrap-up + todo.md synced

## References

- Internal: [file-manager-design.md](./file-manager-design.en.md) (the /agents/ dispatch and certificate upload channel),
  [stack-deploy-design.md](./stack-deploy-design.en.md) (command detection and provider registration),
  the P1 reverse proxy item in `todo.md`
- External: official Nginx docs (the conventional proxy_set_header combination),
  [nginxconfig.io](https://nginxconfig.io/) (the parameterised approach to render templates)