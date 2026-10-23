<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, catalog, dateLabel, editable, message, session } from '../api'
import type { Article, Page } from '../types'
import Icon from '../components/Icon.vue'
import { readListState, saveListState } from '../listState'
const route = useRoute(),
  router = useRouter(),
  openFilters = ref(false),
  items = ref<Article[]>([]),
  total = ref(0),
  cursor = ref(''),
  loading = ref(false),
  error = ref(''),
  requestSerial = ref(0)
let activeKey = (session.user?.id || 'guest') + ':' + route.fullPath
function stash() {
  if (session.user || session.public_read)
    saveListState(activeKey, { items: items.value, total: total.value, cursor: cursor.value })
}
function hydrate() {
  const state = readListState(activeKey)
  items.value = state?.items || []
  total.value = state?.total || 0
  cursor.value = state?.cursor || ''
}
hydrate()
onBeforeUnmount(() => {
  stash()
  requestSerial.value++
})
const tag = computed(() => catalog.tags.find((t) => t.id === route.params.id))
const isTimeline = computed(() => route.path === '/timeline' || route.query.view === 'timeline')
const title = computed(
  () =>
    tag.value?.name ||
    (route.path === '/search' ? '搜索' : route.path === '/timeline' ? '时间线' : '最近更新'),
)
const extraTags = computed(() =>
  Array.isArray(route.query.tag) ? route.query.tag : [route.query.tag].filter(Boolean),
)
const groups = computed(() => {
  const out: { year: string; articles: Article[] }[] = []
  for (const a of items.value) {
    const year = a.occurred_at.slice(0, 4)
    let group = out.find((g) => g.year === year)
    if (!group) {
      group = { year, articles: [] }
      out.push(group)
    }
    group.articles.push(a)
  }
  return out
})
function toggleFilter(id: string) {
  const selected = new Set(extraTags.value.map(String))
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
  router.push({ query: { ...route.query, tag: [...selected] } })
}

