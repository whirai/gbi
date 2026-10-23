import { useEffect } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import RequireAuth from './components/RequireAuth'
import AdminLayout from './layouts/AdminLayout'
import Login from './pages/Login'
import Portal from './pages/Portal'
import Studio from './pages/Studio'
import Users from './pages/admin/Users'
import Departments from './pages/admin/Departments'
import Groups from './pages/admin/Groups'
import Roles from './pages/admin/Roles'
import LLM from './pages/admin/LLM'
import Subjects from './pages/admin/Subjects'
import Pages from './pages/admin/Pages'
import Menus from './pages/admin/Menus'
import Branding from './pages/admin/Branding'
import { useBranding } from './store/branding'

export default function App() {
  const loadBranding = useBranding((s) => s.load)
  const browserTitle = useBranding((s) => s.browserTitle)
  const faviconUrl = useBranding((s) => s.faviconUrl)

  useEffect(() => {
    loadBranding()
  }, [loadBranding])

  useEffect(() => {
    document.title = browserTitle
    let link = document.querySelector<HTMLLinkElement>("link[rel~='icon']")
    if (!link) {
      link = document.createElement('link')
      link.rel = 'icon'
      document.head.appendChild(link)
    }
    if (faviconUrl) {
      link.href = faviconUrl
      // 自定义图标多为 png/ico,清掉 index.html 残留的 svg type
      link.removeAttribute('type')
    } else {
      link.href = '/favicon.svg'
      link.type = 'image/svg+xml'
    }
  }, [browserTitle, faviconUrl])

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          path="/portal"
          element={
            <RequireAuth>
              <Portal />
            </RequireAuth>
          }
        />
        <Route
          path="/studio"
          element={
            <RequireAuth roles={['admin', 'developer']}>
              <Studio />
            </RequireAuth>
          }
        />
        <Route
          path="/admin"
          element={
            <RequireAuth roles={['admin', 'developer']}>
              <AdminLayout />
            </RequireAuth>
          }
        >
          <Route index element={<Navigate to="users" replace />} />
          {/* 与 AdminLayout 菜单一致:admin-only 子页单独收口,
              防 developer 直接输入 URL 绕过菜单限制 */}
          <Route
            path="users"
            element={
              <RequireAuth roles={['admin']}>
                <Users />
              </RequireAuth>
            }
          />
          <Route
            path="departments"
            element={
              <RequireAuth roles={['admin']}>
                <Departments />
              </RequireAuth>
            }
          />
          <Route
            path="groups"
            element={
              <RequireAuth roles={['admin']}>
                <Groups />
              </RequireAuth>
            }
          />
          <Route
            path="roles"
            element={
              <RequireAuth roles={['admin']}>
                <Roles />
              </RequireAuth>
            }
          />
          <Route
            path="llm"
            element={
              <RequireAuth roles={['admin']}>
                <LLM />
              </RequireAuth>
            }
          />
          <Route
            path="branding"
            element={
              <RequireAuth roles={['admin']}>
                <Branding />
              </RequireAuth>
            }
          />
          <Route path="subjects" element={<Subjects />} />
          <Route path="pages" element={<Pages />} />
          <Route path="menus" element={<Menus />} />
        </Route>
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
