# gbi 部署指南

本指南面向运维/管理员,介绍如何构建、部署、升级与加固 gbi。安装完成后,请把 [用户指南](USER_GUIDE.zh-CN.md) 交给业务用户。

> English: [DEPLOYMENT.md](DEPLOYMENT.md) · English README: [../README.md](../README.md) · 中文 README: [../README.zh-CN.md](../README.zh-CN.md)

---

## 1. 系统要求

| 场景 | 要求 |
|------|------|
| 运行(分发版) | Windows / Linux / macOS(x64/arm64);**无需**安装 Go、Node 或数据库;可写的数据目录 |
| 构建 | Go 1.26+;Node.js 20.19+(推荐 22 LTS);bash(Linux/macOS/Git Bash) |
| 浏览器 | 现代 Chromium/Firefox/Safari 内核浏览器 |

单实例部署:同一 `data/` 目录只允许一个 gbi 进程(元数据库为 SQLite,单写者)。

## 2. 构建单文件分发版

```bash
# 仓库根目录
scripts/build-release.sh
```

脚本依次执行:构建前端(`web/`)→ 把 `web/dist` 与 `server/skills` 拷入内嵌资源目录 → 用 `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"` 交叉编译(纯 Go,无需目标平台工具链)。产物为:

| 文件 | 平台 |
|------|------|
| `dist/gbi-windows-amd64.exe` | Windows x64 |
| `dist/gbi-windows-arm64.exe` | Windows on ARM |
| `dist/gbi-linux-amd64` | Linux x64 |
| `dist/gbi-linux-arm64` | Linux arm64 |

只构建部分目标(逗号或空格分隔的 `os/arch`):

```bash
GBI_TARGETS="linux/amd64" scripts/build-release.sh
GBI_TARGETS="windows/arm64,linux/arm64" scripts/build-release.sh
```

> **分发版必须使用该脚本构建。** 普通 `go build` 不会刷新内嵌前端:要么启动时因缺少内嵌 `index.html` 而显式失败,要么(当上一次发布构建的 staging 文件仍在时)静默编入陈旧前端。脚本在每次构建结束(成功或失败)都会清空 staging,使裸构建一律显式失败,避免误分发旧前端。

## 3. 首次启动与目录布局

把可执行文件放进任意可写目录并运行(建议把该目录作为工作目录):

```bash
./gbi-windows-amd64.exe        # Linux x64: ./gbi-linux-amd64
```

> 从 Windows 拷贝或经 zip 传输的 Linux 产物,先赋予执行权限:`chmod +x gbi-linux-amd64`。

首次运行会:

1. 创建必要目录(`data/`、`logs/` 等)与带注释的 `.env` 模板;
2. 初始化角色与默认管理员 **admin / admin123**;
3. 播种默认 LLM Provider(`deepseek` → `https://api.deepseek.com/anthropic`)与模型(`deepseek-flash`,上下文 384000,最大输出 16384),API Key 为空。

首次登录后请立即:

- 在「用户管理」重置 admin 密码;
- 在「大模型设置」填写 Provider 的 API Key,并把模型名改为你的端点实际提供的模型;
- 设置强随机 `GBI_JWT_SECRET`(可写入 `.env` 后重启)。

目录布局:

```
<工作目录>/
├── gbi(.exe)
├── .env                      # 配置(自动生成模板;可手工维护)
├── .env.bak-<时间戳>         # 旧版 .env 迁移时的备份(可能含密钥,注意权限)
├── data/
│   ├── gbi.db                # 元数据库(用户/主题/页面/菜单/LLM 配置等)
│   ├── subject-<id>.db       # 每个数据主题一个 SQLite 库
│   ├── branding/             # logo.<png|ico> / favicon.<png|ico>
│   └── backup/               # 升级前自动备份 gbi-v<旧版本>-<时间戳>.db(保留最近 5 份)
└── logs/
    └── gbi.log               # 按大小轮转的 JSON 日志(GBI_LOG_FILE 为空则不落盘)
```

默认监听 `:8080`。

## 4. 配置项

