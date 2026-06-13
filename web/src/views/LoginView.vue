<template>
  <div class="auth-page">
    <div class="auth-card">
      <div class="auth-title">登录 · Foxhole 仓管</div>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="onSubmit">
        <el-form-item label="Steam64 ID" prop="steam64id">
          <el-input v-model="form.steam64id" placeholder="17 位数字" maxlength="17" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="登录密码" />
        </el-form-item>
        <el-button type="primary" :loading="loading" style="width: 100%" @click="onSubmit">
          登录
        </el-button>
      </el-form>
      <div class="auth-foot">
        <router-link to="/register">还没注册？或被要求重置密码</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { authApi } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { showError } from '@/api/client'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  steam64id: '',
  password: '',
})

const rules: FormRules = {
  steam64id: [
    { required: true, message: '请输入 Steam64ID', trigger: 'blur' },
    { pattern: /^\d{17}$/, message: '必须是 17 位数字', trigger: 'blur' },
  ],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function onSubmit() {
  if (!formRef.value) return
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  loading.value = true
  try {
    const r = await authApi.login(form.steam64id, form.password)
    userStore.setUser(r.user)
    ElMessage.success(`欢迎，${r.user.display_name}`)
    const redirect = (route.query.redirect as string) || '/'
    router.push(redirect)
  } catch (e) {
    showError(e, '登录失败')
  } finally {
    loading.value = false
  }
}
</script>