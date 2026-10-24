<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, clearPrivateState, message, refreshSession, refreshTags, session } from '../api'
const router = useRouter()
async function loginAgain() {
  try {
    await api('/auth/logout', { method: 'POST', body: {} })
    clearPrivateState()
    await refreshSession()
    await router.push('/account')
  } catch (e) {
    error.value = message(e)
  }
}

const data = ref<{
    counts: Record<string, number>
    trash: { id: string; title: string; deleted_at: string }[]
    public_read: boolean
    storage: string
  }>(),
  error = ref(''),
  invite = ref(''),
  role = ref('editor'),
  inviteNote = ref(''),
  saving = ref(false),
  notice = ref('')
const invitations = ref<
    {
      id: string
      role: string
      state: string
      expires_at: string
      note: string
      created_at: string
      created_by_name: string
      consumed_at: string
      consumed_by_name: string
    }[]
  >([]),
  inviteCursor = ref(''),
  authStatus = ref<{ phone_configured: boolean; email_configured: boolean; storage: string }>(),
  audit = ref<{ id: string; event: string; result: string; created_at: string }[]>([])
async function loadInvites(next = false) {
  const p = await api(
    '/admin/invites' + (next ? '?cursor=' + encodeURIComponent(inviteCursor.value) : ''),
  )
  invitations.value = next ? [...invitations.value, ...p.items] : p.items
  inviteCursor.value = p.next_cursor
}
async function revokeInvite(id: string) {
  try {
    await api('/admin/invites/' + id, { method: 'DELETE' })
    invite.value = ''
    await loadInvites()
    notice.value = '邀请已撤销'
  } catch (e) {
    error.value = message(e)
  }
}
async function loadSecurity() {
  try {
    await loadInvites()
    if (session.user?.role === 'system_admin') {
      authStatus.value = await api('/system/auth-status')
      audit.value = (await api('/system/audit')).items
    }
  } catch (e) {
    error.value = message(e)
  }
}
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
    const result = await api('/admin/invites', {
      method: 'POST',
      body: { role: role.value, note: inviteNote.value },
    })
    invite.value = window.location.origin + '/account?invite=' + encodeURIComponent(result.invite)
    inviteNote.value = ''
    await loadInvites()
  } catch (e) {
    error.value = message(e)
  } finally {
    saving.value = false
  }
}
interface Member {
  id: string
  name: string
  login: string
  role: string
  disabled: boolean
  can_edit: boolean
  savedRole: string
  savedCanEdit: boolean
}
const members = ref<Member[]>([]),
  memberCursor = ref('')
