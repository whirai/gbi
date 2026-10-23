# gbi — 生成式 BI

**gbi** 是自托管的生成式 BI 平台:导入 Excel 数据、沉淀为受治理的数据主题,通过与大模型对话(或手动)构建仪表盘。整个系统以**单个可执行文件**分发——Go 服务端、React 前端与 AI 技能文档全部内嵌在一个二进制中。

- [English README](README.md)
- [部署指南(中文)](docs/DEPLOYMENT.zh-CN.md)· [Deployment Guide (English)](docs/DEPLOYMENT.md)
- [用户指南(中文)](docs/USER_GUIDE.zh-CN.md)· [User Guide (English)](docs/USER_GUIDE.md)

## 功能特性

- **单文件部署** — 一个跨平台 Go 二进制,内嵌 React SPA 与 AI 技能;运行时不依赖 Go、Node.js 或任何外部服务。数据保存为本地 SQLite 文件。
- **数据主题** — 每个主题拥有独立 SQLite 库(`subject-N.db`)。Excel 单次上传上限 **2GB**,支持 `create`(自动推断列类型)与 `append`(按列名匹配追加)两种模式;表名按类型加前缀(`dwd_`、`dim_`)。
- **表数据浏览** — 服务端分页、单元格行内编辑、行删除、**服务端列筛选**(去重值多选)、列显隐(全选/全不选)、页大小切换;宽表横向滚动并钉住首尾列。
- **AI 工作台** — 与模型对话即可生成和修改 BI 页面。Agent 可查看表结构、执行只读 SQL、重写页面配置,并**自动核验渲染**:每次更新页面后,前端上报的渲染错误会回灌给模型,让模型真正修好图表,而不是"声称成功"。
- **查询安全** — 仅允许单条 `SELECT`/`WITH`,关键词拦截,自动追加 `LIMIT`(最多 10000 行);各主题库以只读方式 ATTACH 到内存连接执行。
- **供应商/模型管理** — 控制台管理 LLM Provider(Anthropic 兼容 base URL + API Key)与 Model(上下文窗口、最大输出)。切换「当前模型」**无需重启**,历史对话按模型上下文裁剪。
- **BI 页面与门户** — 草稿/发布流程、页面版本与回滚、按用户与用户组的页面 ACL,以及可配置导航菜单的门户。
- **RBAC** — 内置 `admin` / `developer` / `user` 角色,支持部门、用户组与逐用户授权。
- **品牌管理** — 管理员可修改系统名称、浏览器标题、Logo 与 Favicon(PNG/ICO),全站生效(含登录页)。
- **运维保障** — 首次运行自举(目录与 `.env` 模板)、版本化数据库迁移(升级前自动备份、完整性校验、拒绝降级)。

## 优势

- **数据始终在你自己手里** — 不依赖 SaaS;元数据与数据集都是本地 SQLite 文件,可直接拷贝与备份。
- **交付物只有一个文件** — 前端、技能与后端打包进发布版;部署就是"拷贝文件、运行"。
- **带护栏的 AI** — Agent 只能执行只读 SQL 与更新页面配置;生成的每张图都经过真实查询与浏览器渲染反馈校验。
- **开箱即用** — 认证、RBAC、脱敏日志、迁移与备份、品牌、完整的 Playwright e2e(36 用例)均为项目内置能力。

## 架构

```
┌─────────────────────────── gbi(单二进制)───────────────────────────┐
│  Gin HTTP 服务 :8080                                                  │
│   ├─ /api/v1/*  ── REST + SSE(studio 对话)                           │
│   ├─ /debug/status(管理员 JWT)                                       │
│   └─ /          ── 内嵌 React SPA(开发模式可切磁盘目录)              │
│                                                                       │
│  服务层:认证/JWT · RBAC(Casbin)· 主题/表/导入                        │
│         页面/版本/ACL/菜单 · LLM Provider/Model                       │
│         Agent 循环(看结构 → 跑 SQL → 更新页面 → 渲染核验)           │
│                                                                       │
│  存储:data/gbi.db(元数据)                                           │
│        data/subject-<id>.db(每个数据主题一个 SQLite)                 │
│        data/branding/ · data/backup/ · logs/                          │
└───────────────────────────────────────────────────────────────────────┘
```

- **后端**:Go 1.26、Gin、GORM、纯 Go SQLite(glebarez/sqlite)、Casbin、excelize。
- **前端**:React 18、antd、AntV G2、zustand、Vite。
- **端到端**:Playwright 36 用例(内置脚本化的假 LLM)。

## 快速开始

### 方式 A — 单文件构建(推荐)

构建前置:Go 1.26+、Node.js 20.19+(推荐 22 LTS)、bash(Linux/macOS/Git Bash)。

```bash
scripts/build-release.sh          # 构建前端并交叉编译 Windows/Linux 的 amd64/arm64 四个产物
cd dist && ./gbi-windows-amd64.exe    # Linux x64 为 ./gbi-linux-amd64
```

产物为 `dist/gbi-windows-amd64.exe`、`gbi-windows-arm64.exe`、`gbi-linux-amd64`、`gbi-linux-arm64`。只构建部分目标可用 `GBI_TARGETS="linux/amd64,windows/arm64" scripts/build-release.sh`。

