import React from 'react'
import ReactDOM from 'react-dom/client'
import { ConfigProvider } from 'antd'
// 注意:必须走 es 纯 ESM 路径。'antd/locale/zh_CN' 是 CJS 双层包装,
// Vite dev 下 interop 会把 default 包成 {default: locale},导致语言包失效、组件回退英文。
import zhCN from 'antd/es/locale/zh_CN'
import App from './App'
import { antdTheme } from './theme/antd'
import './theme/glass.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider theme={antdTheme} locale={zhCN}>
      <App />
    </ConfigProvider>
  </React.StrictMode>,
)
