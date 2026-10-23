import { Avatar, Dropdown, Layout, Menu } from 'antd'
import {
  ApartmentOutlined,
  ApiOutlined,
  BarChartOutlined,
  DatabaseOutlined,
  LogoutOutlined,
  MenuOutlined,
  PictureOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
  UsergroupAddOutlined,
} from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../store/auth'
import { useBranding } from '../store/branding'

const allItems = [
  { key: '/admin/users', icon: <TeamOutlined />, label: '用户管理', roles: ['admin'] },
  { key: '/admin/departments', icon: <ApartmentOutlined />, label: '部门管理', roles: ['admin'] },
  { key: '/admin/groups', icon: <UsergroupAddOutlined />, label: '用户组管理', roles: ['admin'] },
  { key: '/admin/roles', icon: <SafetyCertificateOutlined />, label: '角色管理', roles: ['admin'] },
  { key: '/admin/llm', icon: <ApiOutlined />, label: '大模型设置', roles: ['admin'] },
  { key: '/admin/branding', icon: <PictureOutlined />, label: '品牌设置', roles: ['admin'] },
  { key: '/admin/subjects', icon: <DatabaseOutlined />, label: '数据主题管理', roles: ['admin', 'developer'] },
  { key: '/admin/pages', icon: <BarChartOutlined />, label: 'BI页面管理', roles: ['admin', 'developer'] },
  { key: '/admin/menus', icon: <MenuOutlined />, label: '导航菜单管理', roles: ['admin', 'developer'] },
]

export default function AdminLayout() {
  const nav = useNavigate()
  const loc = useLocation()
  const { user, logout } = useAuth()
  const systemName = useBranding((s) => s.systemName)
  const logoUrl = useBranding((s) => s.logoUrl)

  const items = allItems.filter((i) => user && i.roles.includes(user.role_code))

  return (
    <Layout style={{ minHeight: '100vh', background: 'transparent' }}>
      <Layout.Sider
        width={220}
        style={{
          background: 'rgba(255,255,255,0.55)',
          backdropFilter: 'blur(14px)',
          borderRight: '1px solid rgba(255,255,255,0.9)',
        }}
      >
        <div style={{ padding: '20px 16px', display: 'flex', alignItems: 'center', gap: 8, fontFamily: 'SimHei', fontSize: 17, fontWeight: 700, color: '#0369a1' }}>
          {logoUrl && <img src={logoUrl} alt="logo" style={{ height: 22 }} />}
          <span>{systemName} · 控制台</span>
        </div>
        <Menu
          mode="inline"
          style={{ background: 'transparent', borderRight: 0 }}
          selectedKeys={[loc.pathname]}
          items={items}
          onClick={({ key }) => nav(key)}
        />
      </Layout.Sider>
      <Layout style={{ background: 'transparent' }}>
        <Layout.Header
          style={{
            background: 'rgba(255,255,255,0.55)',
            backdropFilter: 'blur(14px)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            padding: '0 24px',
            height: 56,
          }}
        >
          <Dropdown
            menu={{
              items: [
                { key: 'portal', label: '进入BI门户', onClick: () => nav('/portal') },
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
            <span style={{ cursor: 'pointer' }}>
              <Avatar size="small" style={{ background: '#0ea5e9', marginRight: 8 }}>
                {(user?.name ?? 'U').slice(0, 1)}
              </Avatar>
              {user?.name ?? user?.username}
            </span>
          </Dropdown>
        </Layout.Header>
        <Layout.Content style={{ padding: 20 }}>
          <div className="glass" style={{ padding: 20, minHeight: 'calc(100vh - 96px)' }}>
            <Outlet />
          </div>
        </Layout.Content>
      </Layout>
    </Layout>
  )
}
