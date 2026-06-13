<template>
  <div class="app-shell">
    <div class="app-header">
      <div class="app-title">FOXHOLE 仓管</div>
      <el-menu mode="horizontal" :default-active="activeMenu" :ellipsis="false" router>
        <el-menu-item index="/">
          <el-icon><DataAnalysis /></el-icon>
          <span>总览</span>
        </el-menu-item>
        <el-menu-item index="/upload">
          <el-icon><UploadFilled /></el-icon>
          <span>上传</span>
        </el-menu-item>
        <el-menu-item index="/wars">
          <el-icon><Calendar /></el-icon>
          <span>War</span>
        </el-menu-item>
        <el-menu-item index="/items">
          <el-icon><Goods /></el-icon>
          <span>物品</span>
        </el-menu-item>
        <el-menu-item v-if="userStore.isOwner" index="/admin">
          <el-icon><Setting /></el-icon>
          <span>管理</span>
        </el-menu-item>
      </el-menu>
      <div class="app-user">
        <span v-if="userStore.user">
          {{ userStore.user.display_name }}
          <el-tag v-if="userStore.user.is_owner" type="success" size="small" effect="dark">owner</el-tag>
        </span>
        <el-button size="small" @click="handleLogout">退出</el-button>
      </div>
    </div>
    <main class="app-main">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const activeMenu = computed(() => {
  const p = route.path
  if (p === '/' || p.startsWith('/stockpiles')) return '/'
  if (p.startsWith('/upload')) return '/upload'
  if (p.startsWith('/wars')) return '/wars'
  if (p.startsWith('/items')) return '/items'
  if (p.startsWith('/admin')) return '/admin'
  return '/'
})

async function handleLogout() {
  await userStore.logout()
  router.push('/login')
}
</script>