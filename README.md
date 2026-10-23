# gbi — Generative BI

**gbi** is a self-hosted generative BI platform. Import Excel data, expose it as governed data subjects, and build dashboards by chatting with an LLM agent — or manually. Everything ships as **one executable**: the Go server, the React frontend, and the agent skills are embedded in a single binary.

- [中文 README](README.zh-CN.md)
- [Deployment Guide (English)](docs/DEPLOYMENT.md) · [部署指南 (中文)](docs/DEPLOYMENT.zh-CN.md)
- [User Guide (English)](docs/USER_GUIDE.md) · [用户指南 (中文)](docs/USER_GUIDE.zh-CN.md)

## Features

- **Single-file deployment** — one cross-platform Go binary with the React SPA and AI skills embedded. Runtime needs no Go, Node.js, or external services. Data lives in local SQLite files.
- **Data subjects** — each subject owns an isolated SQLite database (`subject-N.db`). Excel import up to **2 GB** per upload, with `create` (infer column types) and `append` (match by column name) modes. Tables are prefixed by kind (`dwd_`, `dim_`).
- **Table explorer** — server-side pagination, inline cell editing, row deletion, **server-side column filters** (distinct values), column visibility with select-all, adjustable page size, and frozen first/last columns for wide tables.
- **AI Studio** — chat with the model to build and refine dashboard pages. The agent can inspect schemas, run read-only SQL, rewrite page configs, and **verify rendering**: after each page update, frontend-reported render errors are fed back so the model can fix broken charts instead of claiming success.
- **Safe query engine** — only single `SELECT`/`WITH` statements, blocked keywords, automatic `LIMIT` (max 10,000 rows), executed against per-subject databases attached read-only to an in-memory connection.
- **Provider & model management** — manage LLM providers (Anthropic-compatible base URL + API key) and models (context window, max output) in the console. The current model can be switched **without restart**; conversation history is trimmed using the model's context window.
- **BI pages & portal** — draft/publish workflow, page versions with restore, per-page ACL by user and group, and a portal with configurable navigation menus.
- **RBAC** — built-in `admin` / `developer` / `user` roles, plus departments, groups, and per-user permissions.
- **Branding** — administrators can change the system name, browser title, logo, and favicon (PNG/ICO) for the entire site, including the login page.
- **Operational safety** — first-run bootstrap (directories and `.env` template), versioned schema migrations with an automatic pre-upgrade backup, integrity check, and downgrade protection.

## Why gbi

- **Your data stays yours.** No SaaS dependency; all metadata and datasets are local SQLite files you can copy and back up.
- **One artifact to ship.** The release build bundles UI, skills, and server; deployment is "copy the file and run it".
- **AI with guardrails.** The agent can only execute read-only SQL and update page configs; every generated chart is validated against real queries and browser render feedback.
- **Batteries included.** Authentication, RBAC, audit-friendly logs, migrations, backups, branding, and a full Playwright e2e suite are part of the project.

## Architecture

```
┌─────────────────────────── gbi (single binary) ───────────────────────────┐
│  Gin HTTP server :8080                                                   │
│   ├─ /api/v1/*  ── REST + SSE (studio chat)                              │
│   ├─ /debug/status (admin JWT)                                           │
│   └─ /          ── embedded React SPA (or disk dir in dev)               │
│                                                                          │
│  Services: auth/JWT · RBAC (Casbin) · subjects/tables/import            │
│            pages/versions/ACL/menus · LLM providers/models               │
│            agent loop (schema → SQL → update_page → render check)       │
│                                                                          │
│  Storage: data/gbi.db (metadata)                                         │
│           data/subject-<id>.db (one SQLite per data subject)             │
│           data/branding/ · data/backup/ · logs/                          │
└──────────────────────────────────────────────────────────────────────────┘
```

- **Backend:** Go 1.26, Gin, GORM, pure-Go SQLite (glebarez/sqlite), Casbin, excelize.
- **Frontend:** React 18, antd, AntV G2, zustand, Vite.
- **E2E:** Playwright 36 cases (includes a scripted fake LLM).

## Quick start

### Option A — single-file build (recommended)

Prerequisites for building: Go 1.26+, Node.js 20.19+ (22 LTS recommended), and bash (Linux/macOS/Git Bash).

```bash
scripts/build-release.sh          # builds web, embeds assets + skills, cross-compiles four binaries
cd dist && ./gbi-windows-amd64.exe    # Linux x64: ./gbi-linux-amd64
```

Outputs in `dist/`: `gbi-windows-amd64.exe`, `gbi-windows-arm64.exe`, `gbi-linux-amd64`, `gbi-linux-arm64`. To build only a subset, use `GBI_TARGETS="linux/amd64,windows/arm64" scripts/build-release.sh`.