配置来源:`GBI_*` 环境变量优先;`.env` 在启动时加载且**不覆盖**已存在的环境变量。`.env` 路径由 `GBI_ENV_FILE` 指定(默认 `.env`)。

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `GBI_PORT` / `GBI_HOST` | `8080` / `0.0.0.0` | 监听端口与地址;设置端口后地址为 `host:port` |
| `GBI_ADDR` | `:8080` | 兼容旧配置;仅在未设置 `GBI_PORT` 时生效 |
| `GBI_DB_PATH` | `data/gbi.db` | 元数据库路径 |
| `GBI_DATA_DIR` | `GBI_DB_PATH` 所在目录 | 主题库、品牌资源、备份目录 |
| `GBI_JWT_SECRET` | 开发占位值 | **生产必须设置**强随机字符串;否则任何可伪造管理员令牌 |
| `GBI_STATIC_ROOT` | `web/dist` | 磁盘目录存在则优先用作前端托管,否则使用内嵌前端 |
| `GBI_SPA_MODE` | `true` | SPA 回落;启动时校验前端 `index.html` 是否存在 |
| `GBI_CORS_ORIGIN` | `*` | CORS 允许来源;生产建议改为确切域名 |
| `GBI_LOG_FILE` | 空 | 日志文件路径(JSON);空 = 仅控制台 |
| `GBI_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `GBI_LOG_MAX_SIZE` | `10` | 单个日志文件上限(MB),超出轮转 |
| `GBI_ENV_FILE` | `.env` | 配置文件路径 |
| `GBI_SKILLS_DIR` | `skills` | 技能目录;磁盘存在则优先,否则使用内嵌技能 |
| `GBI_RENDER_CHECK_WAIT` | `1500` | AI 工作台渲染核验等待(毫秒) |
| `GBI_HISTORY_KEEP` | `24` | 历史对话保留原文条数,更早的滚动摘要 |

**已过时的变量(不要在新部署中设置):**

`GBI_LLM_BASE_URL`、`GBI_LLM_API_KEY`、`GBI_LLM_MODEL`、`GBI_LLM_MAX_TOKENS`、`GBI_HISTORY_TOKENS`。

自本版本起,LLM 配置由控制台「大模型设置」管理。启动时的处理规则:

- 数据库尚无任何 Provider,且检测到显式 `GBI_LLM_*`:迁入数据库(缺失字段取出厂默认),随后从 `.env` **物理移除**并备份为 `.env.bak-<时间戳>`(0600);
- 数据库已可使用(存在当前模型及 API Key)→ 以数据库为准,启动时同样清理 `.env` 中的旧键(保留备份);
- 数据库已有 Provider 但当前模型不可用(缺少 API Key)→ **不迁移、不清理**,仅告警;请先在控制台配置,再手工处理 `.env`。

备份文件可能含旧密钥,确认迁移无误后请妥善保管或删除。

> `.env` 中不要保留任何真实密钥的副本;若手工复制过 `.env`,请同步保护 `.env.bak-*`。

## 5. 反向代理与 HTTPS

建议把 gbi 绑定在本机回环地址(`GBI_HOST=127.0.0.1`、`GBI_PORT=8080`),由 Nginx/Caddy 对外提供 TLS。

Nginx 示例:

```nginx
server {
    listen 443 ssl;
    server_name bi.example.com;

    ssl_certificate     /etc/nginx/certs/bi.crt;
    ssl_certificate_key /etc/nginx/certs/bi.key;

    # Excel 导入单次上限 2GB(按需调整)
    client_max_body_size 2048m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # AI 对话为 SSE 流式响应:关闭缓冲并放宽读超时
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

Caddy 示例:

```
bi.example.com {
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1
    }
}
```

注意:

- 上传 Excel **必须**放开代理的请求体上限,否则大文件会在代理层被拒绝;
- SSE 需要禁用代理缓冲,否则对话输出会被攒批;
- 若使用 HTTPS,浏览器会正确启用剪贴板等安全特性。

## 6. 作为系统服务运行

### Linux(systemd)

```ini
# /etc/systemd/system/gbi.service
[Unit]
Description=gbi Generative BI
After=network.target

[Service]
Type=simple
User=gbi
Group=gbi
# 关键:工作目录决定默认 .env / data / logs 的相对位置
WorkingDirectory=/opt/gbi
ExecStart=/opt/gbi/gbi
Restart=on-failure
RestartSec=3
# 建议通过 .env 或 Environment= 提供 GBI_JWT_SECRET 等
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

可用 [NSSM](https://nssm.cc/) 注册为服务:

```bat
nssm install gbi D:\gbi\gbi-windows-amd64.exe
nssm set gbi AppDirectory D:\gbi
nssm set gbi AppStdout D:\gbi\logs\service.out.log
nssm set gbi AppStderr D:\gbi\logs\service.err.log
nssm set gbi Start SERVICE_AUTO_START
nssm start gbi
```

或使用任务计划程序("无论用户是否登录都运行"),并显式设置"起始于"目录为可执行文件所在目录。

## 7. 升级、回滚与备份

### 升级(推荐流程)

1. 备份数据目录(见下);
2. 停止服务;
3. 用新的 `gbi(.exe)` 覆盖旧文件;
4. 启动服务。启动过程会自动:
   - 读取数据库 schema 版本;
   - 若数据库版本 **低于** 程序版本:先用 `VACUUM INTO` 备份到 `data/backup/gbi-v<旧版本>-<时间戳>.db`,再执行 AutoMigrate 与版本化迁移步骤(单事务 + 版本章),随后做 `PRAGMA integrity_check`;
   - 若数据库版本 **高于** 程序版本(降级):**拒绝启动**,需换回新版本程序或从备份恢复;
5. 查看日志中的「迁移前备份完成」「数据库升级完成」等记录,并验证核心功能。

备份保留最近 5 份,更早的自动清理。迁移失败时升级会中止,日志会给出备份路径。

### 回滚

1. 停止服务;
2. 把 `data/` 中使用的新版数据库替换为升级前备份(或整体恢复升级前的 `data/` 目录);
3. 换回旧版本可执行文件并启动。

### 手工备份

- 最佳方式:**停止服务后**整体复制 `data/`(确保 `gbi.db` 与 `subject-*.db` 属于同一时间点);
- 若必须在线备份,请使用 SQLite 的备份机制(如 `VACUUM INTO`),不要直接拷贝正在写入的文件;
- 备份内容包含全部业务数据与 LLM API Key(数据库),请加密存放并限制访问权限;
- 定期演练恢复流程。

## 8. 安全加固清单

- [ ] 设置强随机 `GBI_JWT_SECRET`(≥32 字节),不要使用默认值;
- [ ] 修改默认管理员密码 `admin123`,必要时为每位管理员建立独立账号;
- [ ] 对外仅暴露反向代理的 443 端口;gbi 绑定 `127.0.0.1`;
- [ ] 配置 TLS,并把 `GBI_CORS_ORIGIN` 收紧为确切来源;
- [ ] 限制 `data/`、`.env`、`.env.bak-*` 的文件权限(仅服务账号可读);
- [ ] 定期轮换 LLM API Key(控制台修改;接口只回显掩码);
- [ ] 日志中不含密钥/Authorization 头;但仍建议限制日志读取权限;
- [ ] 定期备份并验证恢复;
- [ ] 生产环境不要运行 e2e(会清空测试库;且端口占用会被复用)。

## 9. 观测与排障

- `GET /health`(无需登录):服务健康与静态资源状态;静态源异常时返回 503 `degraded`;
- `GET /debug/status`(需要管理员 JWT):运行时长、请求计数、goroutine 数、静态源、数据目录、最近错误;
- 日志:控制台 + 可选 JSON 文件(`GBI_LOG_FILE`),按大小轮转;调试可临时设置 `GBI_LOG_LEVEL=debug`;
- 启动日志会记录监听地址、数据库路径、前端来源(disk/embedded)、LLM 播种与 `.env` 清理结果。

### 常见问题

| 现象 | 排查 |
|------|------|
| 启动即退出:「SPA 模式要求前端产物」 | 使用的是未内嵌前端的裸编译产物。请用 `scripts/build-release.sh` 构建分发版,或指定存在的 `GBI_STATIC_ROOT`,或开发时 `GBI_SPA_MODE=false` |
| 端口被占用 | 修改 `GBI_PORT`;确认没有残留旧进程 |
| AI 对话 400/500 | 控制台「大模型设置」检查当前模型与 API Key;`max_tokens` 必须 **大于 8192**(思考预算)且 **小于**上下文窗口 |
| 供应商 401/403 | 核对 base URL 是否为 **Anthropic 兼容**端点、API Key 是否正确;部分网关需在 URL 末尾保留 `/anthropic` |
| 供应商 400(budget/max_tokens) | 调大最大输出或调小上下文,遵守上一条规则 |
| 升级后拒绝启动「数据库版本高于当前程序」 | 正在用旧程序打开新库;换回新版本,或恢复升级前备份 |
| 导入大文件失败 | 确认反向代理 `client_max_body_size` 足够(默认 1m 会拦截);单次上传上限 2GB |
| 页面图表空白 | 在工作台让 AI「检查页面渲染错误」,或查看浏览器控制台；常见原因是图表 `encode` 字段与 SQL 结果列名不一致 |
| 服务运行中数据文件被占用 | 单实例部署;停止服务后再备份/移动数据目录 |

更多业务操作问题见[用户指南](USER_GUIDE.zh-CN.md)。
