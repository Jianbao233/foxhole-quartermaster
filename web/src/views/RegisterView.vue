<template>
  <div class="auth-page">
    <div class="auth-card">
      <div class="auth-title">注册 / 重置密码</div>
      <p class="muted" style="margin-top:0">
        Steam64ID 必须由 owner 提前加进白名单，或者你已被要求重置密码。
      </p>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="onSubmit">
        <el-form-item label="Steam64 ID" prop="steam64id">
          <el-input v-model="form.steam64id" placeholder="17 位数字" maxlength="17" />
        </el-form-item>
        <el-form-item label="昵称" prop="display_name">
          <el-input v-model="form.display_name" placeholder="队内昵称" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password placeholder="≥6 位" />
        </el-form-item>
        <el-form-item label="确认密码" prop="password2">
          <el-input v-model="form.password2" type="password" show-password placeholder="再次输入" />
        </el-form-item>
        <el-button type="primary" :loading="loading" style="width: 100%" @click="onSubmit">
          完成注册
        </el-button>
      </el-form>
      <div class="auth-foot">
        <router-link to="/login">已有账号 → 登录</router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { authApi } from '@/api/auth'
import { useUserStore } from '@/stores/user'
import { showError } from '@/api/client'

const router = useRouter()
const userStore = useUserStore()
const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  steam64id: '',
  display_name: '',
  password: '',
  password2: '',
})

const rules: FormRules = {
  steam64id: [
    { required: true, message: '请输入 Steam64ID', trigger: 'blur' },
    { pattern: /^\d{17}$/, message: '必须是 17 位数字', trigger: 'blur' },
  ],
  display_name: [{ required: true, message: '请输入昵称', trigger: 'blur' }],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '至少 6 位', trigger: 'blur' },
  ],
  password2: [
    {
      validator: (_r, v, cb) => (v === form.password ? cb() : cb(new Error('两次密码不一致'))),
      trigger: 'blur',
    },
  ],
}

async function onSubmit() {
  if (!formRef.value) return
  const ok = await formRef.value.validate().catch(() => false)
  if (!ok) return
  loading.value = true
  try {
    const r = await authApi.register(form.steam64id, form.password, form.display_name)
    userStore.setUser(r.user)
    ElMessage.success('注册成功')
    router.push('/')
  } catch (e) {
    showError(e, '注册失败')
  } finally {
    loading.value = false
  }
}
</script>