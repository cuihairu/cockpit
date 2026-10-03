import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message as antdMessage } from 'antd'
import VaultVerifyModal from './index'

// VaultVerifyModal：凭据保险箱二次验证弹窗。登录密码或 TOTP 动态码二选一，
// 通过后回调 onVerified（token 由 services/remote 内部记录，组件不碰）。
// 三个分支：成功、服务端错误文案回显、非字符串错误体兜底。

const verifyVault = vi.hoisted(() => vi.fn())
vi.mock('@/services/remote', () => ({ verifyVault }))

const msgSuccess = vi.spyOn(antdMessage, 'success')
const msgError = vi.spyOn(antdMessage, 'error')

const props = {
  visible: true,
  onVerified: vi.fn(),
  onCancel: vi.fn(),
}

const submit = (fields: { password?: string; totpCode?: string }) => {
  if (fields.password !== undefined) {
    fireEvent.change(screen.getAllByPlaceholderText('登录密码')[0], { target: { value: fields.password } })
  }
  if (fields.totpCode !== undefined) {
    fireEvent.change(screen.getByPlaceholderText('6 位动态码（未开启 TOTP 可留空）'), {
      target: { value: fields.totpCode },
    })
  }
  fireEvent.click(screen.getByRole('button', { name: /验\s*证/ }))
}

describe('VaultVerifyModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    verifyVault.mockResolvedValue({ token: 'vt-1', expiresAt: '' })
  })

  it('密码提交成功：提示 + 清表单 + onVerified', async () => {
    render(<VaultVerifyModal {...props} />)
    submit({ password: 'pw' })
    await waitFor(() => expect(props.onVerified).toHaveBeenCalledTimes(1))
    expect(verifyVault).toHaveBeenCalledWith({ password: 'pw', totpCode: undefined })
    expect(msgSuccess).toHaveBeenCalledWith('验证通过')
    // resetFields：再提交时密码框已空
    await waitFor(() => {
      expect((screen.getAllByPlaceholderText('登录密码')[0] as HTMLInputElement).value).toBe('')
    })
  })

  it('TOTP 动态码提交：按 totpCode 传给服务', async () => {
    render(<VaultVerifyModal {...props} />)
    submit({ totpCode: '123456' })
    await waitFor(() => expect(props.onVerified).toHaveBeenCalledTimes(1))
    expect(verifyVault).toHaveBeenCalledWith({ password: undefined, totpCode: '123456' })
  })

  it('自定义标题（删除态）与「取消」回调', () => {
    const onCancel = vi.fn()
    render(<VaultVerifyModal {...props} title="验证以删除已存凭据" onCancel={onCancel} />)
    expect(screen.getByText('验证以删除已存凭据')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(verifyVault).not.toHaveBeenCalled()
  })

  it('服务端错误：回显 response.data 文案，不调 onVerified', async () => {
    verifyVault.mockRejectedValueOnce({ response: { data: '验证失败：口令错误' } })
    render(<VaultVerifyModal {...props} />)
    submit({ password: 'wrong' })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('验证失败：口令错误'))
    expect(props.onVerified).not.toHaveBeenCalled()
  })

  it('错误体非字符串（对象）→ 兜底「验证失败，请重试」；无 response 同样兜底', async () => {
    verifyVault.mockRejectedValueOnce({ response: { data: { code: 401 } } })
    render(<VaultVerifyModal {...props} />)
    submit({ password: 'pw' })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('验证失败，请重试'))

    cleanupSecond()
    verifyVault.mockRejectedValueOnce(new Error('network'))
    render(<VaultVerifyModal {...props} />)
    submit({ password: 'pw' })
    await waitFor(() => expect(msgError).toHaveBeenCalledTimes(2))
    expect(msgError).toHaveBeenLastCalledWith('验证失败，请重试')
    expect(props.onVerified).not.toHaveBeenCalled()
  })
})

// 第二个 Modal 需独立容器（antd Modal 挂 body，重复挂载会命中两处同名按钮）
function cleanupSecond() {
  document.querySelectorAll('.ant-modal-wrap').forEach((el) => el.remove())
}