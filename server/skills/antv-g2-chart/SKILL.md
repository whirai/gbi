---
name: antv-g2-chart
description: "Use this skill whenever the user wants to create, customize, or troubleshoot G2 v5 chart visualizations. Triggers include: any mention of 'G2', 'antv g2', '@antv/g2', 'G2 chart', 'G2 可视化', or requests to produce charts like bar charts (柱状图), line charts (折线图), pie charts (饼图), scatter plots (散点图), area charts (面积图), heatmap (热力图), radar charts (雷达图), treemap (矩形树图), funnel charts (漏斗图), sankey diagrams (桑基图), gauge (仪表盘), wordcloud (词云), boxplot (箱线图), as well as G2-specific topics like encode channels, scale config, coordinate systems, transforms, interactions, themes, labels, and animations. Also use when debugging G2 rendering errors, V4→V5 migration issues, or chart type selection. Do NOT use for G6 graph/network visualization, X6 editor diagrams, or S2 pivot tables."
tools:
  - curl
---

# G2 v5 Chart Visualization

## Overview

G2 v5 is AntV's grammar-of-graphics charting library. It uses **Spec Mode** — a declarative, JSON-like configuration style where `chart.options()` defines the entire visualization in one call.

```javascript
import { Chart } from '@antv/g2';

const chart = new Chart({ container: 'container', autoFit: true });
chart.options({
  type: 'interval',
  data: [{ genre: 'Sports', sold: 275 }],
  encode: { x: 'genre', y: 'sold' },
});
chart.render();
```

### CDN Usage

```html
<script src="https://unpkg.com/@antv/g2@5/dist/g2.min.js"></script>
<script>
  const chart = new G2.Chart({ container: 'container', autoFit: true });
  chart.options({
    type: 'interval',
    data: [{ genre: 'Sports', sold: 275 }],
    encode: { x: 'genre', y: 'sold' },
  });
  chart.render();
</script>
```

## Content Retrieval Service

When using AntV G2 for data visualization, if you need to understand the concepts, usage, API, examples, and other aspects of G2 v5, you can use the provided context retrieval service. When using the skill, content is retrieved via an antv HTTP API server using GET requests.

- Host: `https://sive.antv.antgroup.com`
- Endpoint: `/api/v1/context/retrieve`
- Method: `GET`
- Parameters: `query`, `library`, `topK`, `content`, `maxTokens`, `progressiveLevel`

Retrieve skills by query (hybrid search = FTS + vector + RRF fusion). Constraints docs are indexed as regular skill documents and will appear in search results naturally.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `query` | string | ✅ | Search keywords, e.g. `bar chart interval` |
| `library` | string | ✅ | Library name: `g2`, `g6`, `x6` |
| `topK` | number | | Number of results to return (default: 5) |
| `content` | boolean | | Return full reference doc markdown (default: true) |
| `maxTokens` | number | | Max tokens per result (default: unlimited) |
| `progressiveLevel` | number | | Progressive disclosure level: `0`=full, `1`=summary+code, `2`=summary-only |

```bash
curl "https://sive.antv.antgroup.com/api/v1/context/retrieve?query=bar+chart+stacked&library=g2"
```

## Critical Rules

### MUST: Use V5 Spec Mode ONLY

```javascript
// ❌ WRONG — V4 chain API (deprecated, will not render)
chart.interval()
  .data([...])
  .encode('x', 'genre')
  .encode('y', 'sold')
  .style({ radius: 4 });

// ✅ CORRECT — V5 Spec Mode
chart.options({
  type: 'interval',
  data: [...],
  encode: { x: 'genre', y: 'sold' },
  style: { radius: 4 },
});
```

### MUST: `chart.options()` called exactly ONCE

Multiple calls **overwrite** each other. For multi-mark overlays, use `type: 'view'` + `children`:

```javascript
// ❌ WRONG — second options() overwrites the first
chart.options({ type: 'line', data, encode: { x: 'date', y: 'value' } });
chart.options({ type: 'point', data, encode: { x: 'date', y: 'value' } });

// ✅ CORRECT — children array for multi-mark
chart.options({
  type: 'view',
  data,
  children: [
    { type: 'line',  encode: { x: 'date', y: 'value' } },
    { type: 'point', encode: { x: 'date', y: 'value' } },
  ],
});
```

### MUST: `container` is mandatory, `chart.render()` at the end

```javascript
// ❌ WRONG — no container, no render
const chart = new Chart();
chart.options({ type: 'interval', data });

// ✅ CORRECT
const chart = new Chart({ container: 'container', autoFit: true });
chart.options({ type: 'interval', data, encode: { x: 'genre', y: 'sold' } });
chart.render();
```

### MUST: Correct mark types only

| ❌ Hallucinated (from ECharts/Vega) | ✅ G2 correct replacement |
|---|---|
| `type: 'ruleX'` | `type: 'lineX'` |
| `type: 'ruleY'` | `type: 'lineY'` |
| `type: 'regionX'` | `type: 'rangeX'` |
| `type: 'regionY'` | `type: 'rangeY'` |
| `type: 'venn'` | `type: 'path'` + transform |

**Legal G2 marks**: `interval`, `line`, `area`, `point`, `rect`, `cell`, `text`, `image`, `path`, `polygon`, `shape`, `link`, `connector`, `vector`, `lineX`, `lineY`, `rangeX`, `rangeY`, `range`, `box`, `boxplot`, `density`, `heatmap`, `beeswarm`, `treemap`, `pack`, `partition`, `tree`, `sankey`, `chord`, `wordCloud`, `gauge`, `liquid`. `sunburst` requires `@antv/g2-extension-plot`.

### MUST: `encode` is an object, `transform` is an array

