# gbi Deployment Guide

This guide is for operators and administrators: how to build, deploy, upgrade, and harden gbi. After installation, hand the [User Guide](USER_GUIDE.md) to business users.

> English README: [../README.md](../README.md) · 中文 README: [../README.zh-CN.md](../README.zh-CN.md) · 中文部署指南: [DEPLOYMENT.zh-CN.md](DEPLOYMENT.zh-CN.md)

---

## 1. Requirements

| Scenario | Requirements |
|----------|--------------|
| Runtime (release binary) | Windows / Linux / macOS (x64/arm64). **No** Go, Node.js, or database installation required; a writable data directory |
| Build | Go 1.26+; Node.js 20.19+ (22 LTS recommended); bash (Linux/macOS/Git Bash) |
| Browser | Any modern Chromium/Firefox/Safari-based browser |

Single-instance deployment: only one gbi process may use a given `data/` directory (the metadata database is SQLite with a single writer).

## 2. Build the single-file distribution

```bash
# repository root
scripts/build-release.sh
```

The script builds the frontend (`web/`), copies `web/dist` and `server/skills` into the embedded asset directory, then cross-compiles (pure Go, no target toolchain needed) with `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"`. Output:

| File | Platform |
|------|----------|
| `dist/gbi-windows-amd64.exe` | Windows x64 |
| `dist/gbi-windows-arm64.exe` | Windows on ARM |
| `dist/gbi-linux-amd64` | Linux x64 |
| `dist/gbi-linux-arm64` | Linux arm64 |

To build only a subset (comma- or space-separated `os/arch`):

```bash
GBI_TARGETS="linux/amd64" scripts/build-release.sh
GBI_TARGETS="windows/arm64,linux/arm64" scripts/build-release.sh
```

> **Always produce distributable binaries with this script.** A plain `go build` does not refresh the embedded frontend: it either produces a binary that fails at startup (no embedded `index.html`), or — if staging files from a previous release build still exist — silently embeds stale assets. The script clears its staging directories after every build (success or failure), so a bare build always fails loudly instead of shipping stale UI.

## 3. First run and directory layout

Place the executable in any writable directory and run it (use that directory as the working directory):

```bash
./gbi-windows-amd64.exe        # Linux x64: ./gbi-linux-amd64
```

> When a Linux binary was copied from Windows (or transferred in a zip), make it executable first: `chmod +x gbi-linux-amd64`.

On first run the server:

1. creates required directories (`data/`, `logs/`, etc.) and a commented `.env` template;
2. seeds roles and the default administrator **admin / admin123**;
3. seeds a default LLM provider (`deepseek` → `https://api.deepseek.com/anthropic`) and model (`deepseek-flash`, context 384000, max output 16384) with an empty API key.

Immediately after the first login:

- reset the admin password (User Management);
- enter the provider API key in **LLM Settings** and set the model name to one your endpoint actually serves;
- configure a strong random `GBI_JWT_SECRET` (write it into `.env` and restart).

Directory layout:

```
<working dir>/
├── gbi(.exe)
├── .env                      # configuration (template auto-generated; hand-editable)
├── .env.bak-<timestamp>      # backup created while migrating legacy .env keys (may contain secrets)
├── data/
│   ├── gbi.db                # metadata (users/subjects/pages/menus/LLM config, ...)
│   ├── subject-<id>.db       # one SQLite database per data subject
│   ├── branding/             # logo.<png|ico> / favicon.<png|ico>
│   └── backup/               # pre-upgrade backups gbi-v<old>-<timestamp>.db (latest 5 kept)
└── logs/
    └── gbi.log               # size-rotated JSON logs (no file if GBI_LOG_FILE is empty)
```

Default listen address: `:8080`.

## 4. Configuration

Sources: `GBI_*` environment variables take precedence; `.env` is loaded at startup **without overriding** variables already present in the environment. The `.env` path comes from `GBI_ENV_FILE` (default `.env`).

