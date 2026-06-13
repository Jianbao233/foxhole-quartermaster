<template>
  <div v-loading="loading">
    <div v-if="data">
      <div class="flex-row" style="justify-content: space-between; margin-bottom: 12px">
        <div class="flex-row">
          <IconImg :canonical-id="data.item.canonical_id" :size="48" />
          <div>
            <h2 class="section-title" style="margin: 0">{{ data.item.name_zh }}</h2>
            <div class="muted" style="margin-top: 4px">
              {{ data.item.name_en || '-' }} · {{ data.item.category || '-' }} · 箱装
              {{ data.item.crate_size ?? '-' }}
            </div>
          </div>
        </div>
        <div class="flex-row">
          <a v-if="data.item.wiki_url" :href="data.item.wiki_url" target="_blank">
            <el-button>打开 Wiki</el-button>
          </a>
          <el-button :icon="Refresh" @click="load">刷新</el-button>
        </div>
      </div>

      <el-card shadow="never">
        <template #header>
          <span class="section-title" style="margin:0">当前 War 各仓存量</span>
        </template>
        <el-table :data="data.stockpiles || []" stripe>
          <el-table-column prop="region" label="地区" min-width="140" />
          <el-table-column prop="building_name" label="建筑" min-width="160" />
          <el-table-column prop="player_alias" label="别名" min-width="120" />
          <el-table-column label="散件" width="100" align="right">
            <template #default="{ row }">{{ row.loose }}</template>
          </el-table-column>
          <el-table-column label="箱装" width="100" align="right">
            <template #default="{ row }">{{ row.crates }}</template>
          </el-table-column>
          <el-table-column label="最新快照" width="200">
            <template #default="{ row }">{{ formatTs(row.captured_at) }}</template>
          </el-table-column>
          <el-table-column label="" width="100">
            <template #default="{ row }">
              <el-button text type="primary" size="small" @click="$router.push(`/stockpiles/${row.stockpile_id}`)">
                查看
              </el-button>
            </template>
          </el-table-column>
          <template #empty>
            <div class="muted">当前 War 还没有任何仓持有该物品</div>
          </template>
        </el-table>
      </el-card>
    </div>
    <el-empty v-else-if="!loading" description="物品不存在" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { itemsApi, type ItemDetail } from '@/api/items'
import { showError } from '@/api/client'
import { formatTs } from '@/utils/format'
import IconImg from '@/components/IconImg.vue'

const props = defineProps<{ cid: string }>()
const data = ref<ItemDetail | null>(null)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    data.value = await itemsApi.detail(props.cid)
  } catch (e) {
    data.value = null
    showError(e, '加载失败')
  } finally {
    loading.value = false
  }
}

watch(() => props.cid, load)
onMounted(load)
</script>