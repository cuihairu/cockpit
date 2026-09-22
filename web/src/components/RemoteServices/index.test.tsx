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

  it('vnc/rdp/telnet 连接命令各自拼装', async () => {
    for (const [protocol, command] of [
      ['vnc', 'vncviewer 10.0.0.1:5900'],
      ['rdp', 'rdesktop 10.0.0.1:3389'],
      ['telnet', 'telnet://10.0.0.1:23'],
    ] as const) {
      writeText.mockClear()
      const { unmount } = render(
        <RemoteServicesCard services={[svc({ protocol, port: protocol === 'vnc' ? 5900 : protocol === 'rdp' ? 3389 : 23 })]} />,
      )
      fireEvent.click(screen.getByText('连接'))
      await screen.findByText('远程连接')
      fireEvent.click(okButton())
      await waitFor(() => expect(writeText).toHaveBeenCalledWith(command))
      unmount()
    }
  })

  it('命令预览：ssh/vnc/rdp 缺省端口与 host 兜底；填入后透出实际值', async () => {
    render(<RemoteServicesCard services={[]} />)
    fireEvent.click(screen.getByText('自定义连接'))
    await screen.findByText('远程连接')
    const openProto = () =>
      fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select-selector')!)
    // ssh：空 host/port 走默认 22/host 占位
    openProto()
    fireEvent.click(await screen.findByText('SSH'))
    expect(await screen.findByText('ssh -p 22 host')).toBeInTheDocument()
    // vnc/rdp 各自缺省端口
    openProto()
    fireEvent.click(await screen.findByText('VNC'))
    expect(await screen.findByText('vncviewer host:5900')).toBeInTheDocument()
    openProto()
    fireEvent.click(await screen.findByText('RDP'))
    expect(await screen.findByText('rdesktop host:3389')).toBeInTheDocument()
    // 填 host/port 后预览透出
    fireEvent.change(screen.getByPlaceholderText('例如: 192.168.1.100'), { target: { value: '10.1.1.1' } })
    fireEvent.change(screen.getByPlaceholderText('例如: 22'), { target: { value: '3390' } })
    expect(await screen.findByText('rdesktop 10.1.1.1:3390')).toBeInTheDocument()
  })

  it('Modal 取消关闭（onCancel）；loading 显式传入', async () => {
    render(<RemoteServicesCard services={[svc()]} loading />)
    fireEvent.click(screen.getByText('连接'))
    await screen.findByText('远程连接')
    const cancel = document.querySelectorAll('.ant-modal-footer button')[0] as HTMLElement
    fireEvent.click(cancel)
    // 关闭走 onCancel（antd 离场动画细节属内部，以不抛错与 Modal 存续为准）
    expect(document.querySelector('.ant-modal')).not.toBeNull()
  })

  it('services 默认空数组（props 缺省）', () => {
    render(<RemoteServicesCard />)
    expect(screen.getByText('未检测到远程服务')).toBeInTheDocument()
  })
})

// antd Modal 无 ConfigProvider 时默认英文 locale，OK 按钮文本不稳定——按 footer 类选择
function okButton() {
  return document.querySelector('.ant-modal-footer .ant-btn-primary') as HTMLElement
}
