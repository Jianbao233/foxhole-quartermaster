<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 12px">
      <template #header>开新 War</template>
      <el-form inline>
        <el-form-item label="名称">
          <el-input v-model="newForm.label" placeholder="例：War 124" style="width: 220px" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="newForm.notes" placeholder="可选" style="width: 320px" />
        </el-form-item>
        <el-button type="primary" :loading="newLoading" @click="newWar">开新 War（自动归档旧场）</el-button>
      </el-form>
    </el-card>

    <el-table :data="wars" stripe v-loading="loading">
      <el-table-column prop="label" label="名称" min-width="160" />
      <el-table-column label="开始" width="180">
        <template #default="{ row }">{{ formatTs(row.started_at) }}</template>
      </el-table-column>
      <el-table-column label="结束" width="180">
        <template #default="{ row }">{{ row.ended_at ? formatTs(row.ended_at) : '-' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.is_active" type="success">进行中</el-tag>
          <el-tag v-else type="info">已归档</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="notes" label="备注" />
      <el-table-column label="" width="280">
        <template #default="{ row }">
          <el-button v-if="row.is_active" size="small" @click="endWar(row.id)">结束（不开新场）</el-button>
          <el-button size="small" @click="exportWar(row)">导出 JSON</el-button>
          <el-button v-if="!row.is_active" size="small" type="danger" @click="askDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="deleteDialog" :title="`删除 ${deleteTarget?.label}`" width="480px">
      <el-alert type="error" :closable="false" style="margin-bottom: 12px">
        删除后不可恢复（包含全部仓与快照）。请输入下方确认串以继续。
      </el-alert>
      <p>需要输入：<span class="code-inline">DELETE WAR {{ deleteTarget?.label }}</span></p>
      <el-input v-model="confirmText" placeholder="输入确认串" />
      <template #footer>
        <el-button @click="deleteDialog = false">取消</el-button>
        <el-button type="danger" :disabled="confirmText !== expectedConfirm" @click="confirmDelete">
          删除
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { warsApi, type War } from '@/api/stockpiles'
import { adminApi } from '@/api/admin'
import { showError } from '@/api/client'
import { formatTs } from '@/utils/format'

const wars = ref<War[]>([])
const loading = ref(false)
const newForm = reactive({ label: '', notes: '' })
const newLoading = ref(false)

const deleteDialog = ref(false)
const deleteTarget = ref<War | null>(null)
const confirmText = ref('')
const expectedConfirm = computed(() =>
  deleteTarget.value ? `DELETE WAR ${deleteTarget.value.label}` : ''
)

async function load() {
  loading.value = true
  try {
    const r = await warsApi.list()
    wars.value = r.wars
  } catch (e) {
    showError(e, '加载失败')
  } finally {
    loading.value = false
  }
}

async function newWar() {
  if (!newForm.label.trim()) {
    ElMessage.warning('请填写名称')
    return
  }
  newLoading.value = true
  try {
    await adminApi.newWar(newForm.label.trim(), newForm.notes)
    ElMessage.success('已开新 War')
    newForm.label = ''
    newForm.notes = ''
    await load()
  } catch (e) {
    showError(e, '操作失败')
  } finally {
    newLoading.value = false
  }
}

async function endWar(id: number) {
  try {
    await adminApi.endWar(id)
    ElMessage.success('已结束')
    await load()
  } catch (e) {
    showError(e, '操作失败')
  }
}

function askDelete(w: War) {
  deleteTarget.value = w
  confirmText.value = ''
  deleteDialog.value = true
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  try {
    await adminApi.deleteWar(deleteTarget.value.id, confirmText.value)
    ElMessage.success('已删除')
    deleteDialog.value = false
    await load()
  } catch (e) {
    showError(e, '删除失败')
  }
}

async function exportWar(w: War) {
  try {
    const data = await adminApi.exportWar(w.id)
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `war-${w.label}.json`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    showError(e, '导出失败')
  }
}

onMounted(load)
</script>