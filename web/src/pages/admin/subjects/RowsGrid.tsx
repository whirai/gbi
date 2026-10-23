import { useCallback, useEffect, useRef, useState } from 'react'
import { Button, Checkbox, Input, message, Popconfirm, Popover, Select, Space, Table, Tooltip } from 'antd'
import type { TableProps } from 'antd'
import type { FilterDropdownProps } from 'antd/es/table/interface'
import { FilterFilled, SettingOutlined } from '@ant-design/icons'
import * as subjectsApi from '../../../api/subjects'
import type { ColumnDef, RowsFilters, RowsPage, TableDTO } from '../../../api/subjects'
import './rows-grid.css'

const PAGE_SIZE = 20
const PAGE_SIZE_OPTIONS = ['10', '20', '50', '100']

// 固定列宽(tableLayout fixed 下防长文本把列撑爆;横向滚动在表格容器内进行)
const RID_COL_WIDTH = 70
const ACTION_COL_WIDTH = 80
const COL_WIDTH: Record<string, number> = { TEXT: 180, INTEGER: 120, REAL: 140, DATE: 170 }
const colWidth = (type: string) => COL_WIDTH[type] ?? 160

// 列显隐按表记忆(localStorage 不可用时静默降级为仅本次会话生效)
const hiddenKey = (tableId: number) => `gbi.rows_hidden.${tableId}`

function loadHidden(tableId: number): Set<string> {
  try {
    const raw = localStorage.getItem(hiddenKey(tableId))
    return new Set<string>(raw ? (JSON.parse(raw) as string[]) : [])
  } catch {
    return new Set()
  }
}

function saveHidden(tableId: number, hidden: Set<string>) {
  try {
    localStorage.setItem(hiddenKey(tableId), JSON.stringify([...hidden]))
  } catch {
    // 隐私模式等场景写入失败:仅本次会话生效
  }
}

