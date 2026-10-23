import { useEffect } from 'react'
import { Button, Form, Input, message, Modal, Radio, Select, Space } from 'antd'
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons'
import * as subjectsApi from '../../../api/subjects'

const COLUMN_TYPES = ['TEXT', 'INTEGER', 'REAL', 'DATE']

export default function CreateTableModal({
  open,
  subjectId,
  onClose,
  onDone,
}: {
  open: boolean
  subjectId: number
  onClose: () => void
  onDone: () => void
}) {
  const [form] = Form.useForm()

  useEffect(() => {
    if (open) {
      form.resetFields()
      form.setFieldsValue({ kind: 'DWD', columns: [{ name: '', type: 'TEXT', comment: '' }] })
    }
  }, [open, form])

  const submit = async () => {
    try {
      const v = await form.validateFields()
      await subjectsApi.createTable(subjectId, {
        name: v.name,
        kind: v.kind,
        columns: v.columns.map((c: { name: string; type: string; comment?: string }) => ({
          name: c.name,
          type: c.type,
          comment: c.comment ?? '',
        })),
      })
      message.success('建表成功')
      onDone()
      onClose()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  return (
    <Modal title="新建表" open={open} onOk={submit} onCancel={onClose} width={640} destroyOnClose>
      <Form form={form} layout="vertical">
        <Form.Item
          name="name"
          label="表名(不含前缀,系统自动加 dwd_/dim_)"
          rules={[
            { required: true },
            { pattern: /^[A-Za-z_][A-Za-z0-9_]*$/, message: '字母/数字/下划线,字母或下划线开头' },
          ]}
        >
          <Input placeholder="如 orders" />
        </Form.Item>
        <Form.Item name="kind" label="表类型" rules={[{ required: true }]}>
          <Radio.Group>
            <Radio value="DWD">DWD 主数据表</Radio>
            <Radio value="DIM">DIM 配置表</Radio>
          </Radio.Group>
        </Form.Item>
        <Form.Item label="列定义" required>
          <Form.List name="columns">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name }) => (
                  <Space key={key} align="baseline" style={{ display: 'flex', marginBottom: 4 }}>
                    <Form.Item
                      name={[name, 'name']}
                      rules={[
                        { required: true, message: '列名必填' },
                        { pattern: /^[A-Za-z_][A-Za-z0-9_]*$/, message: '合法标识符' },
                      ]}
                      style={{ marginBottom: 0 }}
                    >
                      <Input placeholder="列名" style={{ width: 180 }} />
                    </Form.Item>
                    <Form.Item name={[name, 'type']} rules={[{ required: true }]} style={{ marginBottom: 0 }}>
                      <Select style={{ width: 130 }} options={COLUMN_TYPES.map((t) => ({ value: t, label: t }))} />
                    </Form.Item>
                    <Form.Item name={[name, 'comment']} style={{ marginBottom: 0 }}>
                      <Input placeholder="注释(可选)" style={{ width: 180 }} />
                    </Form.Item>
                    {fields.length > 1 && (
                      <MinusCircleOutlined onClick={() => remove(name)} style={{ color: '#ef4444' }} />
                    )}
                  </Space>
                ))}
                <Button type="dashed" onClick={() => add({ name: '', type: 'TEXT', comment: '' })} icon={<PlusOutlined />}>
                  添加列
                </Button>
              </>
            )}
          </Form.List>
        </Form.Item>
      </Form>
    </Modal>
  )
}
