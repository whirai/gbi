import { useState } from 'react'
import { Button, Form, Input, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { login } from '../api/auth'
import { setToken } from '../api/client'
import { useAuth } from '../store/auth'
import { useBranding } from '../store/branding'

export default function Login() {
  const nav = useNavigate()
  const setUser = useAuth((s) => s.setUser)
  const systemName = useBranding((s) => s.systemName)
  const logoUrl = useBranding((s) => s.logoUrl)
  const [loading, setLoading] = useState(false)

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true)
    try {
      const res = await login(values.username, values.password)
      setToken(res.token)
      setUser({ id: res.user.id, username: res.user.username, name: res.user.name, role_code: res.role_code })
      nav(
        res.role_code === 'admin' ? '/admin/users' : res.role_code === 'developer' ? '/admin/subjects' : '/portal',
        { replace: true },
      )
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh' }}>
      <div className="glass" style={{ width: 380, padding: '40px 36px' }}>
        {logoUrl && (
          <img src={logoUrl} alt="logo" style={{ display: 'block', height: 40, margin: '0 auto 8px' }} />
        )}
        <h2 style={{ textAlign: 'center', margin: '0 0 8px', color: '#0369a1' }}>{systemName}</h2>
        <p style={{ textAlign: 'center', color: '#64748b', marginTop: 0 }}>登录您的账户</p>
        <Form layout="vertical" onFinish={onFinish} autoComplete="off">
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input size="large" placeholder="用户名" />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password size="large" placeholder="密码" />
          </Form.Item>
          <Button type="primary" htmlType="submit" size="large" block loading={loading}>
            登录
          </Button>
        </Form>
      </div>
    </div>
  )
}
