# guacd Deployment Guide


## Overview

guacd (Apache Guacamole Daemon) is the protocol translation layer of the Cockpit remote desktop gateway. The browser talks to guacd through Cockpit Server (the Go gateway), and guacd terminates the SSH/VNC/RDP protocols.

```
Browser ↔ Cockpit Server (Go) ↔ guacd (TCP 4822) ↔ Target machine
```

## Quick Deployment

### Docker Compose (recommended)

```bash
# Create the directory
mkdir -p /opt/cockpit/guacd

# Download the compose file
curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/deployments/guacd/docker-compose.yml \
  -o /opt/cockpit/guacd/docker-compose.yml

# Start
cd /opt/cockpit/guacd
docker compose up -d

# Verify
docker compose ps        # status should be healthy
nc -z 127.0.0.1 4822     # the port should be reachable
```

### Docker Command

```bash
docker run -d \
  --name cockpit-guacd \
  --restart unless-stopped \
  -p 127.0.0.1:4822:4822 \
  -e GUACD_LOG_LEVEL=info \
  -e TZ=Asia/Shanghai \
  -v guacd-recordings:/var/lib/guacamole \
  guacamole/guacd:1.5.5
```

## Configuration

### guacd environment variables

| Variable | Description | Default |
|------|------|--------|
| `GUACD_LOG_LEVEL` | Log level (do not enable debug — it prints passwords) | `info` |
| `GUACD_BIND` | Bind address (used by the compose file) | `127.0.0.1` |
| `GUACD_PORT` | Port (used by the compose file) | `4822` |
| `TZ` | Timezone | `Asia/Shanghai` |

### Cockpit Server relay environment variables

| Variable | Description | Default |
|------|------|--------|
| `GUAC_RELAY_BIND_ADDR` | Specific local IP for the Server relay; empty uses the route interface selected for the guacd connection | automatic |
| `GUAC_RELAY_HOST` | Server hostname/IP advertised to guacd in the connect instruction; empty uses the bind IP | automatic |

### Cockpit Server side

Cockpit Server connects to guacd via the `GUACD_ADDR` environment variable:

```bash
# Same Docker network: use the service DNS name (recommended)
GUACD_ADDR=guacd:4822

# Non-container, same-machine deployment
GUACD_ADDR=127.0.0.1:4822

# Cross-machine: point at guacd's internal address; guacd must be able to call back to Server
GUACD_ADDR=192.168.5.10:4822
```

For same-machine deployment, no extra configuration is needed in `/etc/cockpit/config.yaml`.

## SSH key authentication

guacd itself does not store keys. SSH keys are managed by **Cockpit Agent**:

1. On startup, Agent checks `~/.ssh/` automatically and generates an Ed25519 key if none exists
2. Only when the user starts an SSH connection from the Web and **has filled in neither a private key nor a password** does Server call the Agent's `ssh.getDefaultKey` RPC to fetch the default private key (explicit credentials are always passed through as-is)
3. Server hands the private key PEM to guacd's `private-key` parameter
4. guacd uses that key to authenticate the connection to the target machine

The user only needs to add the Agent's public key to the target machine's `authorized_keys`:

```bash
# View the Agent public key
cat /home/cui/.ssh/id_ed25519.pub

# Add it to the target machine
ssh-copy-id -i /home/cui/.ssh/id_ed25519.pub user@target-host
```

## Data volumes

| Volume | Purpose |
|------|------|
| `guacd-recordings` | Desktop session recording files (`.guac`) |
| `guacd-drive` | RDP drive redirection (reserved for phase two) |

After a session ends, Cockpit Server automatically collects the recording files and archives
them into the volume's `/data/recordings/` directory (metadata is written back to the
database; `collectGuacRecording`).

## Health checks

```bash
# Container health status
docker inspect cockpit-guacd --format='{{.State.Health.Status}}'

# TCP liveness probe
nc -z 127.0.0.1 4822

# Logs
docker logs cockpit-guacd --tail 20
```

## Upgrade

```bash
cd /opt/cockpit/guacd
docker compose pull
docker compose up -d
```

## Separated networks / cross-machine deployment

Prefer placing Cockpit Server and guacd on the same restricted Docker network and
setting `GUACD_ADDR=guacd:4822`. Server creates a temporary relay bound to the
specific local IP selected by its TCP route to guacd, then advertises that IP/port
to guacd. The relay is not a loopback service and does not bind `0.0.0.0`.

For another host or networks without a shared route, in addition to a reachable
`GUACD_ADDR`, set Server-side `GUAC_RELAY_BIND_ADDR` to a concrete local Server IP
that guacd can reach, and `GUAC_RELAY_HOST` to the Server address resolvable and
reachable from guacd. Restrict the firewall to guacd and the relay port range; do
not expose relay ports publicly. Relays use system ephemeral ports, so apply host
or container network ACLs. Publishing guacd port 4822 or setting only `GUACD_ADDR`
does not establish the reverse callback path.

## Supported protocols

| Protocol | Port | Description |
|------|------|------|
| SSH | 22 | Key/password authentication, character terminal |
| VNC | 5900+ | Desktop remote control |
| RDP | 3389 | Windows remote desktop |

All three protocols are handled uniformly through guacd; Cockpit Server only acts as a bidirectional byte-pipe forwarder.
