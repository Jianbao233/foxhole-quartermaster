<template>
  <div>
    <div class="flex-row" style="justify-content: space-between; margin-bottom: 12px">
      <div>
        <h2 class="section-title" style="margin: 0">
          当前 War 总览
          <el-tag v-if="war" type="success" effect="dark" size="small" style="margin-left:8px">
            {{ war.label }}
          </el-tag>
          <span v-if="war" class="muted" style="margin-left: 8px">
            自 {{ formatTs(war.started_at) }}
          </span>
        </h2>
      </div>
      <div class="flex-row">
        <el-button type="primary" :icon="UploadFilled" @click="$router.push('/upload')">
          上传新仓库
        </el-button>
        <el-button :icon="Refresh" @click="load">刷新</el-button>
      </div>
    </div>

    <el-empty v-if="!war" description="还没有任何 War。请联系 owner 在管理页创建。">
      <el-button v-if="userStore.isOwner" type="primary" @click="$router.push('/admin/wars')">
        去创建 War
      </el-button>
    </el-empty>

    <template v-else>
      <el-row :gutter="16" style="margin-bottom: 16px">
        <el-col :xs="12" :md="6">
          <el-card shadow="never">
            <div class="muted">仓库数</div>
            <div style="font-size: 22px; font-weight: 600">{{ stockpiles.length }}</div>
          </el-card>
        </el-col>
        <el-col :xs="12" :md="6">
          <el-card shadow="never">
            <div class="muted">最近上传</div>
            <div style="font-size: 16px">
              {{ uploads[0] ? relTime(uploads[0].uploaded_at) : '-' }}
            </div>
          </el-card>
        </el-col>
        <el-col :xs="12" :md="6">
          <el-card shadow="never">
            <div class="muted">总上传数</div>
            <div style="font-size: 22px; font-weight: 600">{{ uploads.length }}+</div>
          </el-card>
        </el-col>
        <el-col :xs="12" :md="6">
          <el-card shadow="never">
            <div class="muted">物品种类</div>
            <div style="font-size: 22px; font-weight: 600">
              {{ totalDistinctItems }}
            </div>
          </el-card>
        </el-col>
      </el-row>

      <el-card shadow="never" style="margin-bottom: 16px">
        <template #header>
          <div class="flex-row" style="justify-content: space-between">
            <span class="section-title" style="margin:0">仓库列表</span>
            <el-input
              v-model="filter"
              placeholder="搜索地区 / 建筑 / 别名"
              clearable
              size="small"
              style="width: 240px"
            />
          </div>
        </template>
        <el-table
          :data="filteredStockpiles"
          stripe
          row-key="id"
          @row-click="goDetail"
          :empty-text="filter ? '无匹配' : '当前 War 还没有任何仓库快照，去上传一份吧'"
        >
          <el-table-column prop="region" label="地区" min-width="140" />
          <el-table-column prop="sub_region" label="子地区" min-width="140" />
          <el-table-column prop="building_name" label="建筑" min-width="140" />
          <el-table-column prop="player_alias" label="别名" min-width="120" />
          <el-table-column label="可见" width="80">
            <template #default="{ row }">
              <el-tag size="small" :type="row.visibility === 'Public' ? 'success' : 'warning'">
                {{ row.visibility }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="最新快照" width="180">
            <template #default="{ row }">
              <span>{{ formatTs(row.last_captured) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="物品种类" width="100">
            <template #default="{ row }">{{ row.item_count }}</template>
          </el-table-column>
          <el-table-column label="上传者" min-width="120">
            <template #default="{ row }">{{ row.last_uploader || '-' }}</template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card shadow="never">
        <template #header>
          <span class="section-title" style="margin:0">最近上传</span>
        </template>
        <el-table :data="uploads.slice(0, 20)" stripe>
          <el-table-column label="时间" width="180">
            <template #default="{ row }">{{ formatTs(row.uploaded_at) }}</template>
          </el-table-column>
          <el-table-column prop="region" label="地区" />
          <el-table-column prop="building_name" label="建筑" />
          <el-table-column prop="player_alias" label="别名" />
          <el-table-column prop="uploader_name" label="上传者" />
          <el-table-column label="物品" width="80">
            <template #default="{ row }">{{ row.item_count }}</template>
          </el-table-column>
          <el-table-column label="" width="100">
            <template #default="{ row }">
              <el-button text type="primary" size="small" @click="$router.push(`/stockpiles/${row.stockpile_id}`)">
                查看
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { UploadFilled, Refresh } from '@element-plus/icons-vue'
import { warsApi, stockpilesApi, type War, type Stockpile, type RecentUpload } from '@/api/stockpiles'
import { useUserStore } from '@/stores/user'
import { showError } from '@/api/client'
import { formatTs, relTime } from '@/utils/format'

const router = useRouter()
const userStore = useUserStore()
const war = ref<War | null>(null)
const stockpiles = ref<Stockpile[]>([])
const uploads = ref<RecentUpload[]>([])
const filter = ref('')

const filteredStockpiles = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return stockpiles.value
  return stockpiles.value.filter((s) =>
    [s.region, s.sub_region, s.building_name, s.player_alias].some((v) => v.toLowerCase().includes(q))
  )
})

const totalDistinctItems = computed(() =>
  stockpiles.value.reduce((acc, s) => acc + (s.item_count || 0), 0)
)

async function load() {
  try {
    const w = await warsApi.active()
    war.value = w.war
    if (w.war) {
      const sp = await stockpilesApi.list(w.war.id)
      stockpiles.value = sp.stockpiles
    } else {
      stockpiles.value = []
    }
    const up = await stockpilesApi.recent(50)
    uploads.value = up.uploads
  } catch (e) {
    showError(e, '加载失败')
  }
}

function goDetail(row: Stockpile) {
  router.push(`/stockpiles/${row.id}`)
}

onMounted(load)
</script>