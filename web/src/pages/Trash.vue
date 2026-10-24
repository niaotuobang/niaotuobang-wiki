<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, dateLabel, message } from '../api'
import type { Page } from '../types'
import Icon from '../components/Icon.vue'
const items = ref<
    { id: string; title: string; deleted_at: string; deletion_time_unknown?: boolean }[]
  >([]),
  cursor = ref(''),
  loading = ref(false),
  error = ref('')
async function load(more = false) {
  loading.value = true
  error.value = ''
  try {
    const page = await api<Page<(typeof items.value)[number]>>(
      '/admin/trash' + (more ? '?cursor=' + encodeURIComponent(cursor.value) : ''),
    )
    items.value = more ? [...items.value, ...page.items] : page.items
    cursor.value = page.next_cursor
  } catch (e) {
    error.value = message(e)
  } finally {
    loading.value = false
  }
}
onMounted(() => load())
</script>
<template>
  <div class="page-heading">
    <div>
      <h1>回收站</h1>
      <p class="page-description">已删除的文章仍保留正文和历史版本，管理员可以查看或恢复。</p>
    </div>
  </div>
  <p v-if="error" class="notice error" role="alert">
    {{ error }}<button @click="load()">重试</button>
  </p>
  <div v-if="!loading && !error && !items.length" class="empty-state">
    <Icon name="trash" :size="32" />
    <h2>回收站是空的</h2>
    <p>文章放入回收站后，可以在这里找回来。</p>
  </div>
  <div class="article-list">
    <article v-for="item in items" :key="item.id" class="article-row">
      <h2>
        <RouterLink :to="'/trash/' + item.id">{{ item.title || '未命名文章' }}</RouterLink>
      </h2>
      <p class="muted">
        {{
          item.deletion_time_unknown
            ? '旧站已删除，原删除时间未记录'
            : dateLabel(item.deleted_at) + ' 放入回收站'
        }}
      </p>
      <RouterLink class="text-link trash-history-link" :to="'/trash/' + item.id"
        >查看正文与历史 <Icon name="arrow" :size="15"
      /></RouterLink>
    </article>
  </div>
  <p v-if="loading" class="loading">正在查看回收站…</p>
  <button v-else-if="cursor" class="load-more" @click="load(true)">查看更多</button>
</template>
