<template>
  <div>
    <h2 class="section-title">War 列表</h2>
    <el-table :data="wars" stripe>
      <el-table-column prop="label" label="名称" min-width="160" />
      <el-table-column label="开始" width="200">
        <template #default="{ row }">{{ formatTs(row.started_at) }}</template>
      </el-table-column>
      <el-table-column label="结束" width="200">
        <template #default="{ row }">{{ row.ended_at ? formatTs(row.ended_at) : '-' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.is_active" type="success">进行中</el-tag>
          <el-tag v-else type="info">已归档</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="notes" label="备注" />
      <el-table-column label="" width="120">
        <template #default="{ row }">
          <el-button type="primary" size="small" text @click="goWar(row.id)">
            查看仓库
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-divider />
    <h3 class="section-title" v-if="selectedWar">{{ selectedWar.label }} 仓库</h3>
    <el-table v-if="selectedWar" :data="stockpiles" stripe>
      <el-table-column prop="region" label="地区" min-width="140" />
      <el-table-column prop="sub_region" label="子地区" min-width="140" />
      <el-table-column prop="building_name" label="建筑" />
      <el-table-column prop="player_alias" label="别名" />
      <el-table-column label="最新快照" width="180">
        <template #default="{ row }">{{ formatTs(row.last_captured) }}</template>
      </el-table-column>
      <el-table-column label="" width="100">
        <template #default="{ row }">
          <el-button text type="primary" size="small" @click="$router.push(`/stockpiles/${row.id}`)">
            查看
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { warsApi, stockpilesApi, type War, type Stockpile } from '@/api/stockpiles'
import { showError } from '@/api/client'
import { formatTs } from '@/utils/format'

const wars = ref<War[]>([])
const selectedWar = ref<War | null>(null)
const stockpiles = ref<Stockpile[]>([])

async function load() {
  try {
    const r = await warsApi.list()
    wars.value = r.wars
    const active = r.wars.find((w) => w.is_active)
    if (active) {
      selectedWar.value = active
      const sp = await stockpilesApi.list(active.id)
      stockpiles.value = sp.stockpiles
    }
  } catch (e) {
    showError(e, '加载 War 失败')
  }
}

async function goWar(id: number) {
  const war = wars.value.find((w) => w.id === id)
  if (!war) return
  selectedWar.value = war
  try {
    const sp = await stockpilesApi.list(id)
    stockpiles.value = sp.stockpiles
  } catch (e) {
    showError(e, '加载仓库失败')
  }
}

onMounted(load)
</script>