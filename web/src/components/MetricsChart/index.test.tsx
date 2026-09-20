import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MetricsChart from './index'

// MetricsChart：echarts 折线图封装——init/dispose 生命周期与 setOption 数据映射

const chartInstance = vi.hoisted(() => ({
  setOption: vi.fn(),
  dispose: vi.fn(),
  resize: vi.fn(),
}))
vi.mock('echarts/core', () => ({
  use: vi.fn(),
  init: vi.fn(() => chartInstance),
}))
vi.mock('echarts/charts', () => ({ LineChart: vi.fn() }))
vi.mock('echarts/components', () => ({
  GridComponent: vi.fn(),
  TitleComponent: vi.fn(),
  TooltipComponent: vi.fn(),
}))
vi.mock('echarts/renderers', () => ({ CanvasRenderer: vi.fn() }))

describe('MetricsChart', () => {
  beforeEach(() => {
    chartInstance.setOption.mockClear()
    chartInstance.dispose.mockClear()
  })

  const data = [
    { time: '2026-09-20T10:00:00Z', value: 30 },
    { time: '2026-09-20T10:01:00Z', value: 60 },
  ]

  it('挂载即 init 并 setOption；卸载 dispose', () => {
    const { unmount } = render(<MetricsChart title="CPU" data={data} />)
    expect(chartInstance.setOption).toHaveBeenCalledTimes(1)
    unmount()
    expect(chartInstance.dispose).toHaveBeenCalledTimes(1)
  })

  it('option 映射：标题/xy 轴/序列数据与 min/max', () => {
    render(<MetricsChart title="CPU" data={data} unit="%" color="#f00" min={0} max={100} />)
    const option = chartInstance.setOption.mock.calls[0][0] as {
      title: { text: string }
      xAxis: { data: string[] }
      yAxis: { min: number; max: number }
      series: Array<{ name: string; data: number[]; itemStyle: { color: string } }>
    }
    expect(option.title.text).toBe('CPU')
    expect(option.xAxis.data).toEqual(data.map((d) => d.time))
    expect(option.yAxis).toMatchObject({ min: 0, max: 100 })
    expect(option.series[0].name).toBe('CPU')
    expect(option.series[0].data).toEqual([30, 60])
    expect(option.series[0].itemStyle.color).toBe('#f00')
  })

  it('min/max 缺省 0/100', () => {
    render(<MetricsChart title="M" data={data} />)
    const option = chartInstance.setOption.mock.calls[0][0] as { yAxis: { min: number; max: number } }
    expect(option.yAxis).toMatchObject({ min: 0, max: 100 })
  })

  it('tooltip formatter：数组参数拼 name/seriesName/value+unit；非法参数空串', () => {
    render(<MetricsChart title="M" data={data} unit="ms" />)
    const option = chartInstance.setOption.mock.calls[0][0] as {
      tooltip: { formatter: (p: unknown) => string }
    }
    const fmt = option.tooltip.formatter
    expect(
      fmt([{ name: '10:00', seriesName: 'M', value: 42 }]),
    ).toBe('10:00<br/>M: 42ms')
    expect(fmt(null)).toBe('')
    expect(fmt([])).toBe('')
  })
})
