<template>
  <div v-loading="loading">
    <el-row :gutter="16">
      <el-col :xs="12" :md="6">
        <el-card shadow="never">
          <div class="muted">用户数</div>
          <div style="font-size: 22px; font-weight: 600">{{ stats?.user_count ?? '-' }}</div>
        </el-card>
      </el-col>
      <el-col :xs="12" :md="6">
        <el-card shadow="never">
          <div class="muted">物品库</div>
          <div style="font-size: 22px; font-weight: 600">
            {{ stats?.item_count ?? '-' }}
          </div>
          <div class="muted" style="font-size: 12px">
            缺图: {{ stats?.placeholder_items ?? '-' }}
          </div>
        </el-card>
      </el-col>
      <el-col :xs="12" :md="6">
        <el-card shadow="never">
          <div class="muted">仓库数</div>
          <div style="font-size: 22px; font-weight: 600">{{ stats?.stockpile_count ?? '-' }}</div>
          <div class="muted" style="font-size: 12px">
            快照: {{ stats?.snapshot_count ?? '-' }}
          </div>
        </el-card>
      </el-col>
      <el-col :xs="12" :md="6">
        <el-card shadow="never">
          <div class="muted">DB 大小</div>
          <div style="font-size: 22px; font-weight: 600">{{ formatBytes(stats?.db_size_bytes || 0) }}</div>
          <div class="muted" style="font-size: 12px">{{ stats?.db_path }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card style="margin-top: 16px" shadow="never" v-if="stats?.active_war_label">
      <div class="muted">活跃 War</div>
      <div style="font-size: 18px; font-weight: 600">{{ stats.active_war_label }}</div>
      <div class="muted" style="font-size: 12px">自 {{ formatTs(stats.active_war_started) }}</div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { adminApi, type AdminStats } from '@/api/admin'
import { showError } from '@/api/client'
import { formatTs, formatBytes } from '@/utils/format'

const stats = ref<AdminStats | null>(null)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    stats.value = await adminApi.stats()
  } catch (e) {
    showError(e, '加载概览失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>