import { useEffect, useRef, useState } from 'react'
import { Input, message, Modal, Radio, Select, Upload } from 'antd'
import { InboxOutlined } from '@ant-design/icons'
import type { UploadFile } from 'antd'
import * as subjectsApi from '../../../api/subjects'
import type { TableDTO } from '../../../api/subjects'

export default function ImportModal({
  open,
  subjectId,
  tables,
  onClose,
  onDone,
}: {
  open: boolean
  subjectId: number
  tables: TableDTO[]
  onClose: () => void
  onDone: () => void
}) {
  const [mode, setMode] = useState<'create' | 'append'>('create')
  const [file, setFile] = useState<File | null>(null)
  const [fileList, setFileList] = useState<UploadFile[]>([])
  const [tableName, setTableName] = useState('')
  const [kind, setKind] = useState('DWD')
  const [tableId, setTableId] = useState<number | undefined>(undefined)
  const [busy, setBusy] = useState(false)
  // tables 经 ref 读取最新值:重置只随 open 触发,父级刷新表列表
  // (setTables 产生新数组引用)不会在开放中清空已填表单
  const tablesRef = useRef(tables)
  tablesRef.current = tables

  useEffect(() => {
    if (open) {
      setMode('create')
      setFile(null)
      setFileList([])
      setTableName('')
      setKind('DWD')
      setTableId(tablesRef.current[0]?.id)
    }
  }, [open])

  const submit = async () => {
    if (!file) {
      message.error('请选择 .xlsx 文件')
      return
    }
    if (mode === 'create' && !tableName) {
      message.error('请输入表名')
      return
    }
    if (mode === 'append' && tableId === undefined) {
      message.error('请选择目标表')
      return
    }
    setBusy(true)
    try {
      const params: Record<string, string> = { mode }
      if (mode === 'create') {
        params.table_name = tableName
        params.kind = kind
      } else {
        params.table_id = String(tableId)
      }
      const res = await subjectsApi.importExcel(subjectId, file, params)
      message.success(`导入成功,共 ${res.inserted} 行`)
      onDone()
      onClose()
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="导入 Excel" open={open} onOk={submit} onCancel={onClose} confirmLoading={busy} destroyOnClose>
      <Upload.Dragger
        accept=".xlsx"
        maxCount={1}
        fileList={fileList}
        beforeUpload={(f) => {
          if (!f.name.toLowerCase().endsWith('.xlsx')) {
            message.error('仅支持 .xlsx')
            return Upload.LIST_IGNORE
          }
          setFile(f)
          setFileList([{ uid: '-1', name: f.name, status: 'done' }])
          return false // 手动上传
        }}
        onRemove={() => {
          setFile(null)
          setFileList([])
        }}
      >
        <p className="ant-upload-drag-icon">
          <InboxOutlined />
        </p>
        <p>点击或拖拽 .xlsx 文件到此处(首行为表头)</p>
      </Upload.Dragger>

      <Radio.Group value={mode} onChange={(e) => setMode(e.target.value)} style={{ marginTop: 12 }}>
        <Radio value="create">导入并建新表(自动推断列类型)</Radio>
        <Radio value="append">导入已有表(按列名匹配追加)</Radio>
      </Radio.Group>

      {mode === 'create' ? (
        <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
          <Input
            placeholder="表名(不含前缀)"
            value={tableName}
            onChange={(e) => setTableName(e.target.value)}
            style={{ width: 220 }}
          />
          <Radio.Group value={kind} onChange={(e) => setKind(e.target.value)}>
            <Radio.Button value="DWD">DWD</Radio.Button>
            <Radio.Button value="DIM">DIM</Radio.Button>
          </Radio.Group>
        </div>
      ) : (
        <Select
          style={{ width: '100%', marginTop: 12 }}
          placeholder="选择目标表"
          value={tableId}
          onChange={setTableId}
          options={tables.map((t) => ({ value: t.id, label: t.name }))}
        />
      )}
    </Modal>
  )
}
