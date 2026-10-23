<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { api, ApiError, catalog, editable, message, refreshTags, session } from '../api'
import type { Article, ArticleWrite, DocNode, Media, Page } from '../types'
import RichEditor from '../components/RichEditor.vue'
import DocumentView from '../components/DocumentView.vue'
import Icon from '../components/Icon.vue'
const route = useRoute(),
  router = useRouter(),
  id = ref(String(route.params.id || '')),
  loaded = ref(false),
  error = ref(''),
  saveState = ref('草稿已保存'),
  publishing = ref(false),
  preview = ref(false),
  rich = ref<InstanceType<typeof RichEditor>>(),
  fileInput = ref<HTMLInputElement>(),
  tagInput = ref(''),
  showTagSearch = ref(false),
  dirty = ref(false),
  recovery = ref<ArticleWrite>(),
  latest = ref<Article>(),
  linkOpen = ref(false),
  linkQuery = ref(''),
  externalLink = ref(''),
  linkResults = ref<Article[]>([])
const form = reactive<ArticleWrite>({
  title: '',
  document: { type: 'doc', content: [{ type: 'paragraph' }] },
  tag_ids: [],
  occurred_at: '',
  date_precision: '',
  change_summary: '',
  expected_revision: 0,
  status: 'published',
})
const cacheKey = computed(() => `wiki-draft:${session.user?.id}:${id.value}`)
const selectedTags = computed(() => catalog.tags.filter((t) => form.tag_ids.includes(t.id)))
const matchingTags = computed(() =>
  catalog.tags.filter(
    (t) =>
      !form.tag_ids.includes(t.id) &&
      t.name.toLocaleLowerCase().includes(tagInput.value.toLocaleLowerCase()),
  ),
)
let timer: ReturnType<typeof setTimeout> | undefined, saving: Promise<void> | undefined
function changed(doc: DocNode) {
  form.document = doc
}
function body() {
  const value = JSON.parse(JSON.stringify(form)) as ArticleWrite
  value.occurred_at = String(form.occurred_at || '')
  return value
}
async function load() {
  try {
    if (!editable()) {
      await router.replace('/account')
      return
    }
    if (!id.value) {
      const initial = {
        ...body(),
        status: 'draft',
        tag_ids: route.query.tag ? [String(route.query.tag)] : [],
      }
      const article = await api<Article>('/articles', { method: 'POST', body: initial })
      await router.replace('/articles/' + article.id + '/edit')
      return
    }
    const a = await api<Article>('/articles/' + id.value)
    Object.assign(form, {
      title: a.title,
      document: a.document,
      tag_ids: a.tags.map((t) => t.id),
      occurred_at: a.occurred_at,
      date_precision: a.date_precision,
      expected_revision: a.revision,
      status: 'published',
      change_summary: '',
    })
    try {
      const draft = await api<ArticleWrite>('/drafts/' + id.value)
      Object.assign(form, draft, { status: 'published' })
      saveState.value = '已恢复保存的草稿'
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 404)) throw e
    }
    const local = localStorage.getItem(cacheKey.value)
    if (local) recovery.value = JSON.parse(local)
    loaded.value = true
  } catch (e) {
    error.value = message(e)
  }
}
function schedule() {
  if (!loaded.value) return
  dirty.value = true
  saveState.value = '尚未保存'
  try {
    localStorage.setItem(cacheKey.value, JSON.stringify(body()))
  } catch {
    error.value = '本机暂存空间不足，正在尝试同步到服务器，请保持页面打开。'
  }
  clearTimeout(timer)
  timer = setTimeout(() => flush().catch(() => {}), 900)
}
async function flush() {
  clearTimeout(timer)
  if (saving) await saving
  if (!dirty.value || !loaded.value) return
  const snapshot = body()
  saveState.value = '保存中…'
  saving = (async () => {
    try {
      await api('/drafts/' + id.value, { method: 'PUT', body: snapshot })
      if (JSON.stringify(snapshot) === JSON.stringify(body())) {
        dirty.value = false
        localStorage.removeItem(cacheKey.value)
      }
      saveState.value = dirty.value ? '尚未保存' : '草稿已保存'
    } catch (e) {
      saveState.value = '保存失败'
      error.value = message(e)
      throw e
    }
  })()
  try {
    await saving
  } finally {
    saving = undefined
  }
}
watch(form, schedule, { deep: true, flush: 'sync' })
function selectTag(tid: string) {
  form.tag_ids.push(tid)
  tagInput.value = ''
  showTagSearch.value = false
}
async function createTag() {
  try {
    const tag = await api('/tags', {
      method: 'POST',
      body: { name: tagInput.value.trim(), description: '', show_in_nav: false },
    })
    await refreshTags()
    selectTag(tag.id)
  } catch (e) {
    error.value = message(e)
  }
}
function changePrecision() {
  if (!form.date_precision) form.occurred_at = ''
  else if (form.occurred_at) {
    const length = { year: 4, month: 7, day: 10 }[form.date_precision as 'year' | 'month' | 'day']
    form.occurred_at = String(form.occurred_at).slice(0, length)
  }
}
async function publish() {
  error.value = ''
  publishing.value = true
  try {
    await flush()
    const a = await api<Article>('/articles/' + id.value, { method: 'PUT', body: body() })
    dirty.value = false
    localStorage.removeItem(cacheKey.value)
    await refreshTags()
    await router.push('/articles/' + a.id)
  } catch (e) {
    error.value = message(e)
    if (e instanceof ApiError && e.code === 'REVISION_CONFLICT')
      latest.value = await api<Article>('/articles/' + id.value)
  } finally {
    publishing.value = false
  }
}
async function searchLinks() {
  try {
    linkResults.value = (
      await api<Page<Article>>('/articles?q=' + encodeURIComponent(linkQuery.value))
    ).items.filter((a) => a.id !== id.value)
  } catch (e) {
    error.value = message(e)
  }
}
function chooseLink(href: string, title?: string) {
  rich.value?.link(href, title)
  linkOpen.value = false
  externalLink.value = ''
}
interface Upload {
  key: string
  file: File
  progress: number
  status: 'queued' | 'uploading' | 'done' | 'error' | 'cancelled'
  error: string
  mediaId?: string
  xhr?: XMLHttpRequest
}
const uploads = ref<Upload[]>([])
const uploading = computed(() =>
  uploads.value.some((j) => ['queued', 'uploading', 'error'].includes(j.status)),
)
function addFiles(files: File[]) {
  for (const file of files) {
    const limit = file.type.startsWith('image/') ? 20 : 200
    if (file.size > limit * 1024 * 1024) {
      error.value = `${file.name} 超过 ${limit} MiB 上限`
      continue
    }
    uploads.value.push({ key: crypto.randomUUID(), file, progress: 0, status: 'queued', error: '' })
  }
  pump()
}
function pump() {
  let active = uploads.value.filter((j) => j.status === 'uploading').length
  for (const job of uploads.value) {
    if (active >= 2) break
    if (job.status === 'queued') {
      active++
      job.status = 'uploading'
      runUpload(job).finally(pump)
    }
  }
}
function transfer(job: Upload, url: string, headers: Record<string, string>) {
  return new Promise<void>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    job.xhr = xhr
    xhr.open('PUT', url)
    if (url.startsWith('/')) {
      xhr.withCredentials = true
      xhr.setRequestHeader('X-CSRF-Token', session.csrf_token)
    }
    for (const [k, v] of Object.entries(headers)) xhr.setRequestHeader(k, v)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) job.progress = Math.round((e.loaded / e.total) * 95)
    }
    xhr.onload = () =>
      xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new Error('文件传输失败，请重试'))
    xhr.onerror = () => reject(new Error('网络中断，请重试'))
    xhr.onabort = () => reject(new Error('上传已取消'))
    xhr.send(job.file)
  })
}
async function runUpload(job: Upload) {
  try {
    if (job.mediaId) {
      try {
        const m = await api<Media>('/media/uploads/' + job.mediaId + '/complete', {
          method: 'POST',
          body: {},
        })
        if (job.status !== 'cancelled') {
          rich.value?.insertMedia(m.id, job.file.type.startsWith('video/'), job.file.name)
          job.status = 'done'
          job.progress = 100
        }
        return
      } catch {
        await api('/media/uploads/' + job.mediaId, { method: 'DELETE' }).catch(() => {})
        job.mediaId = undefined
      }
    }
    const intent = await api<{ media: Media; url: string; headers: Record<string, string> }>(
      '/media/uploads',
      {
        method: 'POST',
        body: {
          name: job.file.name,
          size: job.file.size,
          content_type: job.file.type || (/\.mov$/i.test(job.file.name) ? 'video/quicktime' : ''),
        },
      },
    )
    job.mediaId = intent.media.id
    if (job.status === 'cancelled') {
      await api('/media/uploads/' + job.mediaId, { method: 'DELETE' })
      return
    }
    await transfer(job, intent.url, intent.headers)
    const m = await api<Media>('/media/uploads/' + job.mediaId + '/complete', {
      method: 'POST',
      body: {},
    })
    if (isCancelled(job)) return
    rich.value?.insertMedia(m.id, job.file.type.startsWith('video/'), job.file.name)
    job.progress = 100
    job.status = 'done'
  } catch (e) {
    if (job.status !== 'cancelled') {
      job.status = 'error'
      job.error = message(e)
    }
  }
}
function cancel(job: Upload) {
  job.status = 'cancelled'
  job.xhr?.abort()
  if (job.mediaId) api('/media/uploads/' + job.mediaId, { method: 'DELETE' }).catch(() => {})
}
function isCancelled(job: Upload) {
  return job.status === 'cancelled'
}
function discardRecovery() {
  localStorage.removeItem(cacheKey.value)
  recovery.value = undefined
}
function retry(job: Upload) {
  job.error = ''
  job.status = 'queued'
  pump()
}
function beforeUnload(e: BeforeUnloadEvent) {
  if (dirty.value || uploading.value) {
    e.preventDefault()
    e.returnValue = ''
  }
}
function restoreDeviceDraft() {
  if (recovery.value) Object.assign(form, recovery.value)
  recovery.value = undefined
}
function acknowledgeConflict() {
  if (!latest.value) return
  form.expected_revision = latest.value.revision
  latest.value = undefined
  error.value = ''
}
function openLinks() {
  linkOpen.value = true
  searchLinks()
}
function selectFiles(event: Event) {
  const input = event.target as HTMLInputElement
  addFiles(Array.from(input.files || []))
  input.value = ''
}
function chooseFirstTag() {
  const tag = matchingTags.value[0]
  if (tag) selectTag(tag.id)
  else if (tagInput.value.trim()) createTag()
}

