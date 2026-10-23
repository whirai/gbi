import { useEffect, useState } from 'react'
import { Dropdown, Empty, Menu, Segmented, Spin, message } from 'antd'
import { LogoutOutlined } from '@ant-design/icons'
import { useNavigate, useSearchParams } from 'react-router-dom'
import type { VisibleMenu } from '../api/pages'
import { portalMenus } from '../api/pages'
import PageView from '../components/PageView'
import { useAuth } from '../store/auth'
import { useBranding } from '../store/branding'

export default function Portal() {
  const nav = useNavigate()
  const { user, logout } = useAuth()
  const systemName = useBranding((s) => s.systemName)
  const logoUrl = useBranding((s) => s.logoUrl)
  const [params, setParams] = useSearchParams()
  const [menus, setMenus] = useState<VisibleMenu[]>([])
  const [loading, setLoading] = useState(true)

  const directPage = params.get('page')

  useEffect(() => {
    portalMenus()
      .then(setMenus)
      .catch((e) => message.error((e as Error).message))
      .finally(() => setLoading(false))
  }, [])

  const activeMenu =
    menus.find((m) => m.name === params.get('menu')) ?? menus[0]
  const activePageId = directPage
    ? Number(directPage)
    : activeMenu?.items[0]?.page_id ?? null
  const activeItem = activeMenu?.items.find((i) => i.page_id === activePageId)

  return (
    <div style={{ minHeight: '100vh' }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 16,
          padding: '12px 24px',
          background: 'rgba(255,255,255,0.55)',
          backdropFilter: 'blur(14px)',
          borderBottom: '1px solid rgba(255,255,255,0.9)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontFamily: 'SimHei', fontSize: 17, fontWeight: 700, color: '#0369a1' }}>
          {logoUrl && <img src={logoUrl} alt="logo" style={{ height: 24 }} />}
          <span>{systemName} · 门户</span>
        </div>
        {menus.length > 0 && (
          <Segmented
            options={menus.map((m) => m.name)}
            value={activeMenu?.name}
            onChange={(v) => setParams({ menu: String(v) })}
          />
        )}
        <div style={{ marginLeft: 'auto' }}>
          <Dropdown
            menu={{
              items: [
                ...(user?.role_code === 'admin' || user?.role_code === 'developer'
                  ? [{ key: 'admin', label: '管理控制台', onClick: () => nav('/admin/subjects') }]
                  : []),
                {
                  key: 'logout',
                  icon: <LogoutOutlined />,
                  label: '退出登录',
                  onClick: () => {
                    logout()
                    nav('/login')
                  },
                },
              ],
            }}
          >
            <span style={{ cursor: 'pointer' }}>{user?.name}</span>
          </Dropdown>
        </div>
      </div>

      {loading ? (
        <div style={{ padding: 60, textAlign: 'center' }}>
          <Spin size="large" />
        </div>
      ) : menus.length === 0 && !directPage ? (
        <div className="glass" style={{ margin: 24, padding: 60 }}>
          <Empty description="暂无有权查看的 BI 页面" />
        </div>
      ) : (
        <div style={{ padding: menus.length === 0 ? 20 : '12px 24px' }}>
          {menus.length > 0 && (
            <Menu
              mode="horizontal"
              selectedKeys={activeItem ? [String(activeItem.id)] : []}
              items={(activeMenu?.items ?? []).map((i) => ({ key: String(i.id), label: i.name }))}
              onClick={({ key }) => {
                const item = activeMenu?.items.find((i) => String(i.id) === key)
                if (item && activeMenu) setParams({ menu: activeMenu.name, page: String(item.page_id) })
              }}
              style={{
                marginBottom: 12,
                background: 'transparent',
                borderBottom: '1px solid rgba(14,165,233,0.15)',
              }}
            />
          )}
          {activePageId ? (
            <PageView pageId={activePageId} />
          ) : (
            <div className="glass" style={{ padding: 60 }}>
              <Empty description="该菜单下暂无页面" />
            </div>
          )}
        </div>
      )}
    </div>
  )
}