export default function RowsGrid({ subjectId, table }: { subjectId: number; table: TableDTO }) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(PAGE_SIZE)
  const [data, setData] = useState<RowsPage | null>(null)
  const [loading, setLoading] = useState(false)
  // rev 并入单元格 key:保存被拒时服务端值未变,EditableCell 的 old 不变、
  // effect 不触发,只能靠 key 变化强制重挂载以复位输入(load(page) 不足以复位)
  const [rev, setRev] = useState(0)
  // 服务端列筛选(列名 → 候选值);空对象=无筛选
  const [filters, setFilters] = useState<RowsFilters>({})
  // 隐藏列(仅影响展示,不影响筛选/编辑数据)
  const [hidden, setHidden] = useState<Set<string>>(() => loadHidden(table.id))

  const load = useCallback(
    async (p: number, size: number, flt: RowsFilters) => {
      setLoading(true)
      try {
        setData(await subjectsApi.listRows(subjectId, table.id, p, size, flt))
      } catch (e) {
        message.error((e as Error).message)
      } finally {
        setLoading(false)
      }
    },
    [subjectId, table.id],
  )

  useEffect(() => {
    setPage(1)
    setPageSize(PAGE_SIZE)
    setFilters({})
    setHidden(loadHidden(table.id))
    load(1, PAGE_SIZE, {})
  }, [load, table.id])

  const saveCell = async (rowid: number, column: string, raw: string, old: string) => {
    if (raw === old) return
    const col = table.columns.find((c) => c.name === column)
    let value: unknown = raw
    if (col && (col.type === 'INTEGER' || col.type === 'REAL')) {
      if (raw.trim() === '') {
        value = null
      } else {
        const n = Number(raw)
        if (Number.isNaN(n)) {
          message.error(`${column} 需要数字`)
          setRev((r) => r + 1)
          load(page, pageSize, filters)
          return
        }
        value = n
      }
    }
    try {
      await subjectsApi.updateCell(subjectId, table.id, rowid, column, value)
      message.success('已保存')
      load(page, pageSize, filters)
    } catch (e) {
      message.error((e as Error).message)
      setRev((r) => r + 1)
      load(page, pageSize, filters)
    }
  }

  const applyFilters = (next: RowsFilters) => {
    setFilters(next)
    setPage(1)
    load(1, pageSize, next)
  }

  const toggleColumn = (name: string, visible: boolean) => {
    setHidden((prev) => {
      const next = new Set(prev)
      if (visible) next.delete(name)
      else next.add(name)
      saveHidden(table.id, next)
      return next
    })
  }

  // 全选/全不选:仅作用于数据列;`#` 与操作列恒定显示
  const setAllColumns = (visible: boolean) => {
    const next = visible ? new Set<string>() : new Set(table.columns.map((c) => c.name))
    setHidden(next)
    saveHidden(table.id, next)
  }

  const visibleCols = table.columns.filter((c) => !hidden.has(c.name))
  const tableWidth =
    RID_COL_WIDTH + ACTION_COL_WIDTH + visibleCols.reduce((sum, c) => sum + colWidth(c.type), 0)
  const filterCount = Object.keys(filters).length
  const visibleCount = table.columns.filter((c) => !hidden.has(c.name)).length
  const allVisible = visibleCount === table.columns.length
  const someVisible = visibleCount > 0 && visibleCount < table.columns.length

  const columns: TableProps<Record<string, unknown>>['columns'] = [
    {
      title: '#',
      key: '_rid',
      dataIndex: '_rid',
      width: RID_COL_WIDTH,
      fixed: 'left',
      render: (v: unknown) => <span style={{ color: '#94a3b8' }}>{String(v)}</span>,
    },
    ...visibleCols.map((c) => ({
      key: c.name,
      title: <ColumnTitle col={c} />,
      dataIndex: c.name,
      width: colWidth(c.type),
      filteredValue: filters[c.name]?.length ? filters[c.name] : null,
      filterDropdown: ({ confirm }: FilterDropdownProps) => (
        <ColumnFilter
          subjectId={subjectId}
          tableId={table.id}
          column={c.name}
          selected={filters[c.name] ?? []}
          onApply={(vals) => {
            const next = { ...filters }
            if (vals.length > 0) next[c.name] = vals
            else delete next[c.name]
            applyFilters(next)
            confirm({ closeDropdown: true })
          }}
        />
      ),
      filterIcon: (filtered: boolean) => (
        <FilterFilled style={{ color: filtered ? '#1677ff' : undefined }} />
      ),
      // 筛选在服务端执行;antd 仅用受控 filteredValue 展示激活态
      onFilter: () => true,
      render: (v: unknown, row: Record<string, unknown>) => (
        <EditableCell
          key={`${row._rid}-${c.name}-${rev}`}
          value={v}
          onSave={(raw, old) => saveCell(Number(row._rid), c.name, raw, old)}
        />
      ),
    })),
    {
      title: '操作',
      key: '_actions',
      width: ACTION_COL_WIDTH,
      fixed: 'right',
      render: (_: unknown, row: Record<string, unknown>) => (
        <Popconfirm
          title="删除该行?"
          onConfirm={async () => {
            try {
              await subjectsApi.deleteRow(subjectId, table.id, Number(row._rid))
              message.success('已删除')
              // 末页删到空页时回退一页,避免停在不存在的空页
              const nextPage = (data?.rows.length ?? 1) > 1 ? page : Math.max(1, page - 1)
              setPage(nextPage)
              load(nextPage, pageSize, filters)
            } catch (e) {
              message.error((e as Error).message)
            }
          }}
        >
          <a style={{ color: '#ef4444' }}>删除</a>
        </Popconfirm>
      ),
    },
  ]

  return (
    <div className="gbi-rows-grid">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <Space size={8}>
          <Popover
            trigger="click"
            placement="bottomLeft"
            content={
              <div style={{ maxHeight: 320, overflowY: 'auto', minWidth: 180 }}>
                <div style={{ borderBottom: '1px solid #f0f0f0', paddingBottom: 6, marginBottom: 6 }}>
                  <Checkbox
                    checked={allVisible}
                    indeterminate={someVisible}
                    onChange={(e) => setAllColumns(e.target.checked)}
                  >
                    全选
                  </Checkbox>
                </div>
                {table.columns.map((c) => (
                  <div key={c.name}>
                    <Checkbox checked={!hidden.has(c.name)} onChange={(e) => toggleColumn(c.name, e.target.checked)}>
                      {c.name}
                    </Checkbox>
                  </div>
                ))}
              </div>
            }
          >
            <Button size="small" icon={<SettingOutlined />}>
              列设置
            </Button>
          </Popover>
          {filterCount > 0 && (
            <Button size="small" onClick={() => applyFilters({})}>
              清除筛选({filterCount})
            </Button>
          )}
        </Space>
      </div>
      <Table
        rowKey="_rid"
        size="small"
        columns={columns}
        dataSource={data?.rows ?? []}
        loading={loading}
        scroll={{ x: tableWidth }}
        pagination={{
          current: page,
          pageSize,
          total: data?.total ?? 0,
          showSizeChanger: true,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          showTotal: (t) => `共 ${t} 行`,
          onChange: (p, ps) => {
            setPage(p)
            setPageSize(ps)
            load(p, ps, filters)
          },
        }}
      />
    </div>
  )
}

