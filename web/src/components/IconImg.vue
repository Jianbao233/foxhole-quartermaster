<template>
  <img
    class="icon-img"
    :src="src"
    :alt="alt"
    @error="onErr"
    :style="{ width: size + 'px', height: size + 'px' }"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = defineProps<{
  canonicalId: string
  alt?: string
  size?: number
}>()

const failed = ref(false)
const fallback = '/icons/_unknown.png'

const src = computed(() => {
  if (failed.value) return fallback
  if (!props.canonicalId) return fallback
  return `/icons/${encodeURIComponent(props.canonicalId)}.png`
})

watch(
  () => props.canonicalId,
  () => {
    failed.value = false
  }
)

function onErr() {
  failed.value = true
}
</script>