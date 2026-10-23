import { type ReactNode, useEffect, useRef, useState } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { Spin } from 'antd'
import { getToken } from '../api/client'
import { me } from '../api/auth'
import { useAuth } from '../store/auth'

export default function RequireAuth({
  children,
  roles,
}: {
  children: ReactNode
  roles?: string[]
}) {
  const { user, setUser } = useAuth()
  const [checking, setChecking] = useState(!user)
  const loc = useLocation()
  // 卸载守卫必须是 mount 级 ref:若用 effect 内局部 active 标志(随 [user,setUser]
  // 重跑),setUser 触发的同步重渲染会先执行 cleanup(active=false),随后 pending 的
  // .finally 被吞掉 → setChecking(false) 永不执行 → 永久转圈。
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    if (user || !getToken()) return
    me()
      .then((u) => {
        if (mountedRef.current)
          setUser({ id: u.id, username: u.username, name: u.name, role_code: u.role_code })
      })
      .catch(() => undefined)
      .finally(() => {
        if (mountedRef.current) setChecking(false)
      })
  }, [user, setUser])

  if (!getToken()) return <Navigate to="/login" state={{ from: loc.pathname }} replace />
  if (checking)
    return (
      <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 120 }}>
        <Spin size="large" />
      </div>
    )
  if (roles) {
    // fail-closed:带角色要求的受保护路由上,me() 非 401 失败时 user 为 null,
    // 未核实身份不得渲染 children(非 401 失败 client.ts 不会清 token)。
    if (!user) return <Navigate to="/login" replace />
    if (!roles.includes(user.role_code)) return <Navigate to="/portal" replace />
  }
  return <>{children}</>
}
