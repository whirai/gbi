import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Descriptions, Form, Input, InputNumber, message, Modal, Popconfirm, Space, Table, Tag } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import * as llmApi from '../../api/llm'
import type { LLMCurrent, LLMModel, LLMProvider } from '../../api/llm'

export default function LLM() {
  const [providers, setProviders] = useState<LLMProvider[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [models, setModels] = useState<LLMModel[]>([])
  const [current, setCurrent] = useState<LLMCurrent | null>(null)
  const [loading, setLoading] = useState(false)
  const [providerModal, setProviderModal] = useState<{ open: boolean; editing?: LLMProvider }>({ open: false })
  const [modelModal, setModelModal] = useState<{ open: boolean; editing?: LLMModel }>({ open: false })
  const [providerForm] = Form.useForm()
  const [modelForm] = Form.useForm()

  const loadProviders = useCallback(async () => {
    setLoading(true)
    try {
      const list = await llmApi.listProviders()
      setProviders(list)
      setSelected((prev) => (prev && list.some((p) => p.id === prev) ? prev : list.length ? list[0].id : null))
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [])

  const loadCurrent = useCallback(async () => {
    try {
      setCurrent(await llmApi.currentModel())
    } catch (e) {
      message.error((e as Error).message)
    }
  }, [])

  const loadModels = useCallback(async (pid: number | null) => {
    if (!pid) {
      setModels([])
      return
    }
    try {
      setModels(await llmApi.listModels(pid))
    } catch (e) {
      message.error((e as Error).message)
    }
  }, [])

  useEffect(() => {
    loadProviders()
    loadCurrent()
  }, [loadProviders, loadCurrent])

  useEffect(() => {
    loadModels(selected)
  }, [selected, loadModels])

  const submitProvider = async () => {
    try {
      const v = await providerForm.validateFields()
      const apiKey = v.api_key?.trim()
      if (providerModal.editing) {
        await llmApi.updateProvider(providerModal.editing.id, {
          name: v.name,
          base_url: v.base_url,
          ...(apiKey ? { api_key: apiKey } : {}),
        })
        message.success('已保存')
      } else {
        await llmApi.createProvider({ name: v.name, base_url: v.base_url, ...(apiKey ? { api_key: apiKey } : {}) })
        message.success('已创建')
      }
      setProviderModal({ open: false })
      providerForm.resetFields()
      loadProviders()
      loadCurrent()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  const submitModel = async () => {
    if (!selected) {
      message.error('请先选择 Provider')
      return
    }
    try {
      const v = await modelForm.validateFields()
      if (modelModal.editing) {
        await llmApi.updateModel(modelModal.editing.id, {
          name: v.name,
          context_window: v.context_window,
          max_tokens: v.max_tokens,
        })
        message.success('已保存')
      } else {
        await llmApi.createModel(selected, {
          name: v.name,
          context_window: v.context_window,
          max_tokens: v.max_tokens,
        })
        message.success('已创建')
      }
      setModelModal({ open: false })
      modelForm.resetFields()
      loadModels(selected)
      loadCurrent()
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    }
  }

  const providerColumns = [
    { title: '名称', dataIndex: 'name' },
    { title: 'Base URL', dataIndex: 'base_url' },
    {
      title: 'API Key',
      render: (_: unknown, p: LLMProvider) =>
        p.has_key ? <span>{p.api_key_masked}</span> : <Tag color="orange">未配置</Tag>,
    },
    { title: '模型数', dataIndex: 'model_count' },
    {
      title: '操作',
      render: (_: unknown, p: LLMProvider) => (
        <Space>
          <Button size="small" disabled={selected === p.id} onClick={() => setSelected(p.id)}>
            管理模型
          </Button>
          <Button
            size="small"
            onClick={() => {
              providerForm.setFieldsValue({ name: p.name, base_url: p.base_url, api_key: '' })
              setProviderModal({ open: true, editing: p })
            }}
          >
            编辑
          </Button>
          <Popconfirm
            title="删除该 Provider?(需先删除其下模型)"
            onConfirm={async () => {
              try {
                await llmApi.deleteProvider(p.id)
                message.success('已删除')
                loadProviders()
                loadCurrent()
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

  const modelColumns = [
    { title: '模型名', dataIndex: 'name' },
    { title: '上下文窗口', dataIndex: 'context_window' },
    { title: '输出上限', dataIndex: 'max_tokens' },
    {
      title: '当前',
      render: (_: unknown, m: LLMModel) => (m.is_current ? <Tag color="green">当前</Tag> : null),
    },
    {
      title: '操作',
      render: (_: unknown, m: LLMModel) => (
        <Space>
          <Button
            size="small"
            disabled={m.is_current}
            onClick={async () => {
              try {
                await llmApi.setCurrentModel(m.id)
                message.success('已切换当前模型')
                loadModels(selected)
                loadCurrent()
              } catch (e) {
                message.error((e as Error).message)
              }
            }}
          >
            设为当前
          </Button>
          <Button
            size="small"
            onClick={() => {
              modelForm.setFieldsValue({ name: m.name, context_window: m.context_window, max_tokens: m.max_tokens })
              setModelModal({ open: true, editing: m })
            }}
          >
            编辑
          </Button>
          <Popconfirm
            title="删除该模型?"
            onConfirm={async () => {
              try {
                await llmApi.deleteModel(m.id)
                message.success('已删除')
                loadModels(selected)
                loadCurrent()
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
      <Card size="small" title="当前模型" style={{ marginBottom: 16 }}>
        {current?.model && current.provider ? (
          <Descriptions column={3} size="small">
            <Descriptions.Item label="Provider">{current.provider.name}</Descriptions.Item>
            <Descriptions.Item label="Base URL">{current.provider.base_url}</Descriptions.Item>
            <Descriptions.Item label="模型">{current.model.name}</Descriptions.Item>
            <Descriptions.Item label="上下文窗口">{current.model.context_window}</Descriptions.Item>
            <Descriptions.Item label="输出上限">{current.model.max_tokens}</Descriptions.Item>
            <Descriptions.Item label="API Key">{current.provider.has_key ? '已配置' : '未配置'}</Descriptions.Item>
          </Descriptions>
        ) : (
          <span style={{ color: '#64748b' }}>未配置(请新建 Provider 与模型并设为当前)</span>
        )}
      </Card>

      <Space style={{ marginBottom: 8 }}>
        <strong>Provider</strong>
        <Button
          type="primary"
          size="small"
          icon={<PlusOutlined />}
          onClick={() => {
            providerForm.resetFields()
            setProviderModal({ open: true })
          }}
        >
          新建 Provider
        </Button>
      </Space>
      <Table rowKey="id" size="small" columns={providerColumns} dataSource={providers} loading={loading} pagination={false} />

      <Space style={{ margin: '16px 0 8px' }}>
        <strong>模型</strong>
        <Button
          type="primary"
          size="small"
          icon={<PlusOutlined />}
          disabled={!selected}
          onClick={() => {
            modelForm.resetFields()
            modelForm.setFieldsValue({ context_window: 384000, max_tokens: 16384 })
            setModelModal({ open: true })
          }}
        >
          新建模型
        </Button>
      </Space>
      <Table rowKey="id" size="small" columns={modelColumns} dataSource={models} pagination={false} />

      <Modal
        title={providerModal.editing ? '编辑 Provider' : '新建 Provider'}
        open={providerModal.open}
        onOk={submitProvider}
        onCancel={() => setProviderModal({ open: false })}
        destroyOnClose
      >
        <Form form={providerForm} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item
            name="base_url"
            label="Base URL"
            extra="须为 Anthropic 兼容端点,如 https://api.deepseek.com/anthropic"
            rules={[{ required: true }]}
          >
            <Input />
          </Form.Item>
          <Form.Item name="api_key" label="API Key" extra={providerModal.editing ? '留空表示不修改' : '可稍后填写'}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={modelModal.editing ? '编辑模型' : '新建模型'}
        open={modelModal.open}
        onOk={submitModel}
        onCancel={() => setModelModal({ open: false })}
        destroyOnClose
      >
        <Form form={modelForm} layout="vertical">
          <Form.Item name="name" label="模型名" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="context_window" label="上下文窗口" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item
            name="max_tokens"
            label="输出上限"
            dependencies={['context_window']}
            rules={[
              { required: true },
              ({ getFieldValue }) => ({
                validator(_, value: number) {
                  if (!value || value <= 8192) {
                    return Promise.reject(new Error('须大于 8192(思考预算)'))
                  }
                  const cw = getFieldValue('context_window') as number
                  if (cw && value >= cw) {
                    return Promise.reject(new Error('须小于上下文窗口'))
                  }
                  return Promise.resolve()
                },
              }),
            ]}
          >
            <InputNumber min={8193} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
