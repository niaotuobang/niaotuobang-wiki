<script setup lang="ts">
import { ref, watch } from 'vue'
const props = withDefaults(defineProps<{ name: string; mediaId?: string; size?: number }>(), {
  size: 32,
})
const failed = ref(false)
watch(
  () => props.mediaId,
  () => {
    failed.value = false
  },
)
</script>
<template>
  <span
    class="user-avatar"
    :style="{
      width: size + 'px',
      height: size + 'px',
      fontSize: Math.max(11, Math.round(size * 0.4)) + 'px',
    }"
    aria-hidden="true"
    ><img v-if="mediaId && !failed" :src="'/media/' + mediaId" alt="" @error="failed = true" /><span
      v-else
      >{{ name.trim().slice(0, 1) || '群' }}</span
    ></span
  >
</template>
