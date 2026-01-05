# TunnelZero

All-in-one VPN auto-installer and Telegram bot backend for managing Xray (VMess/VLESS/Trojan), Hysteria 2, and ZiVPN on a VPS.

## Requirements

- Debian/Ubuntu VPS
- Go 1.22+ (installer can bootstrap Go)
- Ports:
  - TCP 443 for Xray (with Reality/TLS and fallback/SNI mux)
  - UDP 443 for Hysteria 2
  - UDP 5667 for ZiVPN

## Quick Start

1. **Clone and build**

   ```bash
   git clone <repo>
   cd tunnelzero
   go build -o tunnelzero
   ```

2. **Run the installer**

   ```bash
   sudo ./tunnelzero
   ```

3. **Follow the prompts**

   - Installation Token: `5407046882`
   - Admin Telegram ID
   - Telegram Bot Token
   - Domain/Subdomain

4. **Bot usage**

   After the installer finishes, the Telegram bot starts automatically. Use `/start` in your bot chat to access the Admin menu.

## What the Installer Does

- Updates APT packages
- Installs dependencies: `certbot`, `socat`, `cron`, `fail2ban`, `vnstat`, `unzip`
- Installs Go if missing
- Requests SSL certificates via Let's Encrypt
- Enables BBR
- Downloads Xray and Hysteria 2 binaries
- Writes Fail2Ban filter/jail for auth failures

## Database

The SQLite database file is `database.db`. Tables include:

- `users`: `id`, `username`, `protocol`, `uuid`, `password`, `expired_date`, `max_devices`, `is_banned`
- `settings`: `admin_id`, `bot_token`, `domain`

## Notes

- Xray inbounds listen on `127.0.0.1` with internal ports. Terminate TLS on port 443 via Xray fallback or an external reverse proxy before routing to these internal ports.
- Hysteria 2 listens on UDP 443.
- ZiVPN listens on UDP 5667.

## Fail2Ban

Auth failures are logged to `/var/log/tunnelzero-auth.log` with entries like:

```
[AUTH_FAIL] Token invalid from IP: 192.168.1.5
```

Fail2Ban reads this log and bans repeated offenders via iptables.
