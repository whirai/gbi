import { useCallback, useEffect, useState } from 'react'
import {
  Button, Card, Form, Input, InputNumber, List, message, Modal, Popconfirm, Select, Space, Table,
} from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import type { Menu, MenuItem } from '../../api/menus'
import * as menusApi from '../../api/menus'
import type { PageSummary } from '../../api/pages'
import * as pagesApi from '../../api/pages'

export default function Menus() {
  const [menus, setMenus] = useState<Menu[]>([])
  const [selected, setSelected] = useState<Menu | null>(null)
  const [items, setItems] = useState<MenuItem[]>([])
  const [pages, setPages] = useState<PageSummary[]>([])
  const [menuModal, setMenuModal] = useState<{ open: boolean; editing?: Menu }>({ open: false })
  const [itemModal, setItemModal] = useState<{ open: boolean; editing?: MenuItem }>({ open: false })
  const [menuForm] = Form.useForm()
  const [itemForm] = Form.useForm()

  const loadMenus = useCallback(async () => {
    try {
      const list = await menusApi.listMenus()
      setMenus(list)
      // 同步 selected 指向刷新后的新对象(重命名后"菜单项 - xxx"标题立即更新);已删则清空
      setSelected((prev) => (prev ? list.find((m) => m.id === prev.id) ?? null : null))
    } catch (e) {
      message.error((e as Error).message)
    }
  }, [])

  const loadItems = useCallback(async (menuId: number) => {
    try {
      setItems(await menusApi.listMenuItems(menuId))
    } catch (e) {
      message.error((e as Error).message)
    }
  }, [])

  useEffect(() => {
    loadMenus()
    pagesApi.listPages().then(setPages).catch(() => undefined)
  }, [loadMenus])

  // 剪除已失效的选中项(与加载解耦:切换选中不再触发重拉)
  useEffect(() => {
    if (selected && !menus.some((m) => m.id === selected.id)) setSelected(null)
  }, [menus, selected])

  useEffect(() => {
    if (selected) loadItems(selected.id)
    else setItems([])
  }, [selected, loadItems])

  const submitMenu = async () => {
    try {
      const v = await menuForm.validateFields()
      if (menuModal.editing) {
        await menusApi.updateMenu(menuModal.editing.id, { name: v.name, desc: v.desc ?? '', sort: v.sort ?? 0 })
      } else {
        await menusApi.createMenu({ name: v.name, desc: v.desc ?? '', sort: v.sort ?? 0 })
      }
      message.success('已保存')
      setMenuModal({ open: false })
      loadMenus()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  const submitItem = async () => {
    if (!selected) return
    try {
      const v = await itemForm.validateFields()
      if (itemModal.editing) {
        await menusApi.updateMenuItem(selected.id, itemModal.editing.id, { name: v.name, sort: v.sort ?? 0, page_id: v.page_id })
      } else {
        await menusApi.createMenuItem(selected.id, { page_id: v.page_id, name: v.name, sort: v.sort ?? 0 })
      }
      message.success('已保存')
      setItemModal({ open: false })
      loadItems(selected.id)
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  const itemColumns = [
    { title: '菜单项', dataIndex: 'name' },
    {
      title: '指向页面',
      dataIndex: 'page_id',
      render: (v: number) => pages.find((p) => p.id === v)?.name ?? `#${v}`,
    },
    { title: '排序', dataIndex: 'sort', width: 80 },
    {
      title: '操作',
      render: (_: unknown, it: MenuItem) => (
        <Space>
          <Button
            size="small"
            onClick={() => {
              itemForm.setFieldsValue({ name: it.name, page_id: it.page_id, sort: it.sort })
              setItemModal({ open: true, editing: it })
            }}
          >
            编辑
          </Button>
          <Popconfirm
            title="删除该菜单项?"
            onConfirm={async () => {
              try {
                await menusApi.removeMenuItem(selected!.id, it.id)
                message.success('已删除')
                loadItems(selected!.id)
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
    <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
      <Card
        size="small"
        title="导航菜单"
        style={{ width: 320, flexShrink: 0 }}
        extra={
          <Button
            size="small"
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              menuForm.resetFields()
              setMenuModal({ open: true })
            }}
          >
            新建菜单
          </Button>
        }
      >
        <List
          dataSource={menus}
          locale={{ emptyText: '暂无菜单' }}
          renderItem={(m) => (
            <List.Item
              style={{
                cursor: 'pointer',
                background: selected?.id === m.id ? 'rgba(14,165,233,0.10)' : undefined,
                borderRadius: 8,
                padding: '8px 10px',
              }}
              onClick={() => setSelected(m)}
              actions={[
                <Button
                  key="edit"
                  size="small"
                  type="link"
                  onClick={(e) => {
                    e.stopPropagation()
                    menuForm.setFieldsValue({ name: m.name, desc: m.desc, sort: m.sort })
                    setMenuModal({ open: true, editing: m })
                  }}
                >
                  编辑
                </Button>,
                <Popconfirm
                  key="del"
                  title="删除菜单及其全部菜单项?"
                  onConfirm={async (e) => {
                    e?.stopPropagation()
                    try {
                      await menusApi.removeMenu(m.id)
                      message.success('已删除')
                      loadMenus()
                    } catch (err) {
                      message.error((err as Error).message)
                    }
                  }}
                  onCancel={(e) => e?.stopPropagation()}
                >
                  <Button size="small" type="link" danger onClick={(e) => e.stopPropagation()}>
                    删除
                  </Button>
                </Popconfirm>,
              ]}
            >
              <List.Item.Meta title={m.name} description={m.desc || undefined} />
            </List.Item>
          )}
        />
      </Card>

      <Card size="small" title={selected ? `菜单项 - ${selected.name}` : '菜单项'} style={{ flex: 1, minWidth: 0 }}>
        {selected ? (
          <>
            <div style={{ marginBottom: 12 }}>
              <Button
                size="small"
                type="primary"
                icon={<PlusOutlined />}
                onClick={() => {
                  itemForm.resetFields()
                  setItemModal({ open: true })
                }}
              >
                添加菜单项
              </Button>
            </div>
            <Table rowKey="id" size="small" columns={itemColumns} dataSource={items} pagination={false} />
          </>
        ) : (
          <div style={{ color: '#64748b', padding: 24, textAlign: 'center' }}>请选择左侧菜单</div>
        )}
      </Card>

      <Modal
        title={menuModal.editing ? '编辑菜单' : '新建菜单'}
        open={menuModal.open}
        onOk={submitMenu}
        onCancel={() => setMenuModal({ open: false })}
        destroyOnClose
      >
        <Form form={menuForm} layout="vertical">
          <Form.Item name="name" label="菜单名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="desc" label="描述">
            <Input.TextArea />
          </Form.Item>
          <Form.Item name="sort" label="排序(越小越靠前)">
            <InputNumber min={0} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={itemModal.editing ? '编辑菜单项' : '添加菜单项'}
        open={itemModal.open}
        onOk={submitItem}
        onCancel={() => setItemModal({ open: false })}
        destroyOnClose
      >
        <Form form={itemForm} layout="vertical">
          <Form.Item name="page_id" label="指向页面" rules={[{ required: true, message: '请选择页面' }]}>
            <Select
              showSearch
              optionFilterProp="label"
              options={pages.map((p) => ({ value: p.id, label: `${p.name} (${p.status === 'published' ? '已发布' : '草稿'})` }))}
            />
          </Form.Item>
          <Form.Item name="name" label="菜单项名称" rules={[{ required: true }]}>
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
