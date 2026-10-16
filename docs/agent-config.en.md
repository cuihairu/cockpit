# Agent Configuration Reference

> Language: [中文](./agent-config.md) ｜ **English**

## Startup Flags

| Flag | Description | Default |
|------|------|--------|
| `-server` | Server WebSocket address (required) | — |
| `-id` | Agent ID (optional, auto-generated from machine-id) | `agent-{hostname}-{machineId[:8]}` |
| `-secret` | Agent authentication secret | — |
| `-region` | Region | — |
| `-zone` | Availability zone | — |
| `-labels` | Labels, format `key1=v1,key2=v2` | — |
| `-bias` | Offset for multiple Agents on the same machine | `0` |
| `-ssh-keys` | SSH private key directory | `~/.ssh/` |

Example:

```bash
cockpit-agent start \
  -server wss://cockpit.cuihairu.site/ws \
  -region shanghai -zone home \
  -secret MY_SECRET \
  -ssh-keys /home/user/.ssh
```

## Environment Variables

Set via `/etc/default/cockpit-agent` (systemd) or Docker `-e`.

| Variable | Description | Default |
|------|------|--------|
| `COCKPIT_SSH_KEYS` | SSH private key directory (overrides `-ssh-keys` and auto-detection) | auto-detected |
| `SERVER_URL` | Server address (used by the systemd wrapper; required, no code default) | — |
| `REGION` / `ZONE` | Region / availability zone (passed through by the systemd unit; the binary reads `COCKPIT_REGION` / `COCKPIT_ZONE` directly) | — |
| `AGENT_ID` | Agent ID | — |
| `SECRET` | Authentication secret | — |
| `LABELS` | Labels | — |

## SSH Key Paths

When the Agent establishes a remote SSH connection, it searches for private keys in this order:

```
1. Key passed explicitly from the front end (via the Web terminal authentication form)
2. Directory given by the COCKPIT_SSH_KEYS environment variable
3. SUDO_USER's ~/.ssh/ (systemd/sudo case, e.g. /home/cui/.ssh/)
4. Current user's ~/.ssh/ (fallback, e.g. /root/.ssh/)
```

Within each directory, scan in priority order:

| File | Algorithm |
|------|------|
| `id_ed25519` | Ed25519 (recommended) |
| `id_rsa` | RSA |
| `id_ecdsa` | ECDSA |
| `id_dsa` | DSA (not recommended) |

### systemd deployment

The Agent runs as the `cockpit` system user (`ProtectHome=true`, `/home/*` unreachable); user keys are supplied via the directory given in `COCKPIT_SSH_KEYS`:

```bash
# Specify it in /etc/default/cockpit-agent
COCKPIT_SSH_KEYS=/home/cui/.ssh
```

### Docker deployment

```bash
# Option 1: mount host keys (read-only)
docker run -v /home/user/.ssh:/root/.ssh:ro cockpit-agent

# Option 2: specify a custom path
docker run \
  -e COCKPIT_SSH_KEYS=/app/keys \
  -v /path/to/keys:/app/keys:ro \
  cockpit-agent

# Option 3: do not specify; reads /root/.ssh/ inside the container by default
docker run cockpit-agent
```

### Generating keys

```bash
# Generate an Ed25519 key (recommended)
ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -C "cockpit-agent@$(hostname)"

# Add the public key to the target machine
ssh-copy-id -i ~/.ssh/id_ed25519.pub user@target-host
```

## systemd Service Management

```bash
# Check status
sudo systemctl status cockpit-agent

# View logs
sudo journalctl -u cockpit-agent -f

# Restart
sudo systemctl restart cockpit-agent

# Configuration file
sudo vim /etc/default/cockpit-agent
```

Configuration file `/etc/default/cockpit-agent` format:

```bash
SERVER_URL=wss://cockpit.cuihairu.site/ws
REGION=shanghai
ZONE=home
SECRET=your-secret-here
COCKPIT_AGENT_OPTS="-labels env=prod,role=web"
```
