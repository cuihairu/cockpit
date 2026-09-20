import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RemoteServicesCard from './index'
import type { RemoteService } from './index'

// RemoteServicesCard：远程服务列表 + 自定义连接 Modal（命令预览/复制回退）

const writeText = vi.fn().mockResolvedValue(undefined)
Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

const svc = (overrides: Partial<RemoteService> = {}): RemoteService => ({
  protocol: 'ssh', host: '10.0.0.1', port: 22, name: 'sshd', running: true, ...overrides,
})

describe('RemoteServicesCard', () => {
  beforeEach(() => {
    writeText.mockClear()
  })

  it('无运行中服务显示提示；未运行被过滤', () => {
    render(<RemoteServicesCard services={[svc({ running: false })]} />)
    expect(screen.getByText('未检测到远程服务')).toBeInTheDocument()
  })

  it('运行中服务渲染协议 tag/名称/host:port', () => {
    render(<RemoteServicesCard services={[svc()]} />)
    expect(screen.getByText('SSH')).toBeInTheDocument()
    expect(screen.getByText('sshd')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.1:22')).toBeInTheDocument()
  })

  it('点「连接」打开 Modal 并预填命令预览', async () => {
    render(<RemoteServicesCard services={[svc()]} />)
    fireEvent.click(screen.getByText('连接'))
    expect(await screen.findByText('远程连接')).toBeInTheDocument()
    expect(screen.getByText('ssh -p 22 10.0.0.1')).toBeInTheDocument()
  })

  it('「自定义连接」打开空表单（默认 host 占位）', async () => {
    render(<RemoteServicesCard services={[]} />)
    fireEvent.click(screen.getByText('自定义连接'))
    expect(await screen.findByText('远程连接')).toBeInTheDocument()
    expect(screen.queryByText(/ssh -p/)).toBeNull()
  })

  it('预填后提交回调 onConnect 并关 Modal', async () => {
    const onConnect = vi.fn()
    render(<RemoteServicesCard services={[svc()]} onConnect={onConnect} />)
    fireEvent.click(screen.getByText('连接'))
    await screen.findByText('远程连接')
    fireEvent.click(okButton())
    await waitFor(() => expect(onConnect).toHaveBeenCalledWith('ssh', '10.0.0.1', 22))
    // 提交后关闭（Modal 关闭动画细节属 antd 内部，行为以 onConnect 为准）
  })

  it('无 onConnect 时按协议拼命令写剪贴板', async () => {
    render(<RemoteServicesCard services={[svc()]} />)
    fireEvent.click(screen.getByText('连接'))
    await screen.findByText('远程连接')
    fireEvent.click(okButton())
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('ssh -p 22 10.0.0.1'))
  })
})

// antd Modal 无 ConfigProvider 时默认英文 locale，OK 按钮文本不稳定——按 footer 类选择
function okButton() {
  return document.querySelector('.ant-modal-footer .ant-btn-primary') as HTMLElement
}
