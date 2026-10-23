<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, message, refreshTags } from '../api'
const data = ref<{
    counts: Record<string, number>
    trash: { id: string; title: string; deleted_at: string }[]
    public_read: boolean
    storage: string
  }>(),
  error = ref(''),
  invite = ref(''),
  role = ref('editor'),
  saving = ref(false),
  notice = ref('')
async function load() {
  try {
    data.value = await api('/admin')
  } catch (e) {
    error.value = message(e)
  }
}
async function restore(id: string) {
  try {
    await api('/articles/' + id + '/restore', { method: 'POST', body: {} })
    await load()
    await refreshTags()
    notice.value = '文章已恢复'
  } catch (e) {
    error.value = message(e)
  }
}
async function makeInvite() {
  saving.value = true
  try {
    const result = await api('/admin/invites', { method: 'POST', body: { role: role.value } })
    invite.value = window.location.origin + '/account?invite=' + encodeURIComponent(result.invite)
  } catch (e) {
    error.value = message(e)
  } finally {
    saving.value = false
  }
}
async function save() {
  try {
    await api('/admin/settings', {
      method: 'PATCH',
      body: { public_read: data.value?.public_read },
    })
    notice.value = '阅读设置已保存'
  } catch (e) {
    error.value = message(e)
  }
}
interface Member {
  id: string
  name: string
  login: string
  role: string
  disabled: boolean
}
const members = ref<Member[]>([]),
  memberCursor = ref('')
async function loadMembers(next = false) {
  try {
    const page = await api(
      '/admin/members' + (next ? '?cursor=' + encodeURIComponent(memberCursor.value) : ''),
    )
    members.value = next ? [...members.value, ...page.items] : page.items
    memberCursor.value = page.next_cursor
  } catch (e) {
    error.value = message(e)
  }
}
async function saveMember(member: Member) {
  try {
    await api('/admin/members/' + member.id, {
      method: 'PATCH',
      body: { role: member.role, disabled: member.disabled },
    })
    notice.value = '成员权限已更新'
  } catch (e) {
    error.value = message(e)
    await loadMembers()
  }
}
onMounted(() => {
  load()
  loadMembers()
})
</script>
<template>
  <div class="page-heading">
    <div>
      <h1>照看这个 Wiki</h1>
      <p class="page-description">成员、阅读范围，以及故事的备份。</p>
    </div>
  </div>
  <p v-if="error" class="notice error" role="alert">{{ error }}</p>
  <p v-if="notice" class="notice" role="status">{{ notice }}</p>
  <div v-if="data" class="admin-content">
    <section class="settings-section">
      <h2>空间概况</h2>
      <div class="admin-counts">
        <span
          ><strong>{{ data.counts.articles }}</strong
          >篇文章</span
        ><span
          ><strong>{{ data.counts.members }}</strong
          >位成员</span
        ><span
          ><strong>{{ data.counts.media }}</strong
          >个媒体文件</span
        >
      </div>
    </section>
    <section class="settings-section">
      <h2>谁可以阅读</h2>
      <label class="checkbox-label"
        ><input v-model="data.public_read" type="checkbox" />允许任何人阅读已发布的文章</label
      >
      <p class="muted">关闭后，只有已登录的成员可读。个人草稿始终不会公开。</p>
      <button class="quiet-button" @click="save">保存阅读设置</button>
    </section>
    <section class="settings-section">
      <h2>邀请一个群友</h2>
      <p class="muted">邀请一次有效，七天内可以使用。</p>
      <div class="inline-form">
        <select v-model="role" aria-label="邀请成员的角色">
          <option value="editor">编辑者：可以补充文章</option>
          <option value="viewer">阅读者：只能阅读</option></select
        ><button class="button" :disabled="saving" @click="makeInvite">生成邀请</button>
      </div>
      <label v-if="invite" class="invite-result"
        >把这个链接交给群友<input
          :value="invite"
          readonly
          aria-label="邀请链接"
          @focus="($event.target as HTMLInputElement).select()"
      /></label>
    </section>
    <section class="settings-section">
      <h2>空间成员</h2>
      <p class="muted">停用后会立即退出这个成员的会话，署名和文章仍会保留。</p>
      <div v-for="member in members" :key="member.id" class="member-row">
        <div>
          <strong>{{ member.name }}</strong
          ><small>{{ member.login }}</small>
        </div>
        <select v-model="member.role" :aria-label="member.name + '的角色'">
          <option value="viewer">阅读者</option>
          <option value="editor">编辑者</option>
          <option value="admin">管理员</option></select
        ><label class="checkbox-label"
          ><input v-model="member.disabled" type="checkbox" />停用</label
        ><button class="quiet-button" @click="saveMember(member)">保存</button>
      </div>
      <button v-if="memberCursor" class="text-link" @click="loadMembers(true)">查看更多成员</button>
    </section>
    <section class="settings-section">
      <h2>留一份备份</h2>
      <p class="muted">
        备份包括文章、标签、历史版本和已完成的媒体文件。请妥善保存，其中包含空间的私有内容。
      </p>
      <a class="quiet-button" href="/api/v1/admin/backup">下载完整备份</a>
    </section>
    <section class="settings-section">
      <h2>回收站</h2>
      <p v-if="!data.trash.length" class="muted">没有放入回收站的文章。</p>
      <div v-for="a in data.trash" :key="a.id" class="trash-row">
        <span>{{ a.title || '未命名文章' }}</span
        ><button class="text-link" @click="restore(a.id)">恢复文章</button>
      </div>
    </section>
  </div>
</template>
