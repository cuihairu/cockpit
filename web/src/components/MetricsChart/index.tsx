import React, { useEffect, useRef } from 'react';
import * as echarts from 'echarts/core';
import { LineChart } from 'echarts/charts';
import {
  GridComponent,
  TitleComponent,
  TooltipComponent,
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import type { LineSeriesOption } from 'echarts/charts';
import type {
  GridComponentOption,
  TitleComponentOption,
  TooltipComponentOption,
} from 'echarts/components';
import type { ComposeOption, ECharts } from 'echarts/core';
import { Spin } from 'antd';

echarts.use([LineChart, GridComponent, TitleComponent, TooltipComponent, CanvasRenderer]);

type MetricsChartOption = ComposeOption<
  LineSeriesOption | GridComponentOption | TitleComponentOption | TooltipComponentOption
>;

interface MetricsChartProps {
  title: string;
  data: Array<{ time: string; value: number }>;
  unit?: string;
  color?: string;
  height?: number;
  loading?: boolean;
  min?: number;
  max?: number;
}

const MetricsChart: React.FC<MetricsChartProps> = ({
  title,
  data,
  unit = '%',
  color = '#1890ff',
  height = 200,
  loading = false,
  min,
  max,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<ECharts | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const chart = echarts.init(containerRef.current);
    chartRef.current = chart;

    const resize = () => {
      chart.resize();
    };
    window.addEventListener('resize', resize);

    return () => {
      window.removeEventListener('resize', resize);
      chart.dispose();
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    const option: MetricsChartOption = {
      title: {
        text: title,
        left: 'center',
        textStyle: {
          fontSize: 14,
          fontWeight: 'normal',
        },
      },
      tooltip: {
        trigger: 'axis',
        formatter: (params: unknown) => {
          if (!Array.isArray(params) || !params[0] || typeof params[0] !== 'object') {
            return '';
          }
          const param = params[0];
          const item = param as { name?: string; seriesName?: string; value?: string | number };
          return `${item.name ?? ''}<br/>${item.seriesName ?? ''}: ${item.value ?? ''}${unit}`;
        },
      },
      grid: {
        left: '50px',
        right: '20px',
        bottom: '30px',
        top: '50px',
        containLabel: true,
      },
      xAxis: {
        type: 'category',
        data: data.map((d) => d.time),
        axisLabel: {
          formatter: (value: string) => {
            const date = new Date(value);
            return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`;
          },
        },
      },
      yAxis: {
        type: 'value',
        min: min ?? 0,
        max: max ?? 100,
        axisLabel: {
          formatter: `{value}${unit}`,
        },
      },
      series: [
        {
          name: title,
          type: 'line',
          smooth: true,
          data: data.map((d) => d.value),
          itemStyle: {
            color,
          },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: color + '40' },
                { offset: 1, color: color + '05' },
              ],
            },
          },
        },
      ],
    };

    chartRef.current?.setOption(option, true, true);
  }, [color, data, max, min, title, unit]);

  return (
    <Spin spinning={loading}>
      <div ref={containerRef} style={{ height: `${height}px` }} />
    </Spin>
  );
};

export default MetricsChart;
