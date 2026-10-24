<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { api, catalog, editable, message, refreshTags, session } from '../api'
import type { Tag } from '../types'
import Icon from '../components/Icon.vue'
const search = ref(''),
  open = ref(false),
  editing = ref<string>(),
  error = ref(''),
  busy = ref(false),
  deleting = ref(false)
const form = reactive({ name: '', description: '', show_in_nav: false, nav_order: 0 })
const filtered = computed(() =>
  catalog.tags.filter((t) => t.name.includes(search.value) || t.description.includes(search.value)),
)
function edit(tag?: Tag) {
  editing.value = tag?.id
  Object.assign(
    form,
    tag
      ? {
          name: tag.name,
          description: tag.description,
          show_in_nav: tag.show_in_nav,
          nav_order: tag.nav_order,
        }
      : { name: '', description: '', show_in_nav: false, nav_order: catalog.tags.length },
  )
  open.value = true
  deleting.value = false
  error.value = ''
}
async function save() {
  busy.value = true
  error.value = ''
  try {
    await api('/tags' + (editing.value ? '/' + editing.value : ''), {
      method: editing.value ? 'PATCH' : 'POST',
      body: { ...form },
    })
    await refreshTags()
    open.value = false
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}
async function remove() {
  busy.value = true
  try {
    await api('/tags/' + editing.value, { method: 'DELETE' })
    await refreshTags()
    open.value = false
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <div class="page-heading">
    <div>
      <h1>全部标签</h1>
      <p class="page-description">同一段故事，可以从不同的入口找到。</p>
    </div>
    <button v-if="editable()" class="quiet-button" @click="edit()">
      <Icon name="plus" :size="17" />新建标签
    </button>
  </div>
  <label class="tag-search"
    ><Icon name="search" :size="18" /><input
      v-model="search"
      aria-label="搜索标签"
      placeholder="找一个标签…"
  /></label>
  <div class="tag-directory">
    <section v-for="t in filtered" :key="t.id">
      <RouterLink :to="'/tags/' + t.id"
        ><h2>
          {{ t.name }}<span>{{ t.count }} 篇</span>
        </h2>
        <p>{{ t.description || '还没有说明，等你来补充。' }}</p></RouterLink
      ><button
        v-if="editable()"
        class="icon-button"
        :aria-label="'编辑标签 ' + t.name"
        @click="edit(t)"
      >
        <Icon name="edit" :size="16" />
      </button>
    </section>
  </div>
  <div v-if="!filtered.length" class="empty-state">
    <h2>{{ search ? '还没有这个标签' : '还没有标签' }}</h2>
    <p>
      {{ search ? '可以换个词，或创建一个新的。' : '用人物、事件或一个群梗，为故事添加入口。' }}
    </p>
  </div>
  <div v-if="open" class="modal-shade" @click.self="open = false">
    <form
      class="dialog"
      role="dialog"
      aria-modal="true"
      aria-labelledby="tag-title"
      @submit.prevent="save"
    >
      <div class="dialog-heading">
        <h2 id="tag-title">{{ editing ? '编辑标签' : '新建标签' }}</h2>
        <button type="button" class="icon-button" aria-label="关闭标签编辑" @click="open = false">
          <Icon name="close" />
        </button>
      </div>
      <div v-if="error" class="notice error" role="alert">{{ error }}</div>
      <label for="tag-name">名称</label
      ><input
        id="tag-name"
        v-model="form.name"
        required
        maxlength="40"
        placeholder="比如：聚会"
      /><label for="tag-description">说明</label
      ><textarea
        id="tag-description"
        v-model="form.description"
        maxlength="300"
        rows="3"
        placeholder="这个标签收录什么故事？"
      /><label class="checkbox-label"
        ><input v-model="form.show_in_nav" type="checkbox" />放到左侧常用标签</label
      ><label v-if="form.show_in_nav" for="tag-order">导航顺序</label
      ><input
        v-if="form.show_in_nav"
        id="tag-order"
        v-model.number="form.nav_order"
        type="number"
      />
      <p class="field-hint">每个标签都有自己的文章列表，是否放进导航可以随时调整。</p>
      <div v-if="deleting" class="notice error">
        这个标签关联
        {{ catalog.tags.find((t) => t.id === editing)?.count || 0 }}
        篇文章。删除只会移除标签关联，文章仍会保留。<button
          type="button"
          :disabled="busy"
          @click="remove"
        >
          确认删除标签
        </button>
      </div>
      <div class="dialog-actions">
        <button
          v-if="
            editing && (session.user?.role === 'admin' || session.user?.role === 'system_admin')
          "
          type="button"
          class="delete-link"
          @click="deleting = true"
        >
          删除标签</button
        ><button type="button" class="quiet-button" @click="open = false">取消</button
        ><button class="button" :disabled="busy">{{ busy ? '保存中…' : '保存标签' }}</button>
      </div>
    </form>
  </div>
</template>
