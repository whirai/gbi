import { useCallback, useEffect, useState } from 'react'
import { Button, Card, message, Popconfirm, Space, Spin, Tabs, Tag } from 'antd'
import { PlusOutlined, UploadOutlined } from '@ant-design/icons'
import * as subjectsApi from '../../../api/subjects'
import type { TableDTO } from '../../../api/subjects'
import type { Subject } from '../../../api/types'
import CreateTableModal from './CreateTableModal'
import ImportModal from './ImportModal'
import RowsGrid from './RowsGrid'

export default function SubjectDetail({ subject }: { subject: Subject }) {
  const [tables, setTables] = useState<TableDTO[]>([])
  const [active, setActive] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const list = await subjectsApi.listTables(subject.id)
      setTables(list)
      setActive((prev) => (list.some((t) => String(t.id) === prev) ? prev : list.length ? String(list[0].id) : ''))
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [subject.id])

  useEffect(() => {
    load()
  }, [load])

  const removeTable = async (t: TableDTO) => {
    try {
      await subjectsApi.removeTable(subject.id, t.id)
      message.success('已删除')
      load()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  return (
    <Card
      size="small"
      title={
        <Space>
          <span style={{ fontFamily: 'SimHei' }}>{subject.name}</span>
          <Tag color="blue">{subject.code}</Tag>
        </Space>
      }
      extra={
        <Space>
          <Button size="small" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            新建表
          </Button>
          <Button size="small" icon={<UploadOutlined />} onClick={() => setImportOpen(true)}>
            导入Excel
          </Button>
        </Space>
      }
    >
      <Spin spinning={loading}>
        {tables.length === 0 ? (
          <div style={{ color: '#64748b', padding: 24, textAlign: 'center' }}>
            暂无表,请新建表或导入 Excel
          </div>
        ) : (
          <Tabs
            activeKey={active}
            onChange={setActive}
            items={tables.map((t) => ({
              key: String(t.id),
              label: (
                <span>
                  {t.name} <Tag style={{ marginLeft: 4 }}>{t.kind}</Tag>
                </span>
              ),
              children: (
                <div>
                  <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 8 }}>
                    <Popconfirm title={`删除表 ${t.name} 及其全部数据?`} onConfirm={() => removeTable(t)}>
                      <Button size="small" danger>
                        删除表
                      </Button>
                    </Popconfirm>
                  </div>
                  <RowsGrid subjectId={subject.id} table={t} />
                </div>
              ),
            }))}
          />
        )}
      </Spin>

      <CreateTableModal open={createOpen} subjectId={subject.id} onClose={() => setCreateOpen(false)} onDone={load} />
      <ImportModal open={importOpen} subjectId={subject.id} tables={tables} onClose={() => setImportOpen(false)} onDone={load} />
    </Card>
  )
}
