import { useEffect, useState } from 'react'
import { Button, Form, Input, InputNumber, message, Modal, Popconfirm, Space, Tree } from 'antd'
import * as deptsApi from '../../api/depts'
import type { Department } from '../../api/types'

const collectKeys = (nodes: Department[]): number[] =>
  nodes.flatMap((n) => [n.id, ...(n.children ? collectKeys(n.children) : [])])

export default function Departments() {
  const [treeData, setTreeData] = useState<Department[]>([])
  // 受控展开:rc-tree 的 defaultExpandAll 只在首次挂载时对"当时已有子节点"的键生效,
  // 异步加载的树必须受控展开,否则后加的节点永远不可见。
  const [expandedKeys, setExpandedKeys] = useState<number[]>([])
  const [modal, setModal] = useState<{ open: boolean; parent?: Department; editing?: Department }>({
    open: false,
  })
  const [form] = Form.useForm()

  const load = () =>
    deptsApi
      .tree()
      .then((t) => {
        setTreeData(t)
        setExpandedKeys(collectKeys(t))
      })
      .catch((e) => message.error(e.message))
  useEffect(() => {
    load()
  }, [])

  const remove = async (n: Department) => {
    try {
      await deptsApi.remove(n.id)
      message.success('已删除')
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const toTreeData = (nodes: Department[]): any[] =>
    nodes.map((n) => ({
      key: n.id,
      title: (
        <Space>
          <span>{n.name}</span>
          <Button
            size="small"
            type="link"
            onClick={() => {
              form.resetFields()
              setModal({ open: true, parent: n })
            }}
          >
            添加子级
          </Button>
          <Button
            size="small"
            type="link"
            onClick={() => {
              form.setFieldsValue({ name: n.name, sort: n.sort })
              setModal({ open: true, editing: n })
            }}
          >
            重命名
          </Button>
          <Popconfirm title="删除该部门?" onConfirm={() => remove(n)}>
            <Button size="small" type="link" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
      children: n.children?.length ? toTreeData(n.children) : undefined,
    }))

  const submit = async () => {
    try {
      const v = await form.validateFields()
      if (modal.editing) {
        await deptsApi.update(modal.editing.id, { name: v.name, sort: v.sort ?? 0 })
      } else {
        await deptsApi.create({ name: v.name, parent_id: modal.parent?.id ?? null, sort: v.sort ?? 0 })
      }
      message.success('已保存')
      setModal({ open: false })
      load()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return // 表单校验失败由 antd 内联提示
      message.error((e as Error).message)
    }
  }

  const title = modal.editing
    ? '重命名部门'
    : modal.parent
      ? `在 "${modal.parent.name}" 下添加子部门`
      : '新建顶层部门'

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
          新建顶层部门
        </Button>
      </Space>
      {treeData.length ? (
        <Tree
          treeData={toTreeData(treeData)}
          expandedKeys={expandedKeys}
          onExpand={(keys) => setExpandedKeys(keys as number[])}
          selectable={false}
        />
      ) : (
        <div>暂无部门</div>
      )}
      <Modal title={title} open={modal.open} onOk={submit} onCancel={() => setModal({ open: false })} destroyOnClose>
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="部门名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="sort" label="排序(越小越靠前)">
            <InputNumber min={0} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
