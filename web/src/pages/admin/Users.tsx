import { useEffect, useState } from 'react'
import { Button, Form, Input, message, Modal, Popconfirm, Select, Space, Table, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import * as deptsApi from '../../api/depts'
import * as rolesApi from '../../api/roles'
import * as usersApi from '../../api/users'
import type { Department, Role, User } from '../../api/types'

const flatten = (nodes: Department[]): Department[] =>
  nodes.flatMap((n) => [n, ...(n.children ? flatten(n.children) : [])])

export default function Users() {
  const [items, setItems] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [depts, setDepts] = useState<Department[]>([])
  const [roles, setRoles] = useState<Role[]>([])
  const [modal, setModal] = useState<{ open: boolean; editing?: User }>({ open: false })
  const [pwdModal, setPwdModal] = useState<{ open: boolean; user?: User }>({ open: false })
  const [form] = Form.useForm()
  const [pwdForm] = Form.useForm()
  const [loading, setLoading] = useState(false)

  const load = async (p: number, kw: string) => {
    setLoading(true)
    try {
      const res = await usersApi.list(p, 20, kw)
      setItems(res.items)
      setTotal(res.total)
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load(1, '')
    // 下拉数据加载失败不再静默:否则表单里部门/角色为空的真实原因不可见
    deptsApi
      .tree()
      .then((t) => setDepts(flatten(t)))
      .catch((e) => message.error((e as Error).message))
    rolesApi.list().then(setRoles).catch((e) => message.error((e as Error).message))
  }, [])

  const submit = async () => {
    try {
      const values = await form.validateFields()
      if (modal.editing) {
        await usersApi.update(modal.editing.id, {
          name: values.name,
          dept_id: values.dept_id ?? null,
          role_id: values.role_id,
          status: values.status,
        })
        message.success('已保存')
      } else {
        await usersApi.create({ ...values, dept_id: values.dept_id ?? null })
        message.success('已创建')
      }
      setModal({ open: false })
      form.resetFields()
      load(page, keyword)
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  const columns = [
    { title: '用户名', dataIndex: 'username' },
    { title: '姓名', dataIndex: 'name' },
    { title: '部门', dataIndex: 'dept_name' },
    {
      title: '角色',
      dataIndex: 'role_code',
      render: (v: string) => <Tag color="blue">{v}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      render: (v: string) => (v === 'active' ? <Tag color="green">启用</Tag> : <Tag color="red">停用</Tag>),
    },
    {
      title: '操作',
      render: (_: unknown, u: User) => (
        <Space>
          <Button
            size="small"
            onClick={() => {
              form.setFieldsValue(u)
              setModal({ open: true, editing: u })
            }}
          >
            编辑
          </Button>
          <Button
            size="small"
            onClick={() => {
              pwdForm.resetFields()
              setPwdModal({ open: true, user: u })
            }}
          >
            重置密码
          </Button>
          <Popconfirm
            title="删除该用户?"
            onConfirm={async () => {
              try {
                await usersApi.remove(u.id)
                message.success('已删除')
                load(page, keyword)
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
        <Input.Search
          placeholder="搜索用户名/姓名"
          allowClear
          style={{ width: 240 }}
          onSearch={(v) => {
            setKeyword(v)
            setPage(1)
            load(1, v)
          }}
        />
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            form.resetFields()
            setModal({ open: true })
          }}
        >
          新建用户
        </Button>
      </Space>
      <Table
        rowKey="id"
        columns={columns}
        dataSource={items}
        loading={loading}
        pagination={{
          current: page,
          pageSize: 20,
          total,
          onChange: (p) => {
            setPage(p)
            load(p, keyword)
          },
        }}
      />
      <Modal
        title={modal.editing ? '编辑用户' : '新建用户'}
        open={modal.open}
        onOk={submit}
        onCancel={() => setModal({ open: false })}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          {!modal.editing && (
            <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
          )}
          {!modal.editing && (
            <Form.Item name="password" label="密码" rules={[{ required: true, min: 6 }]}>
              <Input.Password />
            </Form.Item>
          )}
          <Form.Item name="name" label="姓名" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="dept_id" label="部门">
            <Select allowClear options={depts.map((d) => ({ value: d.id, label: d.name }))} />
          </Form.Item>
          <Form.Item name="role_id" label="角色" rules={[{ required: true }]}>
            <Select options={roles.map((r) => ({ value: r.id, label: r.name }))} />
          </Form.Item>
          {modal.editing && (
            <Form.Item name="status" label="状态" rules={[{ required: true }]}>
              <Select
                options={[
                  { value: 'active', label: '启用' },
                  { value: 'disabled', label: '停用' },
                ]}
              />
            </Form.Item>
          )}
        </Form>
      </Modal>
      <Modal
        title={`重置密码 - ${pwdModal.user?.name ?? ''}`}
        open={pwdModal.open}
        onOk={async () => {
          try {
            const v = await pwdForm.validateFields()
            await usersApi.resetPassword(pwdModal.user!.id, v.password)
            message.success('已重置')
            setPwdModal({ open: false })
            pwdForm.resetFields()
          } catch (e) {
            if ((e as { errorFields?: unknown })?.errorFields) return
            message.error((e as Error).message)
          }
        }}
        onCancel={() => setPwdModal({ open: false })}
        destroyOnClose
      >
        <Form form={pwdForm} layout="vertical">
          <Form.Item name="password" label="新密码" rules={[{ required: true, min: 6 }]}>
            <Input.Password />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
