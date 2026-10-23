<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, message, refreshSession, refreshTags, session } from '../api'
const route = useRoute(),
  router = useRouter(),
  error = ref(''),
  busy = ref(false),
  register = ref(!!route.query.invite)
const form = reactive({
  login: '',
  password: '',
  name: '',
  token: '',
  invite: String(route.query.invite || ''),
})
const mode = computed(() =>
  session.setup_needed ? 'setup' : register.value ? 'register' : 'login',
)
async function submit() {
  busy.value = true
  error.value = ''
  try {
    const body =
      mode.value === 'setup'
        ? { login: form.login, password: form.password, name: form.name, token: form.token }
        : mode.value === 'register'
          ? { login: form.login, password: form.password, name: form.name, invite: form.invite }
          : { login: form.login, password: form.password }
    Object.assign(session, await api('/auth/' + mode.value, { method: 'POST', body }))
    await refreshSession()
    await refreshTags()
    await router.push('/')
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}
async function logout() {
  try {
    await api('/auth/logout', { method: 'POST', body: {} })
    await refreshSession()
  } catch (e) {
    error.value = message(e)
  }
}
async function demo() {
  Object.assign(session, await api('/auth/demo', { method: 'POST', body: {} }))
  await refreshSession()
  await refreshTags()
  router.push('/')
}
function toggleMode() {
  register.value = !register.value
  error.value = ''
}
</script>
<template>
  <section v-if="session.user" class="account-page">
    <div class="page-heading">
      <div>
        <h1>你好，{{ session.user.name }}</h1>
        <p class="page-description">写下的每一笔，都会成为这里的一部分。</p>
      </div>
    </div>
    <dl class="account-details">
      <div>
        <dt>账号</dt>
        <dd>{{ session.user.login }}</dd>
      </div>
      <div>
        <dt>你的角色</dt>
        <dd>{{ { admin: '管理员', editor: '编辑者', viewer: '阅读者' }[session.user.role] }}</dd>
      </div>
    </dl>
    <RouterLink v-if="session.user.role !== 'viewer'" class="button" to="/drafts"
      >看看我的草稿</RouterLink
    ><button class="quiet-button logout-button" @click="logout">退出登录</button>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
  </section>
  <section v-else class="auth-page">
    <h1>
      {{
        mode === 'setup'
          ? '建立我们的 Wiki'
          : mode === 'register'
            ? '加入这个群的故事'
            : '回到我们的 Wiki'
      }}
    </h1>
    <p class="page-description">
      {{
        mode === 'setup'
          ? '创建第一个管理员，开始记录。'
          : mode === 'register'
            ? '使用群管理员给你的邀请加入。'
            : '登录以后，继续写下那些值得记住的事。'
      }}
    </p>
    <form @submit.prevent="submit">
      <p v-if="error" class="notice error" role="alert">{{ error }}</p>
      <template v-if="mode === 'setup'"
        ><label for="setup-token">一次性初始化凭证</label
        ><input id="setup-token" v-model="form.token" required autocomplete="off" />
        <p class="field-hint">凭证保存在服务器数据目录的 setup-token 文件中。</p></template
      ><template v-if="mode === 'register'"
        ><label for="invite">邀请凭证</label
        ><input id="invite" v-model="form.invite" required autocomplete="off" /></template
      ><template v-if="mode !== 'login'"
        ><label for="account-name">群里怎么称呼你</label
        ><input
          id="account-name"
          v-model="form.name"
          required
          maxlength="40"
          autocomplete="nickname" /></template
      ><label for="login">账号</label
      ><input
        id="login"
        v-model="form.login"
        required
        minlength="3"
        autocomplete="username"
      /><label for="password">密码</label
      ><input
        id="password"
        v-model="form.password"
        type="password"
        required
        :minlength="mode === 'login' ? 1 : 10"
        :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
      />
      <p v-if="mode !== 'login'" class="field-hint">
        至少 10 个字符，使用你不在其他地方重复的密码。
      </p>
      <button class="button auth-submit" :disabled="busy">
        {{
          busy
            ? '正在处理…'
            : mode === 'setup'
              ? '创建 Wiki'
              : mode === 'register'
                ? '加入 Wiki'
                : '登录'
        }}
      </button>
    </form>
    <button v-if="!session.setup_needed" class="text-link auth-switch" @click="toggleMode">
      {{ register ? '已有账号，去登录' : '有邀请凭证？加入 Wiki' }}</button
    ><button v-if="session.demo" class="quiet-button" @click="demo">进入演示空间</button>
  </section>
</template>
