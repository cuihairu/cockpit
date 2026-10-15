# guacd Deployment Guide

> Language: [中文](./guacd-deploy.md) ｜ **English**

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

### Environment variables

| Variable | Description | Default |
|------|------|--------|
| `GUACD_LOG_LEVEL` | Log level (do not enable debug — it prints passwords) | `info` |
| `GUACD_BIND` | Bind address (used by the compose file) | `127.0.0.1` |
| `GUACD_PORT` | Port (used by the compose file) | `4822` |
| `TZ` | Timezone | `Asia/Shanghai` |

### Cockpit Server side

Cockpit Server connects to guacd via the `GUACD_ADDR` environment variable:

```bash
# Default value (same-machine deployment)
GUACD_ADDR=127.0.0.1:4822

# For cross-machine deployment, point at the guacd host's internal address
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

## Cross-machine deployment

When guacd and Cockpit Server are not on the same machine:

1. Modify the compose file to bind the internal address:
   ```yaml
   ports:
     - "192.168.5.10:4822:4822"
   ```

2. Set the environment variable on Cockpit Server:
   ```bash
   GUACD_ADDR=192.168.5.10:4822
   ```

3. Make sure the firewall allows port 4822 (internal network only; do not expose it to the public internet).

## Supported protocols

| Protocol | Port | Description |
|------|------|------|
| SSH | 22 | Key/password authentication, character terminal |
| VNC | 5900+ | Desktop remote control |
| RDP | 3389 | Windows remote desktop |

All three protocols are handled uniformly through guacd; Cockpit Server only acts as a bidirectional byte-pipe forwarder.
