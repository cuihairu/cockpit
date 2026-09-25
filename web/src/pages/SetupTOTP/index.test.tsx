import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import SetupTOTP from './index'

// SetupTOTP：四步流程——挂载即拉 TOTP 数据自动到扫码步，
// 「下一步」直进验证（曾误绑备份确认致流程死锁，已修），
// 验证码清洗 + enableTOTP + 备份码确认解锁完成。
// QR/备份码子组件已有独立测试，此处 mock 成轻量桩。

const apiMock = vi.hoisted(() => ({ generateTOTP: vi.fn(), enableTOTP: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/components/QRCodeDisplay', () => ({
  default: ({ title }: { title: string }) => <div data-testid="qr-stub">{title}</div>,
}))
vi.mock('@/components/BackupCodesDisplay', () => ({
  default: ({ codes }: { codes: string[] }) => <div data-testid="backup-stub">{codes.join(',')}</div>,
}))

const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')
const msgSuccess = vi.spyOn(message, 'success')

const totpData = {
  secret: 'SECRET234567',
  qr_code: 'otpauth://totp/x',
  backup_codes: ['b1', 'b2'],
}

const renderPage = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/setup-totp']}>
        <Routes>
          <Route path="/setup-totp" element={<SetupTOTP />} />
          <Route path="/settings" element={<span data-testid="settings-page" />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const nextBtn = () => screen.getByRole('button', { name: '下一步' })
const codeInput = () => screen.getByPlaceholderText('123456')
const verifyBtn = () => screen.getByRole('button', { name: /^验\s*证$/ }) as HTMLButtonElement

describe('SetupTOTP', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.generateTOTP.mockReset()
    apiMock.generateTOTP.mockResolvedValue(totpData)
    apiMock.enableTOTP.mockReset()
    apiMock.enableTOTP.mockResolvedValue({})
  })

  it('挂载拉取成功自动进入扫码步：QR 桩与密钥展示', async () => {
    renderPage()
    expect(await screen.findByTestId('qr-stub')).toBeInTheDocument()
    expect(screen.getByText('或手动输入密钥')).toBeInTheDocument()
    expect((document.querySelector('input[readonly]') as HTMLInputElement).value).toBe('SECRET234567')
  })

  it('扫码步「下一步」直进验证步（不再被备份确认拦截）', async () => {
    renderPage()
    await screen.findByTestId('qr-stub')
    fireEvent.click(nextBtn())
    expect(await screen.findByPlaceholderText('123456')).toBeInTheDocument()
    expect(screen.getByText('打开认证器应用，输入显示的 6 位验证码')).toBeInTheDocument()
  })

  it('扫码步「取消」返回设置页', async () => {
    renderPage()
    await screen.findByTestId('qr-stub')
    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))
    expect(screen.getByTestId('settings-page')).toBeInTheDocument()
  })

  it('验证步「上一步」回到扫码步', async () => {
    renderPage()
    await screen.findByTestId('qr-stub')
    fireEvent.click(nextBtn())
    await screen.findByPlaceholderText('123456')
    fireEvent.click(screen.getByRole('button', { name: /上一步/ }))
    expect(await screen.findByTestId('qr-stub')).toBeInTheDocument()
  })

  it('验证码剔除非数字；不足 6 位时按钮禁用、Enter 提交告警', async () => {
    renderPage()
    await screen.findByTestId('qr-stub')
    fireEvent.click(nextBtn())
    await screen.findByPlaceholderText('123456')
    fireEvent.change(codeInput(), { target: { value: '1a2b3c4d' } })
    expect(codeInput()).toHaveValue('1234')
    expect(verifyBtn().disabled).toBe(true)
    await act(async () => {
      fireEvent.keyDown(codeInput(), { key: 'Enter' })
      fireEvent.keyUp(codeInput(), { key: 'Enter' })
    })
    expect(msgWarning).toHaveBeenCalledWith('请输入 6 位验证码')
    expect(apiMock.enableTOTP).not.toHaveBeenCalled()
  })

  it('验证失败：错误文案展示，重新输入清空', async () => {
    apiMock.enableTOTP.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByTestId('qr-stub')
    fireEvent.click(nextBtn())
    await screen.findByPlaceholderText('123456')
    fireEvent.change(codeInput(), { target: { value: '000000' } })
    await act(async () => {
      fireEvent.click(verifyBtn())
    })
    expect(await screen.findByText('验证失败')).toBeInTheDocument()
    expect(codeInput().classList).toContain('ant-input-status-error') // status="error"
    fireEvent.click(screen.getByRole('button', { name: '重新输入' }))
    expect(codeInput()).toHaveValue('')
    expect(screen.queryByText('验证失败')).toBeNull()
  })

  it('验证成功：完成步备份码展示，勾选确认后完成跳设置页', async () => {
    renderPage()
    await screen.findByTestId('qr-stub')
    fireEvent.click(nextBtn())
    await screen.findByPlaceholderText('123456')
    fireEvent.change(codeInput(), { target: { value: '123456' } })
    await act(async () => {
      fireEvent.click(verifyBtn())
    })
    expect(await screen.findByTestId('backup-stub')).toBeInTheDocument()
    expect(screen.getByTestId('backup-stub')).toHaveTextContent('b1,b2')
    expect(msgSuccess).toHaveBeenCalledWith('TOTP 已启用')
    const finishBtn = screen.getByRole('button', { name: '完 成' }) as HTMLButtonElement
    expect(finishBtn.disabled).toBe(true)
    fireEvent.click(document.querySelector('.backup-confirm input[type="checkbox"]')!)
    expect(finishBtn.disabled).toBe(false)
    fireEvent.click(finishBtn)
    expect(await screen.findByTestId('settings-page')).toBeInTheDocument()
  })

  it('生成 TOTP 失败：报错并跳回设置页', async () => {
    apiMock.generateTOTP.mockRejectedValue(new Error('boom'))
    renderPage()
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('生成 TOTP 失败'))
    expect(await screen.findByTestId('settings-page')).toBeInTheDocument()
  })
})