> **分发版必须用该脚本构建。** 普通 `go build` 不会刷新内嵌前端:要么启动时因缺少内嵌 `index.html` 而显式失败,要么(当上一次发布构建的 staging 文件仍在时)静默编入陈旧前端。脚本在结束时自动清空 staging,使裸构建一律显式失败,避免误分发。

打开 <http://localhost:8080>,使用 **admin / admin123** 登录。

首次运行会:

1. 在工作目录创建 `data/`、`logs/` 与带注释的 `.env` 模板;
2. 初始化角色与 `admin` 用户;
3. 播种默认 LLM Provider(`deepseek` → `https://api.deepseek.com/anthropic`)与模型(`deepseek-flash`,上下文 384000,最大输出 16384),API Key 为空。

随后:

1. **修改 admin 密码**(用户管理 → 重置密码),生产环境务必设置 `GBI_JWT_SECRET`;
2. 打开**「大模型设置」**,编辑默认 Provider、填入 API Key,并把模型名改成你所用端点实际提供的模型;
3. 新建数据主题 → 导入 Excel → 创建页面(或让 AI 工作台生成)→ 发布 → 配置门户菜单。

配置、反向代理、系统服务、升级与备份详见[部署指南](docs/DEPLOYMENT.zh-CN.md)。

### 方式 B — 本地开发

```bash
# 终端 1:后端 API(关闭 SPA 静态托管,Vite 负责页面)
# PowerShell 写法:$env:GBI_SPA_MODE='false'; go run ./cmd/gbi
cd server
GBI_SPA_MODE=false go run ./cmd/gbi        # http://localhost:8080

# 终端 2:前端开发服务器(/api 代理到 :8080)
cd web
npm install
npm run dev                                # http://localhost:5173
```

若希望后端直接托管构建产物:先 `cd web && npm run build`,再以 `GBI_STATIC_ROOT=../web/dist` 启动后端。

## 配置

配置来自 `GBI_*` 环境变量;`.env`(路径由 `GBI_ENV_FILE` 指定,默认 `.env`)在启动时加载,且不覆盖已存在的系统环境变量。常用项:

| 键 | 默认值 | 说明 |
|----|--------|------|
| `GBI_PORT` / `GBI_HOST` | `8080` / `0.0.0.0` | 监听端口/地址(`GBI_ADDR` 为兼容回退) |
| `GBI_DB_PATH` | `data/gbi.db` | 元数据库路径 |
| `GBI_DATA_DIR` | `GBI_DB_PATH` 所在目录 | 主题库、品牌资源、备份目录 |
| `GBI_JWT_SECRET` | 开发占位值 | **生产环境必须设置为强随机值** |
| `GBI_LOG_FILE` / `GBI_LOG_LEVEL` | 空 / `info` | 文件日志(JSON)与级别 |
| `GBI_STATIC_ROOT` | `web/dist` | 磁盘存在则用它托管前端,否则使用内置构建 |
| `GBI_SPA_MODE` | `true` | SPA 回落与启动校验 |
| `GBI_HISTORY_KEEP` | `24` | 滚动摘要前保留的原文消息条数 |

LLM Provider/模型配置**不再通过 `.env` 设置**。旧的 `GBI_LLM_*` 与 `GBI_HISTORY_TOKENS` 会在启动时迁入数据库,然后从 `.env` 移除并生成带时间戳的备份(`.env.bak-*`)。完整配置项见[部署指南](docs/DEPLOYMENT.zh-CN.md)。

## 文档

| 文档 | 语言 | 内容 |
|------|------|------|
| [README.md](README.md) | English | 英文说明 |
| [部署指南(中文)](docs/DEPLOYMENT.zh-CN.md) | 中文 | 构建、配置项、反向代理、systemd/Windows 服务、升级/回滚、备份、安全加固、排障 |
| [Deployment Guide (English)](docs/DEPLOYMENT.md) | English | 同左(英文版) |
| [用户指南(中文)](docs/USER_GUIDE.zh-CN.md) | 中文 | 角色与登录、数据主题与导入、表操作、AI 工作台、页面/发布/ACL、门户、大模型与品牌设置、常见问题 |
| [User Guide (English)](docs/USER_GUIDE.md) | English | 同左(英文版) |

## 项目结构

```
server/   Go 后端(cmd/gbi 入口、internal 各包、skills/)
web/      React 单页应用(Vite、antd、AntV G2)
e2e/      Playwright 端到端测试(内置脚本化假 LLM)
scripts/  build-release.sh(单文件分发构建)
docs/     设计与实施计划文档
```

## 测试

```bash
cd server && go test ./...            # Go 各包(含方法日志覆盖门禁)
cd web && npx vitest run && npm run build
cd e2e && npx playwright test         # 36 个端到端用例;运行前确认 :8080/:5173/:9999 空闲
```

> e2e 会自行拉起后端并清空测试库。**切勿在真实实例占用 8080 时运行**——Playwright 会复用该服务,把测试数据写入你的真实数据库。
