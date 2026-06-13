<template>
  <div>
    <div class="flex-row" style="margin-bottom: 12px">
      <el-input
        v-model="filter"
        placeholder="搜索物品 / canonical_id"
        clearable
        size="small"
        style="width: 280px"
      />
      <el-button type="primary" @click="openEdit(null)">新增物品</el-button>
      <el-button @click="uploadIconBtn?.click()">上传图标到所选行...</el-button>
      <input ref="uploadIconBtn" type="file" accept="image/png" style="display:none" @change="onIconFile" />
      <span class="muted">{{ filtered.length }} 项</span>
    </div>

    <el-table :data="filtered" stripe height="calc(100vh - 240px)" highlight-current-row @current-change="onCurrent">
      <el-table-column label="" width="60">
        <template #default="{ row }">
          <IconImg :canonical-id="row.canonical_id" :size="32" />
        </template>
      </el-table-column>
      <el-table-column prop="name_zh" label="中文名" min-width="180" />
      <el-table-column prop="name_en" label="英文名" min-width="180" />
      <el-table-column prop="category" label="分类" width="120" />
      <el-table-column prop="faction" label="阵营" width="100" />
      <el-table-column label="箱装" width="80" align="right">
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
      <el-table-column label="" width="120">
        <template #default="{ row }">
          <el-button size="small" text type="primary" @click.stop="openEdit(row)">编辑</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="editVisible" :title="editing?.canonical_id ? '编辑物品' : '新增物品'" width="560px">
      <el-form label-width="100px" v-if="editing">
        <el-form-item label="canonical_id">
          <el-input v-model="editing.canonical_id" :disabled="!isNew" placeholder="例：mat_basic_materials" />
        </el-form-item>
        <el-form-item label="中文名">
          <el-input v-model="editing.name_zh" />
        </el-form-item>
        <el-form-item label="英文名">
          <el-input v-model="editing.name_en" />
        </el-form-item>
        <el-form-item label="分类">
          <el-input v-model="editing.category" placeholder="material / rifle / ammo / vehicle ..." />
        </el-form-item>
        <el-form-item label="阵营">
          <el-select v-model="editing.faction" clearable placeholder="可选">
            <el-option label="colonial" value="colonial" />
            <el-option label="warden" value="warden" />
            <el-option label="neutral" value="neutral" />
          </el-select>
        </el-form-item>
        <el-form-item label="箱装容量">
          <el-input-number v-model="editing.crate_size" :min="0" :step="1" controls-position="right" />
        </el-form-item>
        <el-form-item label="Wiki URL">
          <el-input v-model="editing.wiki_url" placeholder="https://foxhole.wiki/..." />
        </el-form-item>
        <el-form-item label="别名（每行一个）">
          <el-input v-model="aliasesText" type="textarea" :rows="3" placeholder="自动加入中文名，可再添加历史名 / 英文别名" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { adminApi, type UpsertItemReq } from '@/api/admin'
import { itemsApi, type Item } from '@/api/items'
import { showError } from '@/api/client'
import IconImg from '@/components/IconImg.vue'

const items = ref<Item[]>([])
const filter = ref('')
const editVisible = ref(false)
const editing = ref<UpsertItemReq | null>(null)
const aliasesText = ref('')
const saving = ref(false)
const isNew = ref(false)
const currentRow = ref<Item | null>(null)
const uploadIconBtn = ref<HTMLInputElement>()

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return items.value
  return items.value.filter((it) =>
    [it.name_zh, it.name_en, it.canonical_id, it.category].some((v) =>
      (v || '').toLowerCase().includes(q)
    )
  )
})

async function load() {
  try {
    const r = await itemsApi.list()
    items.value = r.items
  } catch (e) {
    showError(e, '加载物品失败')
  }
}

function openEdit(row: Item | null) {
  if (row) {
    isNew.value = false
    editing.value = {
      canonical_id: row.canonical_id,
      name_zh: row.name_zh,
      name_en: row.name_en || '',
      faction: row.faction || '',
      category: row.category || '',
      crate_size: row.crate_size ?? null,
      wiki_url: row.wiki_url || '',
      aliases: [],
    }
  } else {
    isNew.value = true
    editing.value = {
      canonical_id: '',
      name_zh: '',
      name_en: '',
      faction: '',
      category: '',
      crate_size: null,
      wiki_url: '',
      aliases: [],
    }
  }
  aliasesText.value = ''
  editVisible.value = true
}

async function save() {
  if (!editing.value) return
  if (!editing.value.canonical_id || !editing.value.name_zh) {
    ElMessage.warning('canonical_id 和 中文名 必填')
    return
  }
  const aliases = aliasesText.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
  saving.value = true
  try {
    await adminApi.upsertItem({ ...editing.value, aliases })
    ElMessage.success('已保存')
    editVisible.value = false
    await load()
  } catch (e) {
    showError(e, '保存失败')
  } finally {
    saving.value = false
  }
}

function onCurrent(row: Item | null) {
  currentRow.value = row
}

async function onIconFile(ev: Event) {
  const t = ev.target as HTMLInputElement
  const file = t.files?.[0]
  t.value = ''
  if (!file) return
  if (!currentRow.value) {
    ElMessage.warning('请先在表格中选中一行物品')
    return
  }
  try {
    const fd = new FormData()
    fd.append('icon', file)
    const resp = await fetch(`/api/admin/items/${encodeURIComponent(currentRow.value.canonical_id)}/icon`, {
      method: 'POST',
      body: fd,
      credentials: 'same-origin',
    })
    if (!resp.ok) {
      const txt = await resp.text()
      throw new Error(txt || `HTTP ${resp.status}`)
    }
    ElMessage.success('图标已上传')
    await load()
  } catch (e) {
    showError(e, '上传失败')
  }
}

onMounted(load)
</script>