function query(next = false) {
  const q = new URLSearchParams()
  if (route.query.q) q.set('q', String(route.query.q))
  if (route.params.id) q.append('tag', String(route.params.id))
  for (const t of extraTags.value) if (t) q.append('tag', String(t))
  if (isTimeline.value) q.set('timeline', '1')
  if (route.query.year) q.set('year', String(route.query.year))
  if (route.query.undated) q.set('undated', '1')
  if (next && cursor.value) q.set('cursor', cursor.value)
  return q
}
async function load(next = false) {
  const serial = ++requestSerial.value
  loading.value = true
  error.value = ''
  try {
    if (route.params.id && !tag.value) {
      await api('/tags/' + route.params.id)
    }
    const targetCount = next ? 0 : items.value.length
    const params = query(next)
    let data = await api<Page<Article>>('/articles?' + params)
    if (serial !== requestSerial.value) return
    const collected = [...data.items]
    while (!next && collected.length < targetCount && data.next_cursor) {
      params.set('cursor', data.next_cursor)
      data = await api<Page<Article>>('/articles?' + params)
      if (serial !== requestSerial.value) return
      collected.push(...data.items)
    }
    items.value = next ? [...items.value, ...collected] : collected
    total.value = data.total
    cursor.value = data.next_cursor
    stash()
  } catch (e) {
    if (serial === requestSerial.value) error.value = message(e)
  } finally {
    if (serial === requestSerial.value) loading.value = false
  }
}
function highlight(text: string) {
  const q = String(route.query.q || '').trim()
  if (!q) return [{ text, hit: false }]
  const p = text.toLocaleLowerCase().indexOf(q.toLocaleLowerCase())
  if (p < 0) return [{ text, hit: false }]
  return [
    { text: text.slice(0, p), hit: false },
    { text: text.slice(p, p + q.length), hit: true },
    { text: text.slice(p + q.length), hit: false },
  ]
}
onMounted(() => load())
watch(
  () => route.fullPath,
  () => {
    stash()
    activeKey = (session.user?.id || 'guest') + ':' + route.fullPath
    hydrate()
    load()
  },
)
watch(
  () => tag.value?.id,
  () => {
    if (route.params.id) load()
  },
)
</script>
<template>
  <div class="page-heading">
    <div>
      <h1>{{ title }}</h1>
      <p class="page-description">
        {{
          tag?.description ||
          (isTimeline
            ? '按故事发生的时间，回头看看。'
            : route.path === '/search'
              ? route.query.q
                ? '“' + route.query.q + '” 的搜索结果'
                : '在上方输入人物、故事或一句话。'
              : '最近被写下，或被补充的故事。')
        }}
      </p>
    </div>
    <RouterLink
      v-if="tag && editable()"
      class="quiet-button"
      :to="{ path: '/articles/new', query: { tag: tag.id } }"
      ><Icon name="plus" :size="16" />在此写文章</RouterLink
    >
  </div>
  <div class="list-tools">
    <div class="segmented" v-if="tag">
      <RouterLink
        :to="{ query: { ...route.query, view: undefined } }"
        :class="{ selected: !isTimeline }"
        >列表</RouterLink
      ><RouterLink
        :to="{ query: { ...route.query, view: 'timeline' } }"
        :class="{ selected: isTimeline }"
        >时间线</RouterLink
      >
    </div>
    <span v-else class="result-count">{{ total }} 篇文章</span>
    <div class="filters">
      <div class="filter-picker">
        <button
          class="filter-toggle"
          :aria-expanded="openFilters"
          @click="openFilters = !openFilters"
          @keydown.escape="openFilters = false"
        >
          {{ extraTags.length ? '已选 ' + extraTags.length + ' 个标签' : '筛选标签' }}
          <Icon name="tag" :size="13" />
        </button>
        <div v-if="openFilters" class="filter-popover" role="group" aria-label="标签交集筛选">
          <p>同时带有这些标签的文章</p>
          <label
            v-for="t in catalog.tags.filter((t) => t.id !== tag?.id)"
            :key="t.id"
            class="checkbox-label"
            ><input
              type="checkbox"
              :checked="extraTags.includes(t.id)"
              @change="toggleFilter(t.id)"
            />{{ t.name }}</label
          ><button class="text-link" @click="openFilters = false">完成</button>
        </div>
      </div>
      <label v-if="isTimeline"
        ><span class="sr-only">筛选年份</span
        ><input
          class="year-filter"
          type="number"
          min="1"
          max="9999"
          placeholder="年份"
          :value="route.query.year"
          @change="
            $router.push({
              query: {
                ...route.query,
                year: ($event.target as HTMLInputElement).value || undefined,
              },
            })
          "
      /></label>
    </div>
  </div>
  <div v-if="extraTags.length" class="active-filters">
    <span>同时包含</span
    ><button
      v-for="t in catalog.tags.filter((t) => extraTags.includes(t.id))"
      :key="t.id"
      class="tag-chip"
      :aria-label="'移除标签 ' + t.name"
      @click="toggleFilter(t.id)"
    >
      {{ t.name }}<Icon name="close" :size="12" />
    </button>
  </div>
  <div v-if="error" class="notice error" role="alert">
    {{ error }} <button @click="load()">重试</button>
  </div>
  <div v-else-if="loading && !items.length" class="loading">正在寻找故事…</div>
  <div v-else-if="!items.length" class="empty-state">
    <Icon :name="isTimeline ? 'clock' : 'book'" :size="32" />
    <h2>
      {{
        route.path === '/search'
          ? '还没有找到这段故事'
          : isTimeline
            ? '这里还没有填过日期的故事'
            : '这里还没有文章'
      }}
    </h2>
    <p>
      {{
        route.path === '/search'
          ? '换个词，或者少选一个标签试试。'
          : '从一件小事开始，把记得的写下来。'
      }}
    </p>
    <RouterLink
      v-if="editable()"
      class="button"
      :to="{ path: '/articles/new', query: tag ? { tag: tag.id } : {} }"
      >写一篇文章</RouterLink
    >
  </div>
  <div v-else-if="isTimeline" class="timeline">
    <section v-for="group in groups" :key="group.year" class="timeline-year">
      <h2>{{ group.year }}<small>年</small></h2>
      <div class="timeline-entries">
        <article v-for="a in group.articles" :key="a.id" class="timeline-entry">
          <span class="timeline-date">{{
            a.date_precision === 'year'
              ? '日期未详'
              : a.date_precision === 'month'
                ? Number(a.occurred_at.slice(5, 7)) + ' 月'
                : a.occurred_at.slice(5).replace('-', ' / ')
          }}</span>
          <div>
            <RouterLink class="timeline-title" :to="'/articles/' + a.id">{{ a.title }}</RouterLink>
            <p>{{ a.plain_text.slice(0, 70) }}</p>
            <div class="tag-links">
              <RouterLink v-for="t in a.tags" :key="t.id" :to="'/tags/' + t.id">{{
                t.name
              }}</RouterLink>
            </div>
          </div>
        </article>
      </div>
    </section>
  </div>
  <div v-else class="article-list">
    <article v-for="a in items" :key="a.id" class="article-row">
      <h2>
        <RouterLink :to="'/articles/' + a.id"
          ><template v-for="(part, index) in highlight(a.title)" :key="index"
            ><mark v-if="part.hit">{{ part.text }}</mark
            ><template v-else>{{ part.text }}</template></template
          ></RouterLink
        >
      </h2>
      <p class="article-excerpt">
        <template v-for="(part, index) in highlight(a.plain_text)" :key="index"
          ><mark v-if="part.hit">{{ part.text }}</mark
          ><template v-else>{{ part.text }}</template></template
        >
      </p>
      <div class="row-meta">
        <div class="tag-links">
          <RouterLink v-for="t in a.tags.slice(0, 3)" :key="t.id" :to="'/tags/' + t.id">{{
            t.name
          }}</RouterLink
          ><span
            v-for="kind in a.media_kinds"
            :key="kind"
            class="occurrence"
            :title="kind === 'image' ? '含图片' : '含视频'"
            ><Icon :name="kind" :size="13" /><span class="sr-only">{{
              kind === 'image' ? '含图片' : '含视频'
            }}</span></span
          ><span v-if="a.occurred_at" class="occurrence"
            ><Icon name="clock" :size="13" />{{ dateLabel(a.occurred_at, a.date_precision) }}</span
          >
        </div>
        <span>{{ a.updated_by_name }} 补充于 {{ dateLabel(a.updated_at) }}</span>
      </div>
    </article>
  </div>
  <button v-if="cursor && !error" class="load-more" :disabled="loading" @click="load(true)">
    {{ loading ? '加载中…' : '再看一些故事' }} <Icon name="arrow" :size="16" />
  </button>
  <RouterLink
    v-if="isTimeline"
    class="undated-link"
    :to="{ path: '/search', query: { undated: '1' } }"
    >看看还没有填写发生时间的文章</RouterLink
  >
</template>
