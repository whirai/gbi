import { useEffect, useState } from 'react'
import { Button, Drawer, Form, Input, message, Modal, Popconfirm, Select, Space, Table } from 'antd'
import * as groupsApi from '../../api/groups'
import * as usersApi from '../../api/users'
import type { Group, User } from '../../api/types'

export default function Groups() {
  const [groups, setGroups] = useState<Group[]>([])
  const [modal, setModal] = useState<{ open: boolean; editing?: Group }>({ open: false })
  const [drawer, setDrawer] = useState<Group | null>(null)
  const [members, setMembers] = useState<User[]>([])
  const [allUsers, setAllUsers] = useState<User[]>([])
  const [addUserId, setAddUserId] = useState<number | undefined>(undefined)
  const [form] = Form.useForm()

  const load = () => groupsApi.list().then(setGroups).catch((e) => message.error((e as Error).message))
  const loadMembers = (gid: number) =>
    groupsApi.members(gid).then(setMembers).catch((e) => message.error((e as Error).message))

  useEffect(() => {
    load()
  }, [])

  const submit = async () => {
    try {
      const v = await form.validateFields()
      if (modal.editing) await groupsApi.update(modal.editing.id, v)
      else await groupsApi.create(v)
      message.success('已保存')
      setModal({ open: false })
      load()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  const columns = [
    { title: '组名称', dataIndex: 'name' },
    { title: '描述', dataIndex: 'desc' },
    {
      title: '操作',
      render: (_: unknown, g: Group) => (
        <Space>
          <Button
            size="small"
            onClick={async () => {
              setDrawer(g)
              loadMembers(g.id)
              try {
                // 全量用户(分页拉取):后端单页上限 100,直接只拉首页会漏掉后续用户
                setAllUsers(await usersApi.listAllUsers())
              } catch (e) {
                message.error((e as Error).message)
              }
            }}
          >
            成员
          </Button>
          <Button
            size="small"
            onClick={() => {
              form.setFieldsValue(g)
              setModal({ open: true, editing: g })
            }}
          >
            编辑
          </Button>
          <Popconfirm
            title="删除该用户组?"
            onConfirm={async () => {
              try {
                await groupsApi.remove(g.id)
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

  const memberColumns = [
    { title: '用户名', dataIndex: 'username' },
    { title: '姓名', dataIndex: 'name' },
    {
      title: '操作',
      render: (_: unknown, u: User) => (
        <Popconfirm
          title="从组中移除?"
          onConfirm={async () => {
            try {
              await groupsApi.removeMember(drawer!.id, u.id)
              loadMembers(drawer!.id)
            } catch (e) {
              message.error((e as Error).message)
            }
          }}
        >
          <Button size="small" danger>
            移除
          </Button>
        </Popconfirm>
      ),
    },
  ]

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button
          type="primary"
          onClick={() => {
            form.resetFields()
            setModal({ open: true })
          }}
        >
          新建用户组
        </Button>
      </Space>
      <Table rowKey="id" columns={columns} dataSource={groups} pagination={false} />
      <Modal
        title={modal.editing ? '编辑用户组' : '新建用户组'}
        open={modal.open}
        onOk={submit}
        onCancel={() => setModal({ open: false })}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="组名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="desc" label="描述">
            <Input.TextArea />
          </Form.Item>
        </Form>
      </Modal>
      <Drawer title={`成员 - ${drawer?.name ?? ''}`} open={!!drawer} onClose={() => setDrawer(null)} width={520}>
        <Space style={{ marginBottom: 12 }}>
          <Select
            style={{ width: 340 }}
            placeholder="选择用户添加"
            showSearch
            optionFilterProp="label"
            value={addUserId}
            options={allUsers
              .filter((u) => !members.some((m) => m.id === u.id))
              .map((u) => ({ value: u.id, label: `${u.name} (${u.username})` }))}
            onChange={async (uid: number) => {
              try {
                await groupsApi.addMembers(drawer!.id, [uid])
                loadMembers(drawer!.id)
                // 添加成功后清空:已加入者立刻移出 options,保留旧值会渲染成裸数字 id
                setAddUserId(undefined)
              } catch (e) {
                message.error((e as Error).message)
              }
            }}
          />
        </Space>
        <Table rowKey="id" columns={memberColumns} dataSource={members} pagination={false} />
      </Drawer>
    </div>
  )
}
