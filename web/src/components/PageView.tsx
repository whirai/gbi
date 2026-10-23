import { useEffect, useState } from 'react'
import { message, Spin } from 'antd'
import type { PageDetail } from '../api/pages'
import { getPage } from '../api/pages'
import G2Chart from './G2Chart'

// 12 列栅格:full=12, 1/2=6, 1/3=4, 2/3=8
const widthSpan: Record<string, string> = {
  full: 'span 12',
  '1/2': 'span 6',
  '1/3': 'span 4',
  '2/3': 'span 8',
}

export default function PageView({ pageId }: { pageId: number }) {
  const [page, setPage] = useState<PageDetail | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let active = true
    setLoading(true)
    setPage(null)
    getPage(pageId)
      .then((p) => {
        // 取消守卫:pageId 变化/卸载后,迟到的响应不得写入陈旧页面
        if (active) setPage(p)
      })
      .catch((e) => message.error((e as Error).message))
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [pageId])

  if (loading)
    return (
      <div style={{ padding: 60, textAlign: 'center' }}>
        <Spin />
      </div>
    )
  if (!page) return null

  return (
    <div>
      <h2 style={{ margin: '0 0 4px', color: '#0c4a6e' }}>{page.name}</h2>
      {page.desc && <div style={{ color: '#64748b', marginBottom: 12 }}>{page.desc}</div>}
      {page.config.charts.length === 0 ? (
        <div style={{ color: '#64748b', padding: 40, textAlign: 'center' }}>该页面暂无图表</div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(12, 1fr)', gap: 12 }}>
          {page.config.charts.map((c) => (
            <div
              key={c.id}
              className="glass"
              style={{ padding: 12, gridColumn: widthSpan[c.width] ?? 'span 12', minHeight: 280 }}
            >
              <div style={{ fontFamily: 'SimHei', fontWeight: 600, marginBottom: 8, color: '#0c4a6e' }}>
                {c.title}
              </div>
              <G2Chart chart={c} pageId={pageId} />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
