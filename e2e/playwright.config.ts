import { defineConfig } from '@playwright/test'
import path from 'node:path'

const dataDir = path.resolve(__dirname, '.e2e-data')
// 注意:不在此处 rmSync!worker 进程会重复加载本配置文件,
// 清理动作放在后端 webServer 的启动命令链中(clean-data.js),只执行一次。

export default defineConfig({
  testDir: './tests',
  timeout: 60_000,
  workers: 1,
  fullyParallel: false,
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'retain-on-failure',
  },
  webServer: [
    {
      command: 'go run ./main.go',
      cwd: path.resolve(__dirname, 'fakellm'),
      port: 9999,
      reuseExistingServer: true,
      timeout: 120_000,
    },
    {
      command: 'node ../e2e/clean-data.js && go run ./cmd/gbi > ../e2e/backend-launch.log 2>&1',
      cwd: path.resolve(__dirname, '../server'),
      env: {
        // 显式关闭 SPA 模式并指向不存在的 env 文件,使后端启动配置完全确定,不依赖 gitignored 的 server/.env
        GBI_SPA_MODE: 'false',
        GBI_ENV_FILE: path.join(dataDir, 'no-such-env-file'),
        GBI_ADDR: ':8080',
        GBI_DB_PATH: path.join(dataDir, 'gbi.db'),
        GBI_JWT_SECRET: 'e2e-secret',
        GBI_LLM_BASE_URL: 'http://localhost:9999',
        GBI_LLM_API_KEY: 'fake-key',
        GBI_LLM_MODEL: 'fake-model',
        GBI_RENDER_CHECK_WAIT: '2000',
        GBI_SKILLS_DIR: path.resolve(__dirname, '../server/skills'),
      },
      port: 8080,
      reuseExistingServer: true,
      timeout: 180_000,
    },
    {
      command: 'npm run dev',
      cwd: path.resolve(__dirname, '../web'),
      port: 5173,
      reuseExistingServer: true,
      timeout: 180_000,
    },
  ],
})
