<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  api,
  clearPrivateState,
  editable,
  message,
  refreshSession,
  refreshTags,
  session,
} from '../api'
const route = useRoute(),
  router = useRouter(),
  error = ref(''),
  notice = ref(''),
  busy = ref(false)
const invite = ref(String(route.query.invite || '')),
  joining = ref(!!route.query.invite)
const channel = ref<'email' | 'password'>('email'),
  purpose = ref<'bind' | 'replace' | 'reauth'>('bind'),
  recover = ref(false)
const options = ref({ phone: false, email: false, password: true }),
  identities = ref<{ id: string; kind: string; masked: string }[]>([])
const form = reactive({
  target: '',
  code: '',
  name: '',
  login: '',
  password: '',
  token: '',
  newPassword: '',
})
const challenge = ref(''),
  remaining = ref(0),
  reauthenticated = ref(false)
const mode = computed(() =>
  session.setup_needed ? 'setup' : joining.value ? 'join' : recover.value ? 'recover' : 'login',
)
const codePurpose = computed(() =>
  session.user
    ? purpose.value
    : mode.value === 'join'
      ? 'join'
      : mode.value === 'recover'
        ? 'recover'
        : 'login',
)
const roleLabel = computed(() =>
  session.user
    ? { system_admin: '系统管理员', admin: '管理员', user: '普通用户' }[session.user.role]
    : '',
)
const timer = setInterval(() => {
  if (remaining.value > 0) remaining.value--
}, 1000)
onUnmounted(() => clearInterval(timer))
watch([channel, purpose, joining, recover], () => {
  challenge.value = ''
  form.code = ''
  error.value = ''
})
const sessions = ref<{ id: string; created_at: string; expires_at: string; current: boolean }[]>([])
async function refreshSessions() {
  sessions.value = (await api('/account/sessions')).items
}
async function revokeSession(id?: string) {
  await run(async () => {
    await api('/account/sessions' + (id ? '/' + id : ''), { method: 'DELETE' })
    await refreshSession()
    if (session.user) await refreshSessions()
    else {
      clearPrivateState()
      identities.value = []
      sessions.value = []
      await router.replace('/account')
    }
  })
}
async function load() {
  options.value = await api('/auth/options')
  if (session.user) {
    try {
      identities.value = (await api('/account/identities')).items
      await refreshSessions()
    } catch (e) {
      error.value = message(e)
    }
  }
}
onMounted(async () => {
  if (route.query.invite) await router.replace({ path: '/account', query: {} })
  try {
    await load()
  } catch (e) {
    error.value = message(e)
  }
})
async function run(fn: () => Promise<void>) {
  busy.value = true
  error.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}