> **Use this script for every distributable build.** A plain `go build` does not refresh the embedded frontend: it either produces a binary that fails at startup (no embedded `index.html`), or — if staging files from a previous release build are still around — silently embeds stale assets. The script clears its staging directories when it finishes, so a bare build fails loudly instead.

Open <http://localhost:8080> and sign in with **admin / admin123**.

On first run the server:

1. creates `data/`, `logs/`, and a commented `.env` template in the working directory;
2. seeds roles and the `admin` user;
3. seeds a default LLM provider (`deepseek` → `https://api.deepseek.com/anthropic`) and model (`deepseek-flash`, context 384000, max output 16384) with an empty API key.

Then:

1. **Change the admin password** (Users → Reset password) and set `GBI_JWT_SECRET` for real deployments.
2. Open **大模型设置** (LLM Settings), edit the seeded provider, paste your API key, and adjust the model name to one your provider actually serves.
3. Create a data subject, import an Excel file, build a page (or ask the AI Studio to build it), publish it, and add it to a portal menu.

See the [Deployment Guide](docs/DEPLOYMENT.md) for configuration, reverse proxy, services, upgrades, and backups.

### Option B — development

```bash
# Terminal 1: backend API (disable SPA static serving; Vite serves the UI)
# PowerShell: $env:GBI_SPA_MODE='false'; go run ./cmd/gbi
cd server
GBI_SPA_MODE=false go run ./cmd/gbi        # http://localhost:8080

# Terminal 2: frontend dev server (proxies /api → :8080)
cd web
npm install
npm run dev                                # http://localhost:5173
```

To let the backend serve the built UI instead, run `cd web && npm run build`, then start the backend with `GBI_STATIC_ROOT=../web/dist`.

## Configuration

Configuration is loaded from `GBI_*` environment variables; values in `.env` (path via `GBI_ENV_FILE`, default `.env`) are loaded at startup without overriding existing OS variables. Common keys:

| Key | Default | Purpose |
|-----|---------|---------|
| `GBI_PORT` / `GBI_HOST` | `8080` / `0.0.0.0` | Listen port/host (`GBI_ADDR` is a legacy fallback) |
| `GBI_DB_PATH` | `data/gbi.db` | Metadata database |
| `GBI_DATA_DIR` | directory of `GBI_DB_PATH` | Subject DBs, branding assets, backups |
| `GBI_JWT_SECRET` | dev placeholder | **Set a strong random value in production** |
| `GBI_LOG_FILE` / `GBI_LOG_LEVEL` | empty / `info` | File logging (JSON) and level |
| `GBI_STATIC_ROOT` | `web/dist` | Serve UI from disk if it exists, otherwise use the embedded build |
| `GBI_SPA_MODE` | `true` | SPA fallback and startup check |
| `GBI_HISTORY_KEEP` | `24` | Messages kept verbatim before summarization |

LLM provider/model settings are **not** configured through `.env` anymore. Legacy `GBI_LLM_*` and `GBI_HISTORY_TOKENS` keys are migrated to the database on startup, then removed from `.env` with a timestamped backup (`.env.bak-*`). See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the full list.

## Documentation

| Document | Language | Contents |
|----------|----------|----------|
| [README.zh-CN.md](README.zh-CN.md) | 中文 | This page in Chinese |
| [Deployment Guide (English)](docs/DEPLOYMENT.md) | English | Build, configuration, reverse proxy, systemd/Windows service, upgrades, backups, security, troubleshooting |
| [部署指南 (中文)](docs/DEPLOYMENT.zh-CN.md) | 中文 | 同左(中文版) |
| [User Guide (English)](docs/USER_GUIDE.md) | English | Roles, subjects and imports, table operations, AI Studio, pages/publishing/ACL, portal, LLM and branding settings, FAQ |
| [用户指南 (中文)](docs/USER_GUIDE.zh-CN.md) | 中文 | 同左(中文版) |

## Project layout

```
server/   Go backend (cmd/gbi entrypoint, internal packages, skills/)
web/      React SPA (Vite, antd, AntV G2)
e2e/      Playwright end-to-end tests with a scripted fake LLM
scripts/  build-release.sh (single-file distribution build)
docs/     design specs and implementation plans
```

## Testing

```bash
cd server && go test ./...            # Go packages (includes a method-logging coverage gate)
cd web && npx vitest run && npm run build
cd e2e && npx playwright test         # 36 end-to-end cases; make sure :8080/:5173/:9999 are free
```

> The e2e suite starts its own backend which clears its test database. Never run it while a real instance is listening on port 8080 — Playwright would reuse that server and write test data into your real database.