| Variable | Default | Description |
|----------|---------|-------------|
| `GBI_PORT` / `GBI_HOST` | `8080` / `0.0.0.0` | Listen port and address (`host:port`) |
| `GBI_ADDR` | `:8080` | Legacy fallback, used only when `GBI_PORT` is unset |
| `GBI_DB_PATH` | `data/gbi.db` | Metadata database path |
| `GBI_DATA_DIR` | directory of `GBI_DB_PATH` | Subject databases, branding assets, backups |
| `GBI_JWT_SECRET` | dev placeholder | **Must be a strong random value in production**; otherwise admin tokens can be forged |
| `GBI_STATIC_ROOT` | `web/dist` | Serve the UI from this directory when it exists, otherwise use the embedded build |
| `GBI_SPA_MODE` | `true` | SPA fallback; startup verifies that a frontend `index.html` exists |
| `GBI_CORS_ORIGIN` | `*` | Allowed CORS origin; tighten to your domain in production |
| `GBI_LOG_FILE` | empty | JSON log file path; empty = console only |
| `GBI_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `GBI_LOG_MAX_SIZE` | `10` | Max size per log file (MB) before rotation |
| `GBI_ENV_FILE` | `.env` | Configuration file path |
| `GBI_SKILLS_DIR` | `skills` | Skills directory; disk wins when present, otherwise embedded skills |
| `GBI_RENDER_CHECK_WAIT` | `1500` | AI Studio render-check wait (ms) |
| `GBI_HISTORY_KEEP` | `24` | Messages kept verbatim before rolling summarization |

**Obsolete variables (do not set them in new deployments):**

`GBI_LLM_BASE_URL`, `GBI_LLM_API_KEY`, `GBI_LLM_MODEL`, `GBI_LLM_MAX_TOKENS`, `GBI_HISTORY_TOKENS`.

LLM settings are managed from now on in **LLM Settings** in the console. Startup behavior:

- no provider in the database and explicit `GBI_LLM_*` detected → values are migrated into the database (missing fields fall back to factory defaults), then the keys are **physically removed** from `.env` and a backup `.env.bak-<timestamp>` (0600) is written;
- the database is usable (a current model with an API key exists) → the database wins; legacy keys in `.env` are cleaned up as well (backup kept);
- a provider exists but the current model is unusable (no API key) → **no migration and no cleanup**, only a warning; configure the console first, then handle `.env` manually.

Backups may contain old secrets — protect or delete them once the migration is verified.

## 5. Reverse proxy and HTTPS

Bind gbi to loopback (`GBI_HOST=127.0.0.1`, `GBI_PORT=8080`) and let Nginx/Caddy terminate TLS.

Nginx example:

```nginx
server {
    listen 443 ssl;
    server_name bi.example.com;

    ssl_certificate     /etc/nginx/certs/bi.crt;
    ssl_certificate_key /etc/nginx/certs/bi.key;

    # Excel uploads are capped at 2GB per request (adjust as needed)
    client_max_body_size 2048m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # AI chat uses SSE: disable buffering and relax read timeouts
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

Caddy example:

```
bi.example.com {
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1
    }
}
```

Notes:

- raise the proxy request-body limit, or large Excel uploads will be rejected at the proxy;
- SSE requires buffering to be disabled, otherwise chat output is batched;
- with HTTPS, browsers enable secure features (e.g., clipboard) correctly.

## 6. Running as a system service

### Linux (systemd)

```ini
# /etc/systemd/system/gbi.service
[Unit]
Description=gbi Generative BI
After=network.target

[Service]
Type=simple
User=gbi
Group=gbi
# Important: the working directory determines relative .env / data / logs paths
WorkingDirectory=/opt/gbi
ExecStart=/opt/gbi/gbi
Restart=on-failure
RestartSec=3
# Prefer providing GBI_JWT_SECRET etc. via .env or Environment=
# EnvironmentFile=/opt/gbi/.env

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gbi
sudo systemctl status gbi
journalctl -u gbi -f
```

### Windows

Register as a service with [NSSM](https://nssm.cc/):

```bat
nssm install gbi D:\gbi\gbi-windows-amd64.exe
nssm set gbi AppDirectory D:\gbi
nssm set gbi AppStdout D:\gbi\logs\service.out.log
nssm set gbi AppStderr D:\gbi\logs\service.err.log
nssm set gbi Start SERVICE_AUTO_START
nssm start gbi
```

Alternatively use Task Scheduler ("Run whether user is logged on or not") and set "Start in" to the executable directory.

## 7. Upgrade, rollback, and backup

### Upgrade (recommended procedure)

1. Back up the data directory (see below);
2. stop the service;
3. replace the old `gbi(.exe)` with the new one;
4. start the service. Startup automatically:
   - reads the database schema version;
   - if the database is **older** than the binary: takes a `VACUUM INTO` backup under `data/backup/gbi-v<old>-<timestamp>.db`, runs AutoMigrate and versioned migration steps (one transaction plus a version stamp), then runs `PRAGMA integrity_check`;
   - if the database is **newer** than the binary (downgrade): **refuses to start**; restore the newer binary or recover from a backup;
5. confirm the "migration backup" and "database upgrade completed" log lines and smoke-test key features.

The latest 5 backups are kept; older ones are pruned. If a migration fails, the upgrade aborts and the log points to the backup.

### Rollback

1. Stop the service;
2. replace the upgraded database(s) under `data/` with the pre-upgrade backup (or restore the whole pre-upgrade `data/` directory);
3. restore the previous executable and start.

### Manual backup

- Best practice: **stop the service** and copy the whole `data/` directory (this keeps `gbi.db` and `subject-*.db` consistent);
- If you must back up online, use SQLite's backup mechanism (e.g., `VACUUM INTO`); never copy actively-written files directly;
- Backups contain all business data **and LLM API keys** (in the database) — encrypt them and restrict access;
- Test restores regularly.

## 8. Security hardening checklist

- [ ] Set a strong random `GBI_JWT_SECRET` (≥32 bytes), never the default;
- [ ] Change the default admin password `admin123`; create individual admin accounts where appropriate;
- [ ] Expose only the reverse proxy's 443 port; bind gbi to `127.0.0.1`;
- [ ] Enable TLS and tighten `GBI_CORS_ORIGIN` to the exact origin;
- [ ] Restrict permissions on `data/`, `.env`, and `.env.bak-*` (readable only by the service account);
- [ ] Rotate LLM API keys periodically (edit in the console; the API only returns masks);
- [ ] Logs never contain secrets or Authorization headers, but still restrict log access;
- [ ] Back up regularly and verify restores;
- [ ] Never run the e2e suite in production (it wipes its test DB and reuses occupied ports).

## 9. Observability and troubleshooting

- `GET /health` (no login): health and static-asset status; returns 503 `degraded` when the static source is broken;
- `GET /debug/status` (requires an admin JWT): uptime, request counters, goroutine count, static source, data directory, last error;
- Logs: console plus optional JSON file (`GBI_LOG_FILE`), size-rotated; set `GBI_LOG_LEVEL=debug` temporarily for diagnostics;
- Startup logs record the listen address, database path, frontend source (disk/embedded), LLM seeding, and `.env` cleanup results.

### Common issues

| Symptom | Investigation |
|---------|---------------|
| Exits at startup: "SPA mode requires frontend assets" | You are using a bare build without embedded assets. Use `scripts/build-release.sh`, or point `GBI_STATIC_ROOT` to an existing build, or set `GBI_SPA_MODE=false` in development |
| Port already in use | Change `GBI_PORT`; check for a leftover process |
| AI chat returns 400/500 | Check the current model and API key in **LLM Settings**; `max_tokens` must be **greater than 8192** (thinking budget) and **less than** the context window |
| Provider 401/403 | Verify the base URL is an **Anthropic-compatible** endpoint and the API key is correct; some gateways require the `/anthropic` suffix |
| Provider 400 (budget/max_tokens) | Increase max output or decrease the context window; follow the rule above |
| Refuses to start after upgrade: "database version is newer than this program" | You are running an old binary against a newer database; use the newer binary or restore the pre-upgrade backup |
| Large imports fail | Ensure the reverse proxy `client_max_body_size` is large enough (the default 1m rejects them); the server accepts up to 2GB per upload |
| Charts are blank | Ask the AI in Studio to "check page render errors", or inspect the browser console; the usual cause is chart `encode` fields not matching SQL result columns |
| Data file locked while running | Single-instance deployment; stop the service before backing up or moving `data/` |

For business-level questions see the [User Guide](USER_GUIDE.md).