async function send() {
  await run(async () => {
    const data = await api('/auth/challenges', {
      method: 'POST',
      body: {
        channel: channel.value,
        purpose: codePurpose.value,
        target: form.target,
        ...(codePurpose.value === 'join' ? { invite: invite.value } : {}),
      },
    })
    challenge.value = data.challenge_id
    remaining.value = data.resend_after
    notice.value = data.message
  })
}
async function finishLogin() {
  await refreshSession()
  await refreshTags()
  identities.value = []
  form.code = ''
  form.password = ''
  challenge.value = ''
  await router.push('/')
}
async function submit() {
  await run(async () => {
    let path = '',
      body: unknown
    if (mode.value === 'setup') {
      path = '/auth/setup'
      body = { token: form.token, name: form.name, login: form.login, password: form.password }
    } else if (!session.user && channel.value === 'password') {
      path = '/auth/login'
      body = { login: form.login, password: form.password }
    } else {
      if (!challenge.value) throw new Error('请先获取验证码')
      const proof = { challenge_id: challenge.value, code: form.code }
      if (session.user) {
        path =
          purpose.value === 'reauth' ? '/account/reauth' : '/account/identities/' + purpose.value
        body = proof
      } else if (mode.value === 'join') {
        path = '/auth/join'
        body = { ...proof, invite: invite.value, nickname: form.name }
      } else {
        path = mode.value === 'recover' ? '/auth/recover' : '/auth/login-code'
        body = proof
      }
    }
    const result = await api(path, { method: 'POST', body })
    if (session.user && path.startsWith('/account/')) {
      if (purpose.value === 'reauth') {
        reauthenticated.value = true
        notice.value = '身份已确认，五分钟内可执行敏感操作'
      } else {
        clearPrivateState()
        await refreshSession()
        identities.value = []
        notice.value = '绑定已更新，请使用新联系方式重新登录'
        challenge.value = ''
        form.code = ''
        await load()
      }
    } else {
      Object.assign(session, result)
      await finishLogin()
    }
  })
}
async function logout() {
  await run(async () => {
    await api('/auth/logout', { method: 'POST', body: {} })
    clearPrivateState()
    await refreshSession()
    identities.value = []
    reauthenticated.value = false
    await router.replace('/account')
  })
}
async function reauthPassword() {
  await run(async () => {
    await api('/account/reauth', { method: 'POST', body: { password: form.password } })
    form.password = ''
    reauthenticated.value = true
    notice.value = '身份已确认，五分钟内可执行敏感操作'
  })
}
async function changePassword() {
  await run(async () => {
    await api('/account/password', { method: 'PUT', body: { password: form.newPassword } })
    form.newPassword = ''
    clearPrivateState()
    await refreshSession()
    identities.value = []
    notice.value = '密码已更新，请重新登录'
  })
}
async function removeIdentity(id: string) {
  if (!confirm('解除此绑定并退出所有会话？')) return
  await run(async () => {
    await api('/account/identities/' + id, { method: 'DELETE' })
    clearPrivateState()
    await refreshSession()
    identities.value = []
    notice.value = '绑定已解除，请重新登录'
  })
}
function toggleJoin() {
  joining.value = !joining.value
  recover.value = false
  channel.value = 'email'
}
function toggleRecover() {
  recover.value = !recover.value
  channel.value = 'email'
}
</script>
<template>
  <section :class="session.user ? 'account-page' : 'auth-page'">
    <h1>
      {{
        session.user
          ? '我的账号'
          : mode === 'setup'
            ? '建立我们的 Wiki'
            : mode === 'join'
              ? '加入这个群的故事'
              : mode === 'recover'
                ? '找回账号'
                : '先登录，再看看群里的故事'
      }}
    </h1>
    <p class="page-description">
      {{ session.user ? '联系方式与登录安全' : '只有受邀成员可以进入，使用邮箱验证码登录。' }}
    </p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <template v-if="session.user">
      <dl class="account-details">
        <div>
          <dt>昵称</dt>
          <dd>{{ session.user.name }}</dd>
        </div>
        <div>
          <dt>身份</dt>
          <dd>{{ roleLabel }}{{ !editable() ? '（只读）' : '' }}</dd>
        </div>
        <div v-for="kind in ['email']" :key="kind">
          <dt>邮箱</dt>
          <dd>
            {{ identities.find((x) => x.kind === kind)?.masked || '未绑定' }}
            <button
              v-if="identities.find((x) => x.kind === kind)"
              class="text-link"
              @click="removeIdentity(identities.find((x) => x.kind === kind)!.id)"
            >
              解除
            </button>
          </dd>
        </div>
      </dl>
      <h2>确认本人身份</h2>
      <form @submit.prevent="reauthPassword">
        <label for="reauth-password">已有登录密码</label
        ><input
          id="reauth-password"
          v-model="form.password"
          type="password"
          autocomplete="current-password"
          required
        /><button class="quiet-button" :disabled="busy">重新验证</button>
      </form>
      <p class="field-hint">
        更换绑定、解除绑定或修改密码前请先重新验证。没有密码的普通成员可选择“通过已有联系方式重新验证”。系统管理员使用密码验证。
      </p>
      <h2>绑定联系方式</h2>
      <label for="identity-purpose">操作</label
      ><select id="identity-purpose" v-model="purpose">
        <option value="bind">首次绑定</option>
        <option value="replace">更换绑定</option>
        <option v-if="session.user.role !== 'system_admin'" value="reauth">
          通过已有联系方式重新验证
        </option>
      </select>
    </template>
    <template v-if="mode !== 'setup'">
      <div class="inline-form auth-channel" aria-label="验证方式">
        <button
          type="button"
          :class="channel === 'email' ? 'button' : 'quiet-button'"
          @click="channel = 'email'"
        >
          邮箱验证码</button
        ><button
          v-if="!session.user && !joining"
          type="button"
          :class="channel === 'password' ? 'button' : 'quiet-button'"
          @click="channel = 'password'"
        >
          密码登录
        </button>
      </div>
    </template>
    <form @submit.prevent="submit">
      <template v-if="mode === 'setup'"
        ><label for="setup-token">一次性初始化凭证</label
        ><input id="setup-token" v-model="form.token" required autocomplete="off" /><label
          for="setup-name"
          >昵称</label
        ><input id="setup-name" v-model="form.name" required maxlength="40"
      /></template>
      <template v-if="mode === 'join' && !session.user"
        ><label for="invite">邀请凭证</label
        ><input id="invite" v-model="invite" required autocomplete="off" /><label for="nickname"
          >昵称</label
        ><input id="nickname" v-model="form.name" required maxlength="40"
      /></template>
      <template v-if="mode === 'setup' || channel === 'password'"
        ><label for="login">账号</label
        ><input id="login" v-model="form.login" required autocomplete="username" /><label
          for="password"
          >密码</label
        ><input
          id="password"
          v-model="form.password"
          type="password"
          required
          :minlength="mode === 'setup' ? 10 : 1"
          :autocomplete="mode === 'setup' ? 'new-password' : 'current-password'"
      /></template>
      <template v-else
        ><label for="target">邮箱地址</label
        ><input
          id="target"
          v-model="form.target"
          type="email"
          autocomplete="email"
          required
        /><label for="code">验证码</label>
        <div class="inline-form">
          <input
            id="code"
            v-model="form.code"
            inputmode="numeric"
            autocomplete="one-time-code"
            pattern="[0-9]{6}"
            maxlength="6"
            required
          /><button
            type="button"
            class="quiet-button"
            :disabled="busy || remaining > 0 || !options[channel]"
            @click="send"
          >
            {{ remaining > 0 ? remaining + ' 秒后重发' : '获取验证码' }}
          </button>
        </div>
        <p v-if="!options[channel]" class="field-hint">
          此渠道尚未就绪，可使用其他登录方式。
        </p></template
      >
      <button class="button auth-submit" :disabled="busy">
        {{
          busy
            ? '正在处理…'
            : session.user
              ? '确认'
              : mode === 'setup'
                ? '创建 Wiki'
                : mode === 'join'
                  ? '验证并加入'
                  : '登录'
        }}
      </button>
    </form>
    <template v-if="session.user"
      ><h2>修改登录密码</h2>
      <form @submit.prevent="changePassword">
        <label for="new-password">新密码（10 至 72 字节）</label
        ><input
          id="new-password"
          v-model="form.newPassword"
          type="password"
          minlength="10"
          required
          autocomplete="new-password"
        /><button class="quiet-button" :disabled="busy || !reauthenticated">
          更新密码并退出会话
        </button>
      </form>
      <h2>登录会话</h2>
      <p class="field-hint">撤销后，该会话不能继续读取文章或媒体。</p>
      <div v-for="item in sessions" :key="item.id" class="member-row">
        <span>{{ item.current ? '当前浏览器' : '其他会话' }}</span
        ><button class="text-link" @click="revokeSession(item.id)">退出此会话</button>
      </div>
      <button class="quiet-button" @click="revokeSession()">退出所有会话</button>
      <RouterLink v-if="editable()" class="text-link" to="/drafts">我的草稿</RouterLink
      ><button class="quiet-button logout-button" @click="logout">退出登录</button></template
    >
    <template v-else-if="mode !== 'setup'"
      ><button class="text-link auth-switch" @click="toggleJoin">
        {{ joining ? '已有账号，去登录' : '使用邀请加入' }}</button
      ><button v-if="!joining" class="text-link auth-switch" @click="toggleRecover">
        {{ recover ? '返回登录' : '找回账号' }}
      </button></template
    >
  </section>
</template>
