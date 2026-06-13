<template>
  <div>
    <el-card shadow="never" style="margin-bottom: 12px">
      <template #header>添加白名单</template>
      <el-form inline>
        <el-form-item label="Steam64ID">
          <el-input v-model="newForm.steam64id" placeholder="17 位数字" maxlength="17" style="width: 220px" />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model="newForm.display_name" placeholder="默认 Member-xxxx" style="width: 200px" />
        </el-form-item>
        <el-button type="primary" @click="addUser" :loading="addLoading">添加 / 启用</el-button>
      </el-form>
    </el-card>

    <el-table :data="users" stripe v-loading="loading">
      <el-table-column prop="display_name" label="昵称" min-width="140" />
      <el-table-column prop="steam64id" label="Steam64ID" min-width="180" />
      <el-table-column label="角色" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.is_owner" type="success">owner</el-tag>
          <el-tag v-else type="info">member</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="180">
        <template #default="{ row }">
          <el-tag v-if="!row.is_active" type="danger">已踢出</el-tag>
          <el-tag v-else-if="!row.registered" type="warning">未注册</el-tag>
          <el-tag v-else-if="row.must_reset_pw" type="warning">需重置密码</el-tag>
          <el-tag v-else type="success">在线可用</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="加入时间" width="180">
        <template #default="{ row }">{{ formatTs(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="" width="220">
        <template #default="{ row }">
          <el-button v-if="row.registered && !row.is_owner" size="small" @click="resetUser(row.steam64id)">
            标记重置密码
          </el-button>
          <el-popconfirm
            v-if="!row.is_owner && row.is_active"
            :title="`确认踢出 ${row.display_name}？`"
            @confirm="kickUser(row.steam64id)"
          >
            <template #reference>
              <el-button size="small" type="danger">踢出</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { adminApi, type AdminUser } from '@/api/admin'
import { showError } from '@/api/client'
import { formatTs } from '@/utils/format'

const users = ref<AdminUser[]>([])
const loading = ref(false)
const addLoading = ref(false)
const newForm = reactive({ steam64id: '', display_name: '' })

async function load() {
  loading.value = true
  try {
    const r = await adminApi.users()
    users.value = r.users
  } catch (e) {
    showError(e, '加载用户失败')
  } finally {
    loading.value = false
  }
}

async function addUser() {
  if (!/^\d{17}$/.test(newForm.steam64id)) {
    ElMessage.warning('Steam64ID 必须是 17 位数字')
    return
  }
  addLoading.value = true
  try {
    await adminApi.addWhitelist(newForm.steam64id, newForm.display_name)
    ElMessage.success('已添加 / 启用')
    newForm.steam64id = ''
    newForm.display_name = ''
    await load()
  } catch (e) {
    showError(e, '添加失败')
  } finally {
    addLoading.value = false
  }
}

async function resetUser(id: string) {
  try {
    await adminApi.resetUser(id)
    ElMessage.success('已标记重置密码')
    await load()
  } catch (e) {
    showError(e, '操作失败')
  }
}

async function kickUser(id: string) {
  try {
    await adminApi.kickUser(id)
    ElMessage.success('已踢出')
    await load()
  } catch (e) {
    showError(e, '操作失败')
  }
}

onMounted(load)
</script>