// ColumnTitle 表头:列名超长省略 + Tooltip 展示全名/类型/注释,类型用浅灰小字降噪。
function ColumnTitle({ col }: { col: ColumnDef }) {
  const hint = `${col.name} · ${col.type}${col.comment ? ` · ${col.comment}` : ''}`
  return (
    <Tooltip title={hint}>
      <span
        style={{
          display: 'inline-block',
          maxWidth: colWidth(col.type) - 48,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          verticalAlign: 'bottom',
        }}
      >
        {col.name}
      </span>
      <span style={{ color: '#94a3b8', fontSize: 11, marginLeft: 4 }}>{col.type}</span>
    </Tooltip>
  )
}

// ColumnFilter 列筛选下拉:首次展开时懒加载该列去重候选值,确定后交给服务端筛选。
function ColumnFilter({
  subjectId,
  tableId,
  column,
  selected,
  onApply,
}: {
  subjectId: number
  tableId: number
  column: string
  selected: string[]
  onApply: (vals: string[]) => void
}) {
  const [options, setOptions] = useState<string[] | null>(null)
  const [value, setValue] = useState<string[]>(selected)

  useEffect(() => {
    let alive = true
    subjectsApi
      .listColumnValues(subjectId, tableId, column)
      .then((vals) => {
        if (alive) setOptions(vals)
      })
      .catch((e: Error) => {
        if (alive) {
          setOptions([])
          message.error(e.message)
        }
      })
    return () => {
      alive = false
    }
  }, [subjectId, tableId, column])

  return (
    <div style={{ padding: 8, width: 240 }}>
      <Select
        mode="multiple"
        size="small"
        style={{ width: '100%' }}
        loading={options === null}
        value={value}
        onChange={setValue}
        options={(options ?? []).map((v) => ({ label: v, value: v }))}
        placeholder="选择值"
        maxTagCount="responsive"
        allowClear
        showSearch
      />
      <Space style={{ marginTop: 8, display: 'flex', justifyContent: 'flex-end' }}>
        <Button
          size="small"
          onClick={() => {
            setValue([])
            onApply([])
          }}
        >
          重置
        </Button>
        <Button size="small" type="primary" onClick={() => onApply(value)}>
          确定
        </Button>
      </Space>
    </div>
  )
}

function EditableCell({ value, onSave }: { value: unknown; onSave: (raw: string, old: string) => void }) {
  const old = value === null || value === undefined ? '' : String(value)
  const ref = useRef<string>(old)
  const [v, setV] = useState(old)
  useEffect(() => {
    ref.current = old
    setV(old)
  }, [old])
  return (
    <Input
      size="small"
      variant="borderless"
      value={v}
      title={old} // 原生悬浮提示:宽度受限时仍能看到完整值
      onChange={(e) => setV(e.target.value)}
      onBlur={() => onSave(v, ref.current)}
      onPressEnter={(e) => (e.target as HTMLInputElement).blur()}
    />
  )
}
