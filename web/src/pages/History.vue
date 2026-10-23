<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { diffLines } from 'diff'
import { useRoute, useRouter } from 'vue-router'
import { api, dateLabel, editable, message, refreshTags } from '../api'
import type { Article } from '../types'
import DocumentView from '../components/DocumentView.vue'
import Icon from '../components/Icon.vue'
const route = useRoute(),
  router = useRouter(),
  current = ref<Article>(),
  versions = ref<Article[]>([]),
  chosen = ref(0),
  base = ref(0),
  error = ref(''),
  restoring = ref(false),
  confirm = ref(false),
  ignore = ref(false),
  missing = ref(false)
const cache = ref<Record<number, Article>>({}),
  cursor = ref('')
const selected = computed(() => cache.value[chosen.value]),
  previous = computed(() => cache.value[base.value])
async function loadDetails() {
  try {
    await Promise.all(
      [...new Set([chosen.value, base.value])]
        .filter((v) => v > 0 && !cache.value[v])
        .map(async (revision) => {
          cache.value[revision] = await api<Article>(
            '/articles/' + route.params.id + '/versions/' + revision,
          )
        }),
    )
  } catch (e) {
    error.value = message(e)
  }
}
watch([chosen, base], loadDetails)
async function more() {
  try {
    const page = await api(
      '/articles/' + route.params.id + '/versions?cursor=' + encodeURIComponent(cursor.value),
    )
    versions.value.push(...page.items)
    cursor.value = page.next_cursor
  } catch (e) {
    error.value = message(e)
  }
}
const changes = computed(() =>
  diffLines(previous.value?.plain_text || '', selected.value?.plain_text || '', {
    timeout: 150,
    maxEditLength: 1000,
  }),
)
const removed = computed(() =>
  (changes.value || [])
    .filter((part) => part.removed)
    .flatMap((part) => part.value.trimEnd().split('\n')),
)
const added = computed(() =>
  (changes.value || [])
    .filter((part) => part.added)
    .flatMap((part) => part.value.trimEnd().split('\n')),
)
const tagChange = computed(
  () =>
    JSON.stringify(previous.value?.tags.map((t) => t.name) || []) !==
    JSON.stringify(selected.value?.tags.map((t) => t.name)),
)
function mediaIDs(a?: Article) {
  const ids: string[] = []
  function walk(n: any) {
    if (n.type === 'image' || n.type === 'video') ids.push(n.attrs.mediaId)
    for (const c of n.content || []) walk(c)
  }
  if (a) walk(a.document)
  return ids
}
const mediaChange = computed(
  () => JSON.stringify(mediaIDs(previous.value)) !== JSON.stringify(mediaIDs(selected.value)),
)
async function load() {
  error.value = ''
  try {
    current.value = await api<Article>('/articles/' + route.params.id)
    cache.value[current.value.revision] = current.value
    const page = await api('/articles/' + route.params.id + '/versions')
    versions.value = page.items
    cursor.value = page.next_cursor
    chosen.value = versions.value[0]?.revision || 0
    base.value = versions.value[1]?.revision || 0
  } catch (e) {
    error.value = message(e)
  }
}
async function restore() {
  restoring.value = true
  try {
    await api('/articles/' + route.params.id + '/versions/' + chosen.value + '/restore', {
      method: 'POST',
      body: { expected_revision: current.value?.revision, ignore_missing_tags: ignore.value },
    })
    await refreshTags()
    await router.push('/articles/' + route.params.id)
  } catch (e) {
    error.value = message(e)
    if ((e as any).code === 'MISSING_TAGS') missing.value = true
  } finally {
    restoring.value = false
  }
}
function chooseVersion(v: Article) {
  chosen.value = v.revision
  base.value = versions.value.find((x) => x.revision < v.revision)?.revision || 0
}
function openRestore() {
  confirm.value = true
  error.value = ''
  missing.value = false
  ignore.value = false
}
onMounted(load)
</script>
<template>
  <RouterLink :to="'/articles/' + route.params.id" class="back-link"
    ><Icon name="back" :size="16" />返回文章</RouterLink
  >
  <div class="page-heading">
    <div>
      <h1>每一次补充，都有记录</h1>
      <p class="page-description">{{ current?.title }}</p>
    </div>
  </div>
  <div v-if="error && !confirm" class="notice error" role="alert">
    {{ error }}<button @click="load">重试</button>
  </div>
  <div v-if="selected" class="history-layout">
    <nav aria-label="选择历史版本" class="version-list">
      <button
        v-for="v in versions"
        :key="v.revision"
        :class="{ selected: v.revision === chosen }"
        @click="chooseVersion(v)"
      >
        <strong
          >第 {{ v.revision }} 版
          <small v-if="v.revision === current?.revision">当前版本</small></strong
        ><span>{{ v.updated_by_name }} · {{ dateLabel(v.updated_at) }}</span>
        <p>{{ v.change_summary || '补充文章' }}</p></button
      ><button v-if="cursor" class="text-link" @click="more">查看更早的版本</button>
    </nav>
    <section class="version-detail">
      <div class="version-heading">
        <h2>{{ selected.title }}</h2>
        <button
          v-if="editable() && selected.revision !== current?.revision"
          class="quiet-button"
          @click="openRestore"
        >
          恢复这个版本
        </button>
      </div>
      <label class="comparison-select"
        >与<select v-model="base">
          <option :value="0">空白版本</option>
          <option
            v-for="v in versions.filter((v) => v.revision !== chosen)"
            :key="v.revision"
            :value="v.revision"
          >
            第 {{ v.revision }} 版
          </option></select
        >比较</label
      >
      <div class="version-changes">
        <p v-if="!changes" class="muted">差异较多，请展开前后版本全文核对。</p>
        <p v-if="previous && previous.title !== selected.title">
          <strong>标题：</strong>{{ previous.title }} → {{ selected.title }}
        </p>
        <p v-if="tagChange">
          <strong>标签：</strong>{{ previous?.tags.map((t) => t.name).join('、') || '无' }} →
          {{ selected.tags.map((t) => t.name).join('、') || '无' }}
        </p>
        <p v-if="(previous?.occurred_at || '') !== selected.occurred_at">
          <strong>发生时间：</strong
          >{{ dateLabel(previous?.occurred_at || '', previous?.date_precision) }} →
          {{ dateLabel(selected.occurred_at, selected.date_precision) }}
        </p>
        <p v-if="mediaChange">图片或视频引用有变化，可在下方展开版本全文查看。</p>
        <p v-for="(s, i) in removed" :key="'r' + i" class="diff-removed"><span>−</span>{{ s }}</p>
        <p v-for="(s, i) in added" :key="'a' + i" class="diff-added"><span>＋</span>{{ s }}</p>
        <p v-if="changes && !removed.length && !added.length" class="muted">
          正文文字没有变化；格式和媒体请比较版本全文。
        </p>
      </div>
      <details class="version-full" open>
        <summary>第 {{ selected.revision }} 版全文</summary>
        <DocumentView :document="selected.document" />
      </details>
      <details v-if="previous" class="version-full">
        <summary>对照：第 {{ previous.revision }} 版全文</summary>
        <DocumentView :document="previous.document" />
      </details>
    </section>
  </div>
  <div v-if="confirm" class="modal-shade" @click.self="confirm = false">
    <section class="dialog" role="dialog" aria-modal="true" aria-labelledby="restore-title">
      <h2 id="restore-title">恢复为第 {{ chosen }} 版的内容？</h2>
      <p>这会生成一个新版本，现有历史记录继续保留。</p>
      <p v-if="error" class="notice error" role="alert">{{ error }}</p>
      <label v-if="missing" class="checkbox-label"
        ><input v-model="ignore" type="checkbox" />忽略已删除的标签，保留其他内容</label
      >
      <div class="dialog-actions">
        <button class="quiet-button" @click="confirm = false">取消</button
        ><button class="button" :disabled="restoring || (missing && !ignore)" @click="restore">
          {{ restoring ? '恢复中…' : '确认恢复' }}
        </button>
      </div>
    </section>
  </div>
</template>
