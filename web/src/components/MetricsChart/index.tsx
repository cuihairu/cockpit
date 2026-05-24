import React, { useEffect, useRef } from 'react';
import ReactECharts from 'echarts-for-react';
import type { EChartsOption } from 'echarts';
import { Spin } from 'antd';

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
  const chartRef = useRef<any>(null);

  const option: EChartsOption = {
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
      formatter: (params: any) => {
        const param = params[0];
        return `${param.name}<br/>${param.seriesName}: ${param.value}${unit}`;
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

  return (
    <Spin spinning={loading}>
      <ReactECharts
        ref={chartRef}
        option={option}
        style={{ height: `${height}px` }}
        notMerge={true}
        lazyUpdate={true}
      />
    </Spin>
  );
};

export default MetricsChart;
