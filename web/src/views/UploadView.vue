<template>
  <div>
    <h2 class="section-title">上传仓库快照</h2>
    <el-alert type="info" :closable="false" style="margin-bottom: 12px">
      把游戏内导出的 .txt 整段粘贴到下方文本框；服务端会按严格模式解析，缺物品/格式错误整文件拒绝。
    </el-alert>

    <el-form label-position="top">
      <el-form-item label="原始 .txt 内容">
        <el-input
          v-model="rawText"
          type="textarea"
          :rows="16"
          placeholder="例：&#10;Umbral Wildwood - The Gap - 海港 - Public - X: 0.38 Y: 0.52,2026.06.13-06.48.16&#10;Booker风暴步枪Model 838 (箱装),0&#10;基础材料,260"
        />
      </el-form-item>
      <el-row :gutter="12">
        <el-col :xs="24" :md="12">
          <el-form-item label="仓库别名（你给这个仓起的名字）">
            <el-input v-model="alias" placeholder="例：东门仓" clearable />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :md="12">
          <el-form-item label="">
            <el-checkbox v-model="isNewAlias" style="margin-top: 32px">
              这是一个新别名（首次为该仓上传时勾选）
            </el-checkbox>
          </el-form-item>
        </el-col>
      </el-row>
      <div class="flex-row">
        <el-button type="primary" :loading="loading" @click="submit">提交</el-button>
        <el-button :disabled="loading" @click="rawText = ''; alias = ''; isNewAlias = false">
          清空
        </el-button>
        <el-button :disabled="loading" @click="loadSample">
          填入示例（开发用）
        </el-button>
      </div>
    </el-form>

    <el-divider />

    <el-card v-if="result" shadow="never">
      <template #header>
        <span class="section-title" style="margin:0">最近一次提交</span>
      </template>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="结果">
          <el-tag :type="result.duplicate ? 'warning' : 'success'">
            {{ result.duplicate ? '内容重复（已存在）' : '入库成功' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="物品行">{{ result.item_count }}</el-descriptions-item>
        <el-descriptions-item label="地区">{{ result.region }} / {{ result.sub_region }}</el-descriptions-item>
        <el-descriptions-item label="建筑">{{ result.building }}</el-descriptions-item>
        <el-descriptions-item label="可见">{{ result.visibility }}</el-descriptions-item>
        <el-descriptions-item label="别名">{{ result.player_alias }}</el-descriptions-item>
        <el-descriptions-item label="游戏内时间">{{ formatTs(result.captured_at) }}</el-descriptions-item>
        <el-descriptions-item label="">
          <el-button type="primary" size="small" @click="$router.push(`/stockpiles/${result.stockpile_id}`)">
            打开仓库详情
          </el-button>
        </el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-dialog v-model="newAliasDialog" title="新别名确认" width="420px">
      <p>当前 War 下还没有以下组合的仓：</p>
      <p class="muted">
        别名 <span class="code-inline">{{ alias }}</span> 是否新建？
      </p>
      <template #footer>
        <el-button @click="newAliasDialog = false">取消</el-button>
        <el-button type="primary" @click="confirmNewAlias">确认新建并提交</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ingestApi, type IngestResult } from '@/api/stockpiles'
import { HttpError, showError } from '@/api/client'
import { formatTs } from '@/utils/format'

const rawText = ref('')
const alias = ref('')
const isNewAlias = ref(false)
const loading = ref(false)
const result = ref<IngestResult | null>(null)
const newAliasDialog = ref(false)

async function submit() {
  if (!rawText.value.trim()) {
    ElMessage.warning('请粘贴 .txt 内容')
    return
  }
  if (!alias.value.trim()) {
    ElMessage.warning('请填写仓库别名')
    return
  }
  loading.value = true
  try {
    const r = await ingestApi.submit(rawText.value, alias.value.trim(), isNewAlias.value)
    result.value = r
    if (r.duplicate) {
      ElMessage.warning('该内容已存在，未重复入库')
    } else {
      ElMessage.success(`入库成功，共 ${r.item_count} 个物品`)
    }
  } catch (e) {
    if (e instanceof HttpError && e.apiError?.code === 'new_alias_required') {
      newAliasDialog.value = true
    } else if (e instanceof HttpError && e.apiError?.code === 'unknown_item') {
      const names = e.apiError.unknown_items || []
      ElMessageBox.alert(
        `以下物品不在物品库，需要 owner 在管理页添加再上传：\n\n${names.join('\n')}`,
        '未知物品',
        { confirmButtonText: '我知道了' }
      )
    } else {
      showError(e, '提交失败')
    }
  } finally {
    loading.value = false
  }
}

async function confirmNewAlias() {
  newAliasDialog.value = false
  isNewAlias.value = true
  await submit()
}

function loadSample() {
  rawText.value = `Umbral Wildwood - The Gap - 海港 - Public - X: 0.38 Y: 0.52,2026.06.13-06.48.16
基础材料,260
精炼材料,42`
  alias.value = '东门仓'
  isNewAlias.value = true
}
</script>