```javascript
// ❌ WRONG
.encode('x', 'genre')
.transform: { type: 'stackY' }

// ✅ CORRECT
encode: { x: 'genre', y: 'sold' }
transform: [{ type: 'stackY' }]
```

### MUST: `labels` is plural, range encoding uses y/y1

```javascript
// ❌ WRONG
label: { text: 'sold' }
encode: { y: ['start', 'end'] }

// ✅ CORRECT
labels: [{ text: 'sold' }]
encode: { y: 'start', y1: 'end' }
```

### MUST: No d3 in user code

```javascript
// ❌ WRONG — d3 is not exposed in user scope
const total = d3.sum(data, d => d.value);

// ✅ CORRECT — use native JS or G2 built-in transforms
const total = data.reduce((sum, d) => sum + d.value, 0);
```

### MUST: No white/near-white fill, no `padding` as array

```javascript
// ❌ WRONG
style: { fill: '#fff' }       // invisible on white background
padding: [40, 30, 40, 50]     // invalid in G2 v5

// ✅ CORRECT
encode: { color: 'group' }    // let G2 assign colors
padding: 40                   // single number or 'auto'
```

### MUST: Transpose is a transform, not a coordinate type

```javascript
// ❌ WRONG
coordinate: { type: 'transpose' }

// ✅ CORRECT
coordinate: { transform: [{ type: 'transpose' }] }
```

## GBI 平台约束(update_page 会硬校验,违反即被拒绝)

### MUST: encode 每个字段名必须与对应图表 SQL 的返回列名完全一致

`update_page` 工具会实际执行每个图表的 SQL 并核对返回列;不一致会直接拒绝并
给出「字段 X 不在 SQL 返回列中(返回列: …)」的报错。写 spec 前先对照
`execute_sql` 输出的 columns。推荐 SQL 别名用英文短名(category/value/total),
避免中文列名带来的歧义。

```javascript
// SQL: SELECT Province AS x, SUM(CASE WHEN FuelType='柴油' THEN Quantity ELSE 0 END) AS diesel FROM t GROUP BY Province
// ❌ WRONG — 列名是 diesel,不是"燃料"也不是"柴油"
encode: { x: 'x', y: '柴油', color: '燃料' }

// ✅ CORRECT
encode: { x: 'x', y: 'diesel' }
```

### MUST: theta 环形图的 innerRadius 在 coordinate 上,不在 style/encode

```javascript
// ✅ CORRECT — G2 v5 环形图
coordinate: { type: 'theta', innerRadius: 0.6, outerRadius: 0.95 },
encode: { y: 'value', color: 'category' }
```

### MUST: stackY 堆叠要求 encode.color 指定分类字段

stackY 按 color 通道分组堆叠;color 字段名必须存在于 SQL 返回列中。没有 color
通道的 stackY 堆叠不生效(柱状图只显示坐标轴是典型症状)。

### MUST: 单图无法表达多层同心环

「占比高的放内环、占比低的放外环」的多层嵌套环,单个 theta 图无法表达
(innerRadius 是全局的)。如实向用户说明限制,或改用:一个 theta 图(单环)+
把分层需求简化,或每个类别一个独立环形图并排。



## Quick Reference

| User Intent | Retrieve Query |
|---|---|
| Chart initialization, container, autoFit | `GET /api/v1/context/retrieve?query=chart+init&library=g2` |
| Bar / column chart | `GET /api/v1/context/retrieve?query=bar+chart+interval&library=g2` |
| Line / area chart | `GET /api/v1/context/retrieve?query=line+area+chart&library=g2` |
| Pie / donut / rose chart | `GET /api/v1/context/retrieve?query=pie+chart+theta&library=g2` |
| Scatter / bubble | `GET /api/v1/context/retrieve?query=scatter+point+bubble&library=g2` |
| Treemap / sunburst / pack | `GET /api/v1/context/retrieve?query=treemap+sunburst+pack&library=g2` |
| Heatmap / density / boxplot | `GET /api/v1/context/retrieve?query=heatmap+density+boxplot&library=g2` |
| Funnel / gauge / wordcloud | `GET /api/v1/context/retrieve?query=funnel+gauge+wordcloud&library=g2` |
| Encode channels (x, y, color, size) | `GET /api/v1/context/retrieve?query=encode+channel&library=g2` |
| Scale / palette / color range | `GET /api/v1/context/retrieve?query=scale+palette+color&library=g2` |
| Coordinate (polar, theta, transpose) | `GET /api/v1/context/retrieve?query=coordinate+polar+theta+transpose&library=g2` |
| Transform (stack, normalize, sort) | `GET /api/v1/context/retrieve?query=transform+stack+normalize&library=g2` |
| Axis / legend / tooltip / labels | `GET /api/v1/context/retrieve?query=axis+legend+tooltip+label&library=g2` |
| Interaction (brush, highlight, drilldown) | `GET /api/v1/context/retrieve?query=interaction+brush+highlight&library=g2` |
| Theme / dark mode | `GET /api/v1/context/retrieve?query=theme+dark+classicDark&library=g2` |
| Animation | `GET /api/v1/context/retrieve?query=animation+animate&library=g2` |
| Data fetch / filter / sort | `GET /api/v1/context/retrieve?query=data+fetch+filter+sort&library=g2` |
| Facet / view composition | `GET /api/v1/context/retrieve?query=facet+view+composition&library=g2` |
| Chart type selection guide | `GET /api/v1/context/retrieve?query=chart+type+selection&library=g2` |
| Rendering troubleshoot | `GET /api/v1/context/retrieve?query=rendering+troubleshoot+debug&library=g2` |

## Dependencies

- `@antv/g2` — G2 v5 charting engine