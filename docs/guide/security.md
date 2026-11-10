# Security

This page summarizes the security boundaries of the control plane. The remote-connection ticket flow and allowed-target restriction are described in [Runtime State](/guide/runtime).

## Security Concepts

| Boundary | Current implementation |
| --- | --- |
| User authentication | JWT Bearer token |
| Admin initialization | `ADMIN_USERNAME` optional, `ADMIN_PASSWORD` required |
| Two-factor verification | TOTP, secret encrypted with `TOTP_ENCRYPTION_KEY` |
| Agent authentication | Agents with a configured secret must present a matching secret at registration |
| WebSocket Origin | Controlled by `ALLOWED_ORIGINS`; unset is fine for development but not for production |
| Remote connections | 5-minute, single-use ticket |
