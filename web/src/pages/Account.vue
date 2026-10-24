<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, clearPrivateState, message, refreshSession, refreshTags, session } from '../api'
const route = useRoute(),
  router = useRouter()
const invite = ref(String(route.query.invite || ''))
const joining = ref(!!invite.value)
const error = ref(''),
  notice = ref(''),
  busy = ref(false),
  emailReady = ref(false)
const email = ref<{ id: string; masked: string } | null>(null)
const records = ref<{ id: string; current: boolean; created_at: string }[]>([])
const form = reactive({ target: '', code: '', name: '', token: '' })
const challenge = ref(''),
  remaining = ref(0)
const purpose = computed(() => {
  if (session.setup_needed) return 'setup'
  return joining.value ? 'join' : 'login'
})
const roleLabel = computed(
  () =>
    ({ system_admin: '系统管理员', admin: '管理员', user: '群成员' })[session.user?.role || 'user'],
)
const title = computed(() =>
  session.user
    ? '我的账号'
    : session.setup_needed
      ? '建立我们的 Wiki'
      : joining.value
        ? '加入这个群的故事'
        : '先登录，再看看群里的故事',
)
const timer = setInterval(() => {
  if (remaining.value > 0) remaining.value--
}, 1000)
onUnmounted(() => clearInterval(timer))
watch(purpose, () => {
  challenge.value = ''
  form.code = ''
  remaining.value = 0
  error.value = ''
})
async function load() {
  emailReady.value = (await api('/auth/options')).email
  const id = session.user?.id
  if (id) {
    const [identities, sessions] = await Promise.all([
      api('/account/identities'),
      api('/account/sessions'),
    ])
    if (session.user?.id !== id) return
    email.value = identities.items.find((i: { kind: string }) => i.kind === 'email') || null
    records.value = sessions.items
  }
}
watch(
  () => session.user?.id,
  (id) => {
    if (id)
      load().catch((e) => {
        error.value = message(e)
      })
  },
)
onMounted(async () => {
  if (route.query.invite) await router.replace({ path: '/account', query: {} })
  await run(load)
})
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await action()
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
        channel: 'email',
        purpose: purpose.value,
        target: form.target,
        ...(purpose.value === 'join' ? { invite: invite.value } : {}),
        ...(purpose.value === 'setup' ? { setup_token: form.token } : {}),
      },
    })
    challenge.value = data.challenge_id
    remaining.value = data.resend_after
    notice.value = data.message
  })
}
async function submit() {
  await run(async () => {
    if (!challenge.value) throw new Error('请先获取验证码')
    const proof = { challenge_id: challenge.value, code: form.code }
    let path = '/auth/login-code',
      body: Record<string, string> = proof
    if (purpose.value === 'setup') {
      path = '/auth/setup'
      body = { ...proof, token: form.token, name: form.name }
    } else if (purpose.value === 'join') {
      path = '/auth/join'
      body = { ...proof, invite: invite.value, nickname: form.name }
    }
    await api(path, { method: 'POST', body })
    await refreshSession()
    await refreshTags()
    form.code = ''
    challenge.value = ''
    notice.value = ''
    await router.push('/')
  })
}
async function endSession(text = '') {
  clearPrivateState()
  email.value = null
  records.value = []
  form.code = ''
  form.target = ''
  challenge.value = ''
  remaining.value = 0
  await refreshSession()
  notice.value = text
  await router.replace('/account')
}
async function logout(all = false) {
  await run(async () => {
    await api(all ? '/account/sessions' : '/auth/logout', {
      method: all ? 'DELETE' : 'POST',
      ...(all ? {} : { body: {} }),
    })
    await endSession()
  })
}
async function revoke(id: string) {
  await run(async () => {
    await api('/account/sessions/' + id, { method: 'DELETE' })
    await load()
    notice.value = '这条登录已退出。'
  })
}
function leaveInvite() {
  joining.value = false
  invite.value = ''
  form.code = ''
  challenge.value = ''
  notice.value = ''
}
</script>
<template>
  <section :class="session.user ? 'account-page' : 'auth-page'">
    <h1>{{ title }}</h1>
    <p class="page-description">
      {{ session.user ? '管理邮箱和登录记录。' : '使用邮箱验证码，无需设置密码。' }}
    </p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <template v-if="session.user">
      <div class="account-profile">
        <span class="account-monogram" aria-hidden="true">{{ session.user.name.slice(0, 1) }}</span>
        <div>
          <strong>{{ session.user.name }}</strong>
          <p>{{ roleLabel }}{{ session.user.can_edit === false ? ' · 只读' : '' }}</p>
        </div>
      </div>
      <section class="account-section" aria-labelledby="email-heading">
        <h2 id="email-heading">登录邮箱</h2>
        <p class="account-value">{{ email?.masked || '尚未绑定邮箱' }}</p>
        <p class="account-caption">用于接收登录验证码。</p>
      </section>
      <section class="account-section" aria-labelledby="sessions-heading">
        <div class="account-row">
          <div>
            <h2 id="sessions-heading">登录记录</h2>
            <p class="account-caption">不认识的登录可以单独退出。</p>
          </div>
          <button
            v-if="records.length > 1"
            class="text-link"
            :disabled="busy"
            @click="logout(true)"
          >
            退出全部登录
          </button>
        </div>
        <ul class="account-sessions">
          <li v-for="item in records" :key="item.id">
            <div>
              <strong>{{ item.current ? '当前浏览器' : '其他登录' }}</strong>
              <p>
                {{
                  item.created_at
                    ? new Date(item.created_at).toLocaleString('zh-CN')
                    : '登录时间未记录'
                }}
              </p>
            </div>
            <span v-if="item.current" class="account-current">正在使用</span
            ><button v-else class="text-link" :disabled="busy" @click="revoke(item.id)">
              退出
            </button>
          </li>
        </ul>
      </section>
      <div class="account-footer">
        <RouterLink to="/">回到 Wiki</RouterLink
        ><button class="quiet-button" :disabled="busy" @click="logout()">退出登录</button>
      </div>
    </template>
    <template v-else>
      <form @submit.prevent="submit">
        <template v-if="session.setup_needed"
          ><label for="setup-token">初始化凭证</label
          ><input id="setup-token" v-model="form.token" autocomplete="off" required
        /></template>
        <template v-if="session.setup_needed || joining"
          ><label for="nickname">昵称</label
          ><input id="nickname" v-model="form.name" maxlength="40" autocomplete="nickname" required
        /></template>
        <label for="target">邮箱地址</label
        ><input id="target" v-model="form.target" type="email" autocomplete="email" required />
        <label for="code">验证码</label>
        <div class="verification-code">
          <input
            id="code"
            v-model="form.code"
            inputmode="numeric"
            autocomplete="one-time-code"
            pattern="[0-9]{6}"
            maxlength="6"
            placeholder="6 位数字"
            required
          /><button
            type="button"
            class="quiet-button"
            :disabled="busy || remaining > 0 || !emailReady"
            @click="send"
          >
            {{ remaining > 0 ? `${remaining} 秒后重发` : '获取验证码' }}
          </button>
        </div>
        <p v-if="!emailReady" class="field-hint">邮箱服务暂不可用，请稍后再试。</p>
        <button class="button auth-submit" :disabled="busy || !challenge">
          {{
            busy
              ? '正在验证…'
              : session.setup_needed
                ? '验证并建立 Wiki'
                : joining
                  ? '验证并加入'
                  : '登录'
          }}
        </button>
      </form>
      <button v-if="joining" class="text-link auth-switch" @click="leaveInvite">
        已有账号，去登录
      </button>
      <p v-else-if="!session.setup_needed" class="field-hint">
        新成员请使用邀请链接，或联系管理员加入邮箱白名单。
      </p>
    </template>
  </section>
</template>
