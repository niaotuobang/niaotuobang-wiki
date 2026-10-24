<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, dateLabel, message } from '../api'
import Icon from '../components/Icon.vue'
const items = ref<{ id: string; title: string; updated_at: string }[]>([]),
  error = ref(''),
  loading = ref(true),
  cursor = ref('')
async function load(next = false) {
  try {
    const page = await api('/drafts' + (next ? '?cursor=' + encodeURIComponent(cursor.value) : ''))
    items.value = next ? [...items.value, ...page.items] : page.items
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
      <h1>我的草稿</h1>
      <p class="page-description">未发布的文章只对你可见，随时回来继续写。</p>
    </div>
    <RouterLink v-if="items.length" to="/articles/new" class="quiet-button"
      ><Icon name="plus" :size="16" />写一篇新的</RouterLink
    >
  </div>
  <p v-if="error" class="notice error" role="alert">
    {{ error }}<button @click="load()">重试</button>
  </p>
  <p v-if="loading" class="loading">正在找回草稿…</p>
  <div v-else-if="!items.length && !error" class="empty-state draft-empty">
    <Icon name="edit" :size="32" />
    <h2>还没有草稿</h2>
    <p>写文章时会自动保存，没写完也不用担心。</p>
    <RouterLink to="/articles/new" class="button">写一篇文章</RouterLink>
  </div>
  <div class="article-list">
    <article v-for="draft in items" :key="draft.id" class="article-row draft-row">
      <div>
        <h2>
          <RouterLink :to="'/articles/' + draft.id + '/edit'">{{
            draft.title || '还没有标题'
          }}</RouterLink>
        </h2>
        <p class="muted">保存于 {{ dateLabel(draft.updated_at) }}</p>
      </div>
      <RouterLink :to="'/articles/' + draft.id + '/edit'" class="text-link"
        >继续写 <Icon name="arrow" :size="16"
      /></RouterLink>
    </article>
  </div>
  <button v-if="cursor" class="load-more" @click="load(true)">查看更早的草稿</button>
</template>
