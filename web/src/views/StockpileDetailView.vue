<template>
  <div v-loading="loading">
    <div v-if="detail">
      <div class="flex-row" style="justify-content: space-between; margin-bottom: 12px">
        <div>
          <h2 class="section-title" style="margin: 0">
            {{ detail.stockpile.region }} - {{ detail.stockpile.sub_region }}
          </h2>
          <div class="muted" style="margin-top: 4px">
            {{ detail.stockpile.building_name }} · {{ detail.stockpile.player_alias }} ·
            <el-tag :type="detail.stockpile.visibility === 'Public' ? 'success' : 'warning'" size="small">
              {{ detail.stockpile.visibility }}
            </el-tag>
            · 最新快照 {{ formatTs(detail.stockpile.captured_at) }}
          </div>
        </div>
        <el-button :icon="Refresh" @click="load">刷新</el-button>
      </div>

      <el-input
        v-model="filter"
        placeholder="搜索物品名 / 分类"
        clearable
        size="small"
        style="width: 280px; margin-bottom: 12px"
      />

      <el-table
        :data="filteredItems"
        stripe
        row-key="canonical_id"
        @expand-change="onExpandChange"
      >
        <el-table-column type="expand" width="40">
          <template #default="{ row }">
            <div style="padding: 8px 24px">
              <TimelineChart
                v-if="timelines[row.canonical_id]"
                :series="timelines[row.canonical_id]"
                :title="`${row.name_zh} 时间轴`"
              />
              <div v-else class="muted">加载图表中...</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="" width="50">
          <template #default="{ row }">
            <IconImg :canonical-id="row.canonical_id" :alt="row.name_zh" :size="28" />
          </template>
        </el-table-column>
        <el-table-column label="物品" min-width="200">
          <template #default="{ row }">
            <router-link :to="`/items/${row.canonical_id}`" style="color: var(--el-color-primary)">
              {{ row.name_zh }}
            </router-link>
            <span v-if="row.name_en" class="muted" style="margin-left: 6px">{{ row.name_en }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="category" label="分类" width="120" />
        <el-table-column label="散件" width="100" align="right">
          <template #default="{ row }">{{ row.loose }}</template>
        </el-table-column>
        <el-table-column label="箱装" width="100" align="right">
          <template #default="{ row }">{{ row.crates }}</template>
        </el-table-column>
        <el-table-column label="按散件总计" width="120" align="right">
          <template #default="{ row }">
            <span :class="{ muted: !row.crate_size }">
              {{ row.total }}
              <template v-if="!row.crate_size && row.crates > 0">*</template>
            </span>
          </template>
        </el-table-column>
      </el-table>

      <div class="muted" style="margin-top: 8px; font-size: 12px">
        * = 该物品 crate_size 未维护，箱装数量未折算
      </div>
    </div>

    <el-empty v-else-if="!loading" description="仓库不存在或还没有快照" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, reactive } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { stockpilesApi, type StockpileDetail } from '@/api/stockpiles'
import { showError } from '@/api/client'
import { formatTs } from '@/utils/format'
import IconImg from '@/components/IconImg.vue'
import TimelineChart from '@/components/TimelineChart.vue'

const props = defineProps<{ id: string }>()
const detail = ref<StockpileDetail | null>(null)
const loading = ref(false)
const filter = ref('')
const timelines = reactive<Record<string, { name: string; data: [number, number][] }[]>>({})

const filteredItems = computed(() => {
  if (!detail.value) return []
  const q = filter.value.trim().toLowerCase()
  if (!q) return detail.value.items
  return detail.value.items.filter((it) =>
    [it.name_zh, it.name_en, it.category].some((v) => (v || '').toLowerCase().includes(q))
  )
})

async function load() {
  loading.value = true
  try {
    detail.value = await stockpilesApi.detail(Number(props.id))
    Object.keys(timelines).forEach((k) => delete timelines[k])
  } catch (e) {
    detail.value = null
    showError(e, '加载仓库详情失败')
  } finally {
    loading.value = false
  }
}

async function onExpandChange(row: any, expanded: any) {
  const expandedRows = Array.isArray(expanded) ? expanded : []
  const isExpand = expandedRows.some((r: any) => r.canonical_id === row.canonical_id)
  if (!isExpand) return
  if (timelines[row.canonical_id]) return
  try {
    const r = await stockpilesApi.timeline(Number(props.id), row.canonical_id)
    timelines[row.canonical_id] = r.series
  } catch (e) {
    showError(e, '加载时间轴失败')
  }
}

watch(() => props.id, load)
onMounted(load)
</script>