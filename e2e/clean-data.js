// 在 webServer 启动链中执行一次:清掉旧的 .e2e-data,保证全新种子库。
// 不能放在 playwright.config.ts 顶层——worker 进程会重复加载配置文件,
// 会把正在被后端使用的数据库目录删掉。
const fs = require('node:fs')
const path = require('node:path')

const dir = path.resolve(__dirname, '.e2e-data')
fs.rmSync(dir, { recursive: true, force: true })
console.log('[e2e] cleaned', dir)
