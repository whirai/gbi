import { useEffect, useState } from 'react'
import { Button, Form, Input, message, Modal, Popconfirm, Space, Table, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import type { PageSummary } from '../../api/pages'
import * as pagesApi from '../../api/pages'
import { useAuth } from '../../store/auth'
import AclModal from './pages/AclModal'

export default function Pages() {
  const nav = useNavigate()
  const user = useAuth((s) => s.user)
  const [items, setItems] = useState<PageSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [renaming, setRenaming] = useState<PageSummary | null>(null)
  const [aclPage, setAclPage] = useState<PageSummary | null>(null)
  const [form] = Form.useForm()
  const [renameForm] = Form.useForm()

  const load = async () => {
    setLoading(true)
    try {
      setItems(await pagesApi.listPages())
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const submitCreate = async () => {
    try {
      const v = await form.validateFields()
      await pagesApi.createPage({ name: v.name, desc: v.desc ?? '' })
      message.success('已创建')
      setCreateOpen(false)
      form.resetFields()
      load()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  const submitRename = async () => {
    if (!renaming) return
    try {
      const v = await renameForm.validateFields()
      await pagesApi.updatePage(renaming.id, { name: v.name })
      message.success('已保存')
      setRenaming(null)
      load()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  const togglePublish = async (p: PageSummary) => {
    try {
      if (p.status === 'published') {
        await pagesApi.unpublishPage(p.id)
        message.success('已取消发布')
      } else {
        await pagesApi.publishPage(p.id)
        message.success('已发布')
      }
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const columns = [
    { title: '名称', dataIndex: 'name' },
    { title: '描述', dataIndex: 'desc' },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: string) =>
        v === 'published' ? <Tag color="green">已发布</Tag> : <Tag color="orange">草稿</Tag>,
    },
    { title: '更新时间', dataIndex: 'updated_at', width: 220 },
    {
      title: '操作',
      render: (_: unknown, p: PageSummary) => (
        <Space>
          <Button size="small" onClick={() => nav(`/portal?page=${p.id}`)}>
            预览
          </Button>
          <Button size="small" onClick={() => nav(`/studio?page=${p.id}`)}>
            编辑
          </Button>
          <Button size="small" onClick={() => togglePublish(p)}>
            {p.status === 'published' ? '取消发布' : '发布'}
          </Button>
          {user?.role_code === 'admin' && (
            <Button size="small" onClick={() => setAclPage(p)}>
              权限
            </Button>
          )}
          <Button
            size="small"
            onClick={() => {
              renameForm.setFieldsValue({ name: p.name })
              setRenaming(p)
            }}
          >
            重命名
          </Button>
          <Popconfirm
            title="删除该页面?关联的菜单项与授权将一并移除。"
            onConfirm={async () => {
              try {
                await pagesApi.removePage(p.id)
                message.success('已删除')
                load()
              } catch (e) {
                message.error((e as Error).message)
              }
            }}
          >
            <Button size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            form.resetFields()
            setCreateOpen(true)
          }}
        >
          新建页面
        </Button>
      </Space>
      <Table rowKey="id" columns={columns} dataSource={items} loading={loading} />

      <Modal title="新建页面" open={createOpen} onOk={submitCreate} onCancel={() => setCreateOpen(false)} destroyOnClose>
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="页面名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="desc" label="描述">
            <Input.TextArea />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title="重命名页面" open={!!renaming} onOk={submitRename} onCancel={() => setRenaming(null)} destroyOnClose>
        <Form form={renameForm} layout="vertical">
          <Form.Item name="name" label="页面名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
        </Form>
      </Modal>

      <AclModal open={!!aclPage} pageId={aclPage?.id ?? null} onClose={() => setAclPage(null)} onDone={load} />
    </div>
  )
}
