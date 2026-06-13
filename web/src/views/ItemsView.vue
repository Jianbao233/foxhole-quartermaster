<template>
  <div>
    <h2 class="section-title">物品库</h2>
    <div class="flex-row" style="margin-bottom: 12px">
      <el-input
        v-model="filter"
        placeholder="搜索物品 / 分类 / 英文名 / canonical_id"
        clearable
        size="small"
        style="width: 320px"
      />
      <el-select v-model="categoryFilter" placeholder="分类" clearable size="small" style="width: 200px">
        <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
      </el-select>
      <el-checkbox v-model="onlyPlaceholder">只看缺图标</el-checkbox>
      <span class="muted">共 {{ filtered.length }} 项</span>
    </div>

    <el-table :data="filtered" stripe height="calc(100vh - 220px)">
      <el-table-column label="" width="60">
        <template #default="{ row }">
          <IconImg :canonical-id="row.canonical_id" :alt="row.name_zh" :size="32" />
        </template>
      </el-table-column>
      <el-table-column label="物品" min-width="200">
        <template #default="{ row }">
          <router-link :to="`/items/${row.canonical_id}`" style="color: var(--el-color-primary)">
            {{ row.name_zh }}
          </router-link>
          <div class="muted" style="font-size: 12px">{{ row.name_en || '-' }}</div>
        </template>
      </el-table-column>
      <el-table-column prop="category" label="分类" width="140" />
      <el-table-column prop="faction" label="阵营" width="100" />
      <el-table-column label="箱装容量" width="100" align="right">
        <template #default="{ row }">{{ row.crate_size ?? '-' }}</template>
      </el-table-column>
      <el-table-column label="图标" width="120">
        <template #default="{ row }">
          <el-tag size="small" :type="row.icon_source === 'placeholder' ? 'info' : 'success'">
            {{ row.icon_source }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="canonical_id" label="canonical_id" min-width="200" />
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { itemsApi, type Item } from '@/api/items'
import { showError } from '@/api/client'
import IconImg from '@/components/IconImg.vue'

const items = ref<Item[]>([])
const filter = ref('')
const categoryFilter = ref('')
const onlyPlaceholder = ref(false)

const categories = computed(() => {
  const set = new Set<string>()
  for (const it of items.value) if (it.category) set.add(it.category)
  return Array.from(set).sort()
})

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  return items.value.filter((it) => {
    if (categoryFilter.value && it.category !== categoryFilter.value) return false
    if (onlyPlaceholder.value && it.icon_source !== 'placeholder') return false
    if (!q) return true
    return [it.name_zh, it.name_en, it.canonical_id, it.category].some((v) =>
      (v || '').toLowerCase().includes(q)
    )
  })
})

async function load() {
  try {
    const r = await itemsApi.list()
    items.value = r.items
  } catch (e) {
    showError(e, '加载物品失败')
  }
}

onMounted(load)
</script>