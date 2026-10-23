import { useEffect, useState } from 'react'
import { Button, Form, Input, message, Modal, Table, Tag } from 'antd'
import * as rolesApi from '../../api/roles'
import type { Role } from '../../api/types'

export default function Roles() {
  const [roles, setRoles] = useState<Role[]>([])
  const [editing, setEditing] = useState<Role | null>(null)
  const [form] = Form.useForm()

  const load = () => rolesApi.list().then(setRoles).catch((e) => message.error((e as Error).message))
  useEffect(() => {
    load()
  }, [])

  const columns = [
    { title: '角色代码', dataIndex: 'code', render: (v: string) => <Tag>{v}</Tag> },
    { title: '角色名称', dataIndex: 'name' },
    {
      title: '操作',
      render: (_: unknown, r: Role) => (
        <Button
          size="small"
          onClick={() => {
            form.setFieldsValue({ name: r.name })
            setEditing(r)
          }}
        >
          重命名
        </Button>
      ),
    },
  ]

  return (
    <div>
      <Table rowKey="id" columns={columns} dataSource={roles} pagination={false} />
      <Modal
        title="重命名角色"
        open={!!editing}
        onCancel={() => setEditing(null)}
        onOk={async () => {
          try {
            const v = await form.validateFields()
            await rolesApi.rename(editing!.id, v.name)
            message.success('已保存')
            setEditing(null)
            load()
          } catch (e) {
            if ((e as { errorFields?: unknown })?.errorFields) return
            message.error((e as Error).message)
          }
        }}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="角色名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