async function loadMembers(next = false) {
  try {
    const page = await api(
      '/admin/members' + (next ? '?cursor=' + encodeURIComponent(memberCursor.value) : ''),
    )
    const items = page.items.map((m: Member) => ({
      ...m,
      savedRole: m.role,
      savedCanEdit: m.can_edit,
    }))
    members.value = next ? [...members.value, ...items] : items
    memberCursor.value = page.next_cursor
  } catch (e) {
    error.value = message(e)
  }
}
async function saveMember(member: Member) {
  try {
    await api('/admin/members/' + member.id, {
      method: 'PATCH',
      body: { role: member.role, disabled: member.disabled, can_edit: member.can_edit },
    })
    member.savedRole = member.role
    member.savedCanEdit = member.can_edit
    notice.value = '成员权限已更新'
  } catch (e) {
    error.value = message(e)
    await loadMembers()
  }
}
const changingMember = ref('')
async function toggleDisabled(member: Member) {
  changingMember.value = member.id
  try {
    await api('/admin/members/' + member.id, {
      method: 'PATCH',
      body: { role: member.savedRole, can_edit: member.savedCanEdit, disabled: !member.disabled },
    })
    notice.value = member.disabled
      ? '已解除禁用，成员可用新的邮箱验证码登录。'
      : '已禁用，所有登录和未使用验证码已失效。'
    await loadMembers()
  } catch (e) {
    error.value = message(e)
  } finally {
    changingMember.value = ''
  }
}
onMounted(() => {
  load()
  loadMembers()
  loadSecurity()
})
function inviteState(value: string) {
  return (
    (
      { active: '可使用', consumed: '已使用', revoked: '已撤销', expired: '已过期' } as Record<
        string,
        string
      >
    )[value] || value
  )
}
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
      <h2>阅读范围</h2>
      <p>只有已登录的成员可以查看文章、搜索与媒体。</p>
      <p class="muted">权限变更和备份需要最近五分钟内的邮箱验证码登录。</p>
      <button class="text-link" @click="loginAgain">重新登录确认身份</button>
    </section>
    <section class="settings-section">
      <h2>邀请一个群友</h2>
      <p class="muted">每条链接仅限一人加入，七天内有效。备注只供管理员查看。</p>
      <label class="invite-note" for="invite-note">创建备注（可选）</label>
      <input
        id="invite-note"
        v-model="inviteNote"
        class="invite-note-input"
        maxlength="200"
        placeholder="例如：邀请周末聚会认识的群友"
      />
      <div class="inline-form">
        <select v-model="role" aria-label="邀请成员的角色">
          <option value="editor">编辑者：可以补充文章</option>
          <option value="viewer">阅读者：只能阅读</option></select
        ><button class="button" :disabled="saving" @click="makeInvite">生成一次性邀请链接</button>
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
      <h2>邀请记录</h2>
      <div v-for="item in invitations" :key="item.id" class="member-row">
        <div>
          <strong>{{ item.note || '未填写备注' }}</strong>
          <small
            >{{ inviteState(item.state) }} ·
            {{ item.role === 'viewer' ? '只读' : '普通成员' }}</small
          >
          <small
            >{{ item.created_by_name || '管理员' }} 创建<span v-if="item.created_at"
              >于 {{ new Date(item.created_at).toLocaleString('zh-CN') }}</span
            ></small
          >
          <small v-if="item.consumed_at"
            >{{ item.consumed_by_name || '成员' }} 于
            {{ new Date(item.consumed_at).toLocaleString('zh-CN') }} 加入</small
          >
        </div>
        <button v-if="item.state === 'active'" class="text-link" @click="revokeInvite(item.id)">
          撤销邀请
        </button>
      </div>
      <button v-if="inviteCursor" class="text-link" @click="loadInvites(true)">更多邀请</button>
    </section>
    <section v-if="authStatus" class="settings-section">
      <h2>验证渠道</h2>
      <p>
        邮箱：{{ authStatus.email_configured ? '已配置' : '未配置' }}；媒体：{{
          authStatus.storage
        }}
      </p>
      <p class="muted">配置完整不等于实际投递成功。真实验证码还需要收件人确认。</p>
      <h2>最近安全记录</h2>
      <div v-for="item in audit" :key="item.id" class="member-row">
        <span>{{ item.event }} · {{ item.result }}</span
        ><time>{{ new Date(item.created_at).toLocaleString() }}</time>
      </div>
    </section>
    <section class="settings-section">
      <h2>空间成员</h2>
      <p class="muted">
        禁用后无法登录，已有登录立即失效；解除禁用后需要重新获取验证码。文章和署名保留。
      </p>
      <div v-for="member in members" :key="member.id" class="member-row">
        <div>
          <strong>{{ member.name }}</strong
          ><small>{{ member.disabled ? '已禁用' : '正常' }}</small>
        </div>
        <select v-model="member.role" :aria-label="member.name + '的角色'">
          <option value="user">普通用户</option>
          <option v-if="session.user?.role === 'system_admin'" value="admin">管理员</option>
          <option v-if="session.user?.role === 'system_admin'" value="system_admin">
            系统管理员
          </option></select
        ><label class="checkbox-label"
          ><input
            v-model="member.can_edit"
            type="checkbox"
            :disabled="member.role !== 'user'"
          />可编辑</label
        ><button class="quiet-button" @click="saveMember(member)">保存</button
        ><button
          class="text-link"
          :disabled="changingMember !== ''"
          @click="toggleDisabled(member)"
        >
          {{ member.disabled ? '解除禁用' : '禁用' }}
        </button>
      </div>
      <button v-if="memberCursor" class="text-link" @click="loadMembers(true)">查看更多成员</button>
    </section>
    <section class="settings-section">
      <h2>留一份备份</h2>
      <p class="muted">
        备份包括文章、标签、历史版本和已完成的媒体文件。请妥善保存，其中包含空间的私有内容。
      </p>
      <a
        v-if="session.user?.role === 'system_admin'"
        class="quiet-button"
        href="/api/v1/admin/backup"
        >下载完整备份</a
      >
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
