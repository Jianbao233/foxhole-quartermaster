<template>
  <div ref="chartEl" :style="{ width: '100%', height: height + 'px' }"></div>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch, nextTick } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import {
  TooltipComponent,
  GridComponent,
  LegendComponent,
  DataZoomComponent,
  TitleComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([
  LineChart,
  TooltipComponent,
  GridComponent,
  LegendComponent,
  DataZoomComponent,
  TitleComponent,
  CanvasRenderer,
])

interface Series {
  name: string
  data: [number, number][]
}

const props = defineProps<{
  series: Series[]
  height?: number
  title?: string
}>()

const height = props.height ?? 280

const chartEl = ref<HTMLDivElement>()
let chart: echarts.ECharts | null = null

function buildOption() {
  return {
    backgroundColor: 'transparent',
    title: props.title
      ? { text: props.title, textStyle: { color: '#d8dde4', fontSize: 14 } }
      : undefined,
    tooltip: {
      trigger: 'axis',
      backgroundColor: '#1d2228',
      borderColor: '#303942',
      textStyle: { color: '#d8dde4' },
    },
    legend: { textStyle: { color: '#b6bcc6' }, top: props.title ? 28 : 0 },
    grid: { left: 56, right: 24, top: props.title ? 64 : 36, bottom: 56 },
    xAxis: {
      type: 'time',
      axisLine: { lineStyle: { color: '#303942' } },
      axisLabel: { color: '#b6bcc6' },
      splitLine: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLine: { lineStyle: { color: '#303942' } },
      axisLabel: { color: '#b6bcc6' },
      splitLine: { lineStyle: { color: '#262d35' } },
    },
    dataZoom: [
      { type: 'inside' },
      {
        type: 'slider',
        height: 18,
        bottom: 12,
        textStyle: { color: '#b6bcc6' },
        borderColor: '#303942',
        fillerColor: 'rgba(103,194,58,0.15)',
        handleStyle: { color: '#67c23a' },
      },
    ],
    series: props.series.map((s, i) => ({
      type: 'line',
      name: s.name,
      data: s.data,
      smooth: true,
      symbol: 'circle',
      symbolSize: 6,
      lineStyle: { width: 2 },
      itemStyle: { color: i === 0 ? '#67c23a' : '#409eff' },
      areaStyle: i === 0 ? { color: 'rgba(103,194,58,0.08)' } : undefined,
    })),
  }
}

function render() {
  if (!chart) return
  chart.setOption(buildOption(), true)
}

onMounted(async () => {
  await nextTick()
  if (!chartEl.value) return
  chart = echarts.init(chartEl.value, undefined, { renderer: 'canvas' })
  render()
  window.addEventListener('resize', onResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  if (chart) {
    chart.dispose()
    chart = null
  }
})

function onResize() {
  chart?.resize()
}

watch(() => props.series, render, { deep: true })
watch(() => props.title, render)
</script>