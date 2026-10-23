import { useEffect, useState } from 'react'
import { Button, Card, Form, Input, message, Popconfirm, Space, Upload } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
import * as brandingApi from '../../api/branding'
import { useBranding } from '../../store/branding'

export default function Branding() {
  const { systemName, browserTitle, logoUrl, faviconUrl, load } = useBranding()
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [uploading, setUploading] = useState<'logo' | 'favicon' | null>(null)

  useEffect(() => {
    form.setFieldsValue({ system_name: systemName, browser_title: browserTitle })
  }, [form, systemName, browserTitle])

  const save = async () => {
    try {
      const v = await form.validateFields()
      setSaving(true)
      await brandingApi.updateBranding({ system_name: v.system_name, browser_title: v.browser_title })
      await load()
      message.success('已保存')
    } catch (e) {
      if ((e as { errorFields?: unknown })?.errorFields) return
      message.error((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const uploadImage = async (kind: 'logo' | 'favicon', file: File) => {
    try {
      setUploading(kind)
      await brandingApi.uploadBrandingImage(kind, file)
      await load()
      message.success('已上传')
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setUploading(null)
    }
    return false // 阻止 antd 默认上传
  }

  const removeImage = async (kind: 'logo' | 'favicon') => {
    try {
      await brandingApi.deleteBrandingImage(kind)
      await load()
      message.success('已恢复默认图片')
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const resetAll = async () => {
    try {
      await brandingApi.resetBranding()
      await load()
      message.success('已恢复默认')
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const imageCard = (kind: 'logo' | 'favicon', title: string, url: string | null) => (
    <Card size="small" title={title} style={{ width: 280 }}>
      <div style={{ height: 72, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12, background: '#f8fafc', borderRadius: 8 }}>
        {url ? <img src={url} alt={title} style={{ maxWidth: 240, maxHeight: 64 }} /> : <span style={{ color: '#94a3b8' }}>未上传</span>}
      </div>
      <Space>
        <Upload
          accept=".png,.ico"
          showUploadList={false}
          beforeUpload={(file) => uploadImage(kind, file)}
        >
          <Button size="small" icon={<UploadOutlined />} loading={uploading === kind}>
            上传(png/ico)
          </Button>
        </Upload>
        {url && (
          <Popconfirm title="恢复默认图片?" onConfirm={() => removeImage(kind)}>
            <Button size="small" danger>
              恢复默认
            </Button>
          </Popconfirm>
        )}
      </Space>
    </Card>
  )

  return (
    <div>
      <Card size="small" title="系统名称与浏览器标题" style={{ maxWidth: 560, marginBottom: 16 }}>
        <Form form={form} layout="vertical">
          <Form.Item name="system_name" label="系统名称" rules={[{ required: true, max: 32 }]}>
            <Input />
          </Form.Item>
          <Form.Item name="browser_title" label="浏览器标题" rules={[{ required: true, max: 64 }]}>
            <Input />
          </Form.Item>
          <Button type="primary" onClick={save} loading={saving}>
            保存
          </Button>
        </Form>
      </Card>

      <Space align="start" style={{ marginBottom: 16 }}>
        {imageCard('logo', 'Logo', logoUrl)}
        {imageCard('favicon', 'Favicon', faviconUrl)}
      </Space>

      <div>
        <Popconfirm title="恢复全部默认(名称与图片)?" onConfirm={resetAll}>
          <Button danger>恢复全部默认</Button>
        </Popconfirm>
      </div>
    </div>
  )
}
