import React from 'react'
import ReactDOM from 'react-dom/client'
// React 19 下 antd v5 静态方法（message/Modal.confirm 等）依赖此 patch 切到 createRoot
import '@ant-design/v5-patch-for-react-19'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import App from './App'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN}>
      <App />
    </ConfigProvider>
  </React.StrictMode>,
)