onMounted(() => {
  load()
  window.addEventListener('beforeunload', beforeUnload)
})
onBeforeRouteLeave(async () => {
  if (publishing.value) return true
  try {
    await flush()
    return true
  } catch {
    return false
  }
})
onBeforeUnmount(() => {
  clearTimeout(timer)
  window.removeEventListener('beforeunload', beforeUnload)
  for (const job of uploads.value)
    if (job.status === 'uploading' || job.status === 'queued') cancel(job)
})
</script>
<template>
  <div class="editor-top">
    <RouterLink :to="form.expected_revision ? '/articles/' + id : '/'" class="back-link"
      ><Icon name="back" :size="16" />{{
        form.expected_revision ? '返回文章' : '返回 Wiki'
      }}</RouterLink
    >
    <div class="editor-save">
      <span role="status" :class="{ error: saveState === '保存失败' }">{{ saveState }}</span
      ><button class="quiet-button" @click="preview = !preview" :disabled="uploading">
        {{ preview ? '继续编辑' : '预览' }}</button
      ><button class="button" :disabled="publishing || !loaded || uploading" @click="publish">
        {{ publishing ? '发布中…' : '发布文章' }}
      </button>
    </div>
  </div>
  <div v-if="error" class="notice error" role="alert">
    {{ error
    }}<button v-if="saveState === '保存失败'" @click="flush().catch(() => {})">重新保存</button>
  </div>
  <div v-if="!loaded && !error" class="loading">正在打开编辑器…</div>
  <template v-if="loaded"
    ><div v-if="recovery" class="notice">
      此设备还有一份未同步的草稿。<button @click="restoreDeviceDraft">恢复这份内容</button
      ><button @click="discardRecovery()">使用服务器草稿</button>
    </div>
    <section v-if="latest" class="conflict-panel">
      <h2>发布前，先核对其他人的补充</h2>
      <p>你的草稿已保留。下面是当前已发布的版本，可以把需要的内容补进编辑器。</p>
      <details>
        <summary>查看最新版本（第 {{ latest.revision }} 版）</summary>
        <h3>{{ latest.title }}</h3>
        <DocumentView :document="latest.document" />
      </details>
      <button class="quiet-button" @click="acknowledgeConflict">
        已核对差异，以最新版本为基础继续编辑
      </button>
    </section>
    <div v-if="preview" class="editor-preview">
      <h1>{{ form.title || '未命名文章' }}</h1>
      <DocumentView :document="form.document" />
    </div>
    <div v-else class="editor-sheet">
      <label class="sr-only" for="article-title">文章标题</label
      ><input
        id="article-title"
        v-model="form.title"
        class="title-input"
        placeholder="给这个故事起个名字"
        maxlength="200"
      />
      <div class="editor-properties">
        <div class="property-row">
          <span class="property-label">标签</span>
          <div class="selected-tags">
            <button
              v-for="t in selectedTags"
              :key="t.id"
              class="tag-chip"
              @click="form.tag_ids = form.tag_ids.filter((id) => id !== t.id)"
            >
              {{ t.name }} <Icon name="close" :size="12" />
            </button>
            <div class="tag-picker">
              <input
                v-model="tagInput"
                aria-label="添加或创建标签"
                placeholder="＋ 添加标签"
                @focus="showTagSearch = true"
                @keydown.escape="showTagSearch = false"
                @keydown.enter.prevent="chooseFirstTag"
              />
              <div v-if="showTagSearch" class="tag-options">
                <button v-for="t in matchingTags.slice(0, 10)" :key="t.id" @click="selectTag(t.id)">
                  {{ t.name }}<small>{{ t.description }}</small></button
                ><button
                  v-if="tagInput.trim() && !catalog.tags.some((t) => t.name === tagInput.trim())"
                  @click="createTag"
                >
                  创建“{{ tagInput.trim() }}”</button
                ><button class="muted" @click="showTagSearch = false">收起</button>
              </div>
            </div>
          </div>
        </div>
        <div class="property-row">
          <label class="property-label" for="date-precision">发生时间</label
          ><select id="date-precision" v-model="form.date_precision" @change="changePrecision">
            <option value="">暂不填写</option>
            <option value="year">知道年份</option>
            <option value="month">知道月份</option>
            <option value="day">准确日期</option></select
          ><input
            v-if="form.date_precision"
            v-model="form.occurred_at"
            aria-label="故事发生时间"
            :type="
              form.date_precision === 'day'
                ? 'date'
                : form.date_precision === 'month'
                  ? 'month'
                  : 'text'
            "
            :inputmode="form.date_precision === 'year' ? 'numeric' : undefined"
            :maxlength="form.date_precision === 'year' ? 4 : undefined"
            :placeholder="form.date_precision === 'year' ? '例如 2025' : ''"
            min="1"
            max="9999"
          /><span class="field-hint">不确定的日期可以留到以后。</span>
        </div>
      </div>
      <div class="editor-toolbar" role="toolbar" aria-label="正文格式">
        <button title="二级标题" aria-label="二级标题" @click="rich?.command('heading')">H₂</button
        ><button title="加粗" aria-label="加粗" @click="rich?.command('bold')"><b>B</b></button
        ><button title="斜体" aria-label="斜体" @click="rich?.command('italic')"><i>I</i></button
        ><button title="列表" aria-label="项目列表" @click="rich?.command('list')">☷</button
        ><button title="引用" aria-label="引用段落" @click="rich?.command('quote')">“</button
        ><span class="toolbar-divider" /><button title="链接文章或网页" @click="openLinks">
          <Icon name="link" :size="17" /><span>链接</span></button
        ><button @click="fileInput?.click()">
          <Icon name="image" :size="17" /><span>图片 / 视频</span></button
        ><span class="toolbar-divider" /><button
          title="撤销"
          aria-label="撤销"
          @click="rich?.command('undo')"
        >
          ↶</button
        ><button title="重做" aria-label="重做" @click="rich?.command('redo')">↷</button
        ><input
          ref="fileInput"
          class="sr-only"
          type="file"
          multiple
          accept="image/jpeg,image/png,image/webp,image/gif,video/mp4,video/webm,video/quicktime"
          @change="selectFiles"
        />
      </div>
      <RichEditor ref="rich" :model-value="form.document" @change="changed" @files="addFiles" />
      <div v-if="uploads.some((j) => j.status !== 'cancelled')" class="upload-list">
        <div
          v-for="job in uploads.filter((j) => j.status !== 'cancelled')"
          :key="job.key"
          class="upload-row"
        >
          <Icon :name="job.file.type.startsWith('video/') ? 'video' : 'image'" :size="18" />
          <div>
            <strong>{{ job.file.name }}</strong
            ><progress v-if="job.status === 'uploading'" :value="job.progress" max="100" /><small
              v-if="job.error"
              class="error"
              >{{ job.error }}</small
            ><small v-else>{{
              job.status === 'done'
                ? '已插入正文'
                : job.status === 'queued'
                  ? '等待上传'
                  : job.progress + '%'
            }}</small>
          </div>
          <button v-if="job.status === 'error'" class="text-link" @click="retry(job)">重试</button
          ><button
            v-if="job.status !== 'done'"
            class="icon-button"
            aria-label="取消上传"
            @click="cancel(job)"
          >
            <Icon name="close" :size="16" /></button
          ><Icon v-else name="check" :size="17" />
        </div>
      </div>
      <div class="change-input">
        <label for="change-summary">这次补充了什么 <span>可选</span></label
        ><input
          id="change-summary"
          v-model="form.change_summary"
          placeholder="比如：补上那天的合照和一句金句"
          maxlength="1000"
        />
      </div>
      <p class="editor-help">
        文字会自动保存为草稿。发布后，群友才会看到这次补充。图片上限 20 MiB，视频 200 MiB。
      </p>
    </div></template
  >
  <div v-if="linkOpen" class="modal-shade" @click.self="linkOpen = false">
    <section class="dialog" role="dialog" aria-modal="true" aria-labelledby="link-title">
      <div class="dialog-heading">
        <h2 id="link-title">链接到另一个故事</h2>
        <button class="icon-button" aria-label="关闭链接窗口" @click="linkOpen = false">
          <Icon name="close" />
        </button>
      </div>
      <input
        v-model="linkQuery"
        aria-label="搜索要链接的文章"
        placeholder="搜索文章标题…"
        @input="searchLinks"
      />
      <div class="link-results">
        <button
          v-for="a in linkResults.slice(0, 8)"
          :key="a.id"
          @click="chooseLink('/articles/' + a.id, a.title)"
        >
          {{ a.title }}<Icon name="arrow" :size="15" />
        </button>
        <p v-if="!linkResults.length" class="muted">没有找到文章，换个词试试。</p>
      </div>
      <label for="external-link">也可以链接网页</label>
      <div class="inline-form">
        <input id="external-link" v-model="externalLink" type="url" placeholder="https://" /><button
          class="button"
          :disabled="!/^https?:\/\//.test(externalLink)"
          @click="chooseLink(externalLink, externalLink)"
        >
          插入
        </button>
      </div>
    </section>
  </div>
</template>
