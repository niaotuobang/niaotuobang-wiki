<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, dateLabel, editable, message } from '../api'
import type { Article } from '../types'
import DocumentView from '../components/DocumentView.vue'
import Icon from '../components/Icon.vue'
import Avatar from '../components/Avatar.vue'
const route = useRoute(),
  router = useRouter(),
  article = ref<Article>(),
  backlinks = ref<{ id: string; title: string; context: string; paragraph_id: string }[]>([]),
  error = ref(''),
  loading = ref(true),
  backlinkCursor = ref(''),
  deleting = ref(false),
  busy = ref(false)
async function load() {
  loading.value = true
  error.value = ''
  try {
    article.value = await api<Article>('/articles/' + route.params.id)
    if (article.value.status === 'draft') {
      await router.replace('/articles/' + article.value.id + '/edit')
      return
    }
    const links = await api('/articles/' + route.params.id + '/backlinks')
    backlinks.value = links.items
    backlinkCursor.value = links.next_cursor
    document.title = article.value.title + ' · 鸟托邦 Wiki'
    await nextTick()
    if (route.hash)
      document.getElementById(decodeURIComponent(route.hash.slice(1)))?.scrollIntoView()
  } catch (e) {
    error.value = message(e)
  } finally {
    loading.value = false
  }
}
async function remove() {
  busy.value = true
  try {
    await api('/articles/' + article.value?.id, { method: 'DELETE' })
    router.push('/')
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}
async function moreBacklinks() {
  try {
    const page = await api(
      '/articles/' +
        route.params.id +
        '/backlinks?cursor=' +
        encodeURIComponent(backlinkCursor.value),
    )
    backlinks.value.push(...page.items)
    backlinkCursor.value = page.next_cursor
  } catch (e) {
    error.value = message(e)
  }
}
onMounted(load)
</script>
<template>
  <div v-if="loading" class="loading">正在翻开这篇文章…</div>
  <div v-else-if="error && !article" class="empty-state">
    <h1>这篇文章暂时打不开</h1>
    <p role="alert">{{ error }}</p>
    <button class="button" @click="load">重试</button
    ><RouterLink class="text-link" to="/">返回首页</RouterLink>
  </div>
  <template v-else-if="article"
    ><RouterLink to="/" class="back-link"><Icon name="back" :size="16" />回到 Wiki</RouterLink>
    <div class="article-layout">
      <article class="article-main">
        <header class="article-heading">
          <h1>{{ article.title }}</h1>
          <p class="article-updated">
            最后更新于 {{ dateLabel(article.updated_at) }}<span>第 {{ article.revision }} 版</span>
          </p>
        </header>
        <DocumentView :document="article.document" />
        <section v-if="backlinks.length" class="backlinks">
          <h2><Icon name="link" :size="18" />提到这篇文章</h2>
          <RouterLink
            v-for="a in backlinks"
            :key="a.id"
            :to="'/articles/' + a.id + (a.paragraph_id ? '#' + a.paragraph_id : '')"
            ><strong>{{ a.title }}</strong
            ><span>{{ a.context.slice(0, 100) }}</span></RouterLink
          >
          <button v-if="backlinkCursor" class="text-link" @click="moreBacklinks">
            查看更多提及
          </button>
        </section>
      </article>
      <aside class="article-aside">
        <section>
          <h2>收录于</h2>
          <div class="article-tags">
            <RouterLink v-for="t in article.tags" :key="t.id" :to="'/tags/' + t.id">{{
              t.name
            }}</RouterLink
            ><span v-if="!article.tags.length" class="muted">尚未添加标签</span>
          </div>
        </section>
        <section v-if="article.occurred_at">
          <h2>故事发生在</h2>
          <p class="event-date">{{ dateLabel(article.occurred_at, article.date_precision) }}</p>
        </section>
        <section>
          <h2>最后补充的人</h2>
          <p class="editor-name version-editor">
            <Avatar
              :name="article.updated_by_name"
              :media-id="article.updated_by_avatar"
              :size="32"
            />{{ article.updated_by_name }}
          </p>
          <p class="muted">共同记录这个群的故事</p>
        </section>
        <section>
          <h2>这次改了什么</h2>
          <p class="change-summary">{{ article.change_summary || '又添了一点我们的故事。' }}</p>
        </section>
        <div class="article-actions">
          <RouterLink
            v-if="editable()"
            :to="'/articles/' + article.id + '/edit'"
            class="button article-action"
            ><Icon name="edit" />编辑</RouterLink
          ><RouterLink
            :to="'/articles/' + article.id + '/history'"
            class="button button-outline article-action"
            ><Icon name="clock" />历史版本</RouterLink
          ><button v-if="editable()" class="delete-link" @click="deleting = true">
            <Icon name="trash" :size="15" />放入回收站
          </button>
        </div>
      </aside>
    </div>
    <div v-if="deleting" class="modal-shade" @click.self="deleting = false">
      <section class="dialog" role="dialog" aria-modal="true" aria-labelledby="delete-title">
        <h2 id="delete-title">把这篇文章放入回收站？</h2>
        <p>文章会从列表中隐藏，管理员可以在管理页面恢复。历史版本会保留。</p>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <div class="dialog-actions">
          <button class="quiet-button" @click="deleting = false">取消</button
          ><button class="button" :disabled="busy" @click="remove">
            {{ busy ? '处理中…' : '放入回收站' }}
          </button>
        </div>
      </section>
    </div></template
  >
</template>
