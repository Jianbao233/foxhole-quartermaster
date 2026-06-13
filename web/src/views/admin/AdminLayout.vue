<template>
  <div>
    <h2 class="section-title">管理后台</h2>
    <el-tabs v-model="active" type="card" @tab-change="onTab">
      <el-tab-pane label="概览" name="stats" />
      <el-tab-pane label="白名单" name="users" />
      <el-tab-pane label="War" name="wars" />
      <el-tab-pane label="物品" name="items" />
    </el-tabs>
    <router-view />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const active = ref('stats')

function syncFromRoute() {
  if (route.path.endsWith('/users')) active.value = 'users'
  else if (route.path.endsWith('/wars')) active.value = 'wars'
  else if (route.path.endsWith('/items')) active.value = 'items'
  else active.value = 'stats'
}

function onTab(name: string | number) {
  router.push(`/admin/${String(name)}`)
}

watch(() => route.path, syncFromRoute)
onMounted(syncFromRoute)
</script>