# Agent ID Generation Policy


## Overview

The Agent ID is the Cockpit Agent's unique identifier on the Server. The ID stays stable for the Agent's whole lifecycle — **it does not change across restarts, and reconnects reuse the same ID**, so the Server treats it as the same machine.

## Generation Rules

### Automatic mode (default, without the `-id` flag)

Generated from the operating system's native **machine identifier**, in the format:

```
agent-{hostname}-{first 8 chars of machineId}
```

Example: `agent-coding-edb1f8ab`

Machine identifier sources per platform:

| Platform | Source | Notes |
|------|------|------|
| Linux | `/etc/machine-id` | generated on first systemd boot, 32-char hex, persistent |
| macOS | `IOPlatformUUID` | hardware-level UUID, bound to the mainboard, survives OS reinstalls |
| Windows | `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` | generated at OS install time, persistent |

> Unsupported platforms fall back to `agent-{hostname}-{random}` (changes on every restart; specify `-id` explicitly instead).

### Manual mode (the `-id` flag)

The specified value is used directly as the Agent ID:

```bash
cockpit-agent start -server wss://example.com/ws -id my-custom-agent
```

## Offset (`-bias`)

When multiple Agents are deployed on the same machine, `-bias` distinguishes them (default 0, no suffix appended):

```bash
# Agent 0: agent-coding-edb1f8ab
cockpit-agent start -server wss://example.com/ws

# Agent 1: agent-coding-edb1f8ab-1
cockpit-agent start -server wss://example.com/ws -bias 1

# Agent 2: agent-coding-edb1f8ab-2
cockpit-agent start -server wss://example.com/ws -bias 2
```

A manual `-id` supports bias as well:

```bash
# my-agent-1
cockpit-agent start -server wss://example.com/ws -id my-agent -bias 1
```

## Full ID Generation Flow

```
Is -id specified?
├── yes → ID = {-id} + (bias > 0 ? "-{bias}" : "")
└── no  → read machineID()
         ├── success → ID = "agent-{hostname}-{machineId[:8]}" + (bias > 0 ? "-{bias}" : "")
         └── failure → ID = "agent-{hostname}-{random}" + (bias > 0 ? "-{bias}" : "")
```

## Notes

### Cloned VMs

VMs created in bulk from the same image share the same `/etc/machine-id`; regenerate it:

```bash
# Regenerate machine-id (systemd)
sudo systemd-machine-id-setup
# or manually
sudo rm /etc/machine-id
sudo systemd-machine-id-setup
sudo systemctl restart cockpit-agent
```

### Docker containers

Containers inherit the host's machine-id by default. Ways to isolate:

```bash
# Option 1: mount a dedicated machine-id
echo "$(uuidgen | tr -d '-')" > /etc/cockpit-agent/machine-id
docker run -v /etc/cockpit-agent/machine-id:/etc/machine-id:ro ...

# Option 2: specify the ID explicitly
cockpit-agent start -server wss://example.com/ws -id container-01
```

### macOS first run

Reading `IOPlatformUUID` on macOS needs no root privileges and no extra configuration.

### Windows first run

Reads the registry key `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`; standard user privileges are enough.
