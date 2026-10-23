import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, List, message, Modal, Popconfirm, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import * as subjectsApi from '../../api/subjects'
import type { Subject } from '../../api/types'
import SubjectDetail from './subjects/SubjectDetail'

export default function Subjects() {
  const [subjects, setSubjects] = useState<Subject[]>([])
  const [selected, setSelected] = useState<Subject | null>(null)
  const [modal, setModal] = useState<{ open: boolean; editing?: Subject }>({ open: false })
  const [form] = Form.useForm()

  const load = async () => {
    try {
      const list = await subjectsApi.listSubjects()
      setSubjects(list)
      // 同步 selected 指向刷新后的新对象(编辑后 SubjectDetail 标题立即更新);已删则清空
      setSelected((prev) => (prev ? list.find((s) => s.id === prev.id) ?? null : null))
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const submit = async () => {
    try {
      const v = await form.validateFields()
      if (modal.editing) {
        await subjectsApi.updateSubject(modal.editing.id, { name: v.name, desc: v.desc ?? '' })
      } else {
        await subjectsApi.createSubject({ name: v.name, code: v.code, desc: v.desc ?? '' })
      }
      message.success('已保存')
      setModal({ open: false })
      form.resetFields()
      load()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  return (
    <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
      <Card
        size="small"
        title="数据主题"
        style={{ width: 320, flexShrink: 0 }}
        extra={
          <Button
            size="small"
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              form.resetFields()
              setModal({ open: true })
            }}
          >
            新建
          </Button>
        }
      >
        <List
          dataSource={subjects}
          locale={{ emptyText: '暂无主题' }}
          renderItem={(s) => (
            <List.Item
              style={{
                cursor: 'pointer',
                background: selected?.id === s.id ? 'rgba(14,165,233,0.10)' : undefined,
                borderRadius: 8,
                padding: '8px 10px',
              }}
              onClick={() => setSelected(s)}
              actions={[
                <Button
                  key="edit"
                  size="small"
                  type="link"
                  onClick={(e) => {
                    e.stopPropagation()
                    form.setFieldsValue({ name: s.name, code: s.code, desc: s.desc })
                    setModal({ open: true, editing: s })
                  }}
                >
                  编辑
                </Button>,
                <Popconfirm
                  key="del"
                  title="删除主题及其所有表与数据?"
                  onConfirm={async (e) => {
                    e?.stopPropagation()
                    try {
                      await subjectsApi.removeSubject(s.id)
                      if (selected?.id === s.id) setSelected(null)
                      message.success('已删除')
                      load()
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
              <List.Item.Meta
                title={<span style={{ fontFamily: 'SimHei' }}>{s.name}</span>}
                description={<Typography.Text type="secondary">code: {s.code}</Typography.Text>}
              />
            </List.Item>
          )}
        />
      </Card>

      <div style={{ flex: 1, minWidth: 0 }}>
        {selected ? (
          <SubjectDetail key={selected.id} subject={selected} />
        ) : (
          <Card size="small">
            <div style={{ color: '#64748b', padding: 24, textAlign: 'center' }}>
              请选择左侧数据主题,或新建一个主题
            </div>
          </Card>
        )}
      </div>

      <Modal
        title={modal.editing ? '编辑主题' : '新建主题'}
        open={modal.open}
        onOk={submit}
        onCancel={() => setModal({ open: false })}
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="主题名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="code"
            label="主题编码(英文标识,创建后不可改)"
            rules={[
              { required: true },
              { pattern: /^[a-z][a-z0-9_]*$/, message: '小写字母开头,仅含小写字母/数字/下划线' },
              { max: 32, message: '不超过32字符' },
            ]}
          >
            <Input disabled={!!modal.editing} placeholder="如 sales" />
          </Form.Item>
          <Form.Item name="desc" label="描述">
            <Input.TextArea />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
