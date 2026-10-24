<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  api,
  catalog,
  editable,
  administrative,
  message,
  refreshSession,
  refreshTags,
  session,
} from './api'
import Icon from './components/Icon.vue'
import brandMark from './assets/brand.svg'
import Avatar from './components/Avatar.vue'
const route = useRoute(),
  router = useRouter(),
  search = ref(''),
  menu = ref(false),
  error = ref('')
const articlePage = computed(
  () => route.path.startsWith('/articles/') || route.path.startsWith('/trash/'),
)
const navigation = computed(() => catalog.tags.filter((t) => t.show_in_nav))
watch(
  () => route.fullPath,
  () => {
    menu.value = false
    search.value = String(route.query.q || '')
  },
)
watch(
  () => session.user,
  (user) => {
    if (session.ready && !user && route.path !== '/account') router.replace('/account')
  },
)
function guardComposition(event: KeyboardEvent) {
  if (event.isComposing) event.preventDefault()
}
function submitSearch() {
  router.push({ path: '/search', query: search.value.trim() ? { q: search.value.trim() } : {} })
}
async function random() {
  try {
    const v = await api<{ id: string }>('/articles/random')
    router.push('/articles/' + v.id)
  } catch (e) {
    error.value = message(e)
  }
}
async function initialise() {
  try {
    error.value = ''
    await refreshSession()
    if (session.demo && !session.user) {
      Object.assign(session, await api('/auth/demo', { method: 'POST', body: {} }))
      await refreshSession()
    }
    if (session.user) await refreshTags()
    else if (route.path !== '/account') await router.replace('/account')
  } catch (e) {
    error.value = message(e)
  }
}
async function recheck() {
  if (document.visibilityState === 'visible') {
    try {
      await refreshSession()
      if (!session.user && route.path !== '/account') await router.replace('/account')
    } catch {}
  }
}
onMounted(() => {
  initialise()
  window.addEventListener('pageshow', recheck)
  document.addEventListener('visibilitychange', recheck)
})
onUnmounted(() => {
  window.removeEventListener('pageshow', recheck)
  document.removeEventListener('visibilitychange', recheck)
})
</script>
<template>
  <a class="skip-link" href="#main">跳到正文</a>
  <div class="site-shell" :class="{ 'article-screen': articlePage, 'guest-screen': !session.user }">
    <header class="site-header">
      <RouterLink class="brand" to="/" aria-label="鸟托邦 Wiki 首页"
        ><img class="brand-mark" :src="brandMark" width="42" height="42" alt="" />
        <span>鸟托邦<span class="brand-wiki">Wiki</span></span></RouterLink
      >
      <form v-if="session.user" class="header-search" role="search" @submit.prevent="submitSearch">
        <input
          v-model="search"
          @keydown.enter="guardComposition"
          aria-label="搜索文章"
          placeholder="搜索人物、故事、群梗…"
          type="search"
        /><button aria-label="搜索"><Icon name="search" /></button>
      </form>
      <nav class="header-nav" aria-label="账号与编辑">
        <RouterLink v-if="administrative()" to="/admin">管理</RouterLink
        ><RouterLink v-if="editable()" to="/articles/new" class="write-link"
          ><Icon name="plus" :size="18" />写文章</RouterLink
        ><RouterLink to="/account" class="header-account"
          ><Avatar
            v-if="session.user"
            :name="session.user.name"
            :media-id="session.user.avatar_media_id"
            :size="24"
          /><span>{{ session.user?.name || '登录' }}</span></RouterLink
        >
      </nav>
      <button
        v-if="session.user"
        class="mobile-menu icon-button"
        aria-label="展开导航"
        :aria-expanded="menu"
        @click="menu = !menu"
      >
        <Icon :name="menu ? 'close' : 'menu'" />
      </button>
    </header>
    <div v-if="session.demo" class="demo-note">
      演示空间<span>这里的人物和故事均为虚构，可以自由试写。</span>
    </div>
    <div v-if="error" class="notice error" role="alert">
      {{ error }} <button @click="initialise">重试</button
      ><button class="icon-button" aria-label="关闭提示" @click="error = ''">
        <Icon name="close" :size="16" />
      </button>
    </div>
    <div
      class="workspace"
      :class="{ 'reading-layout': articlePage, 'single-column': !session.user, 'menu-open': menu }"
    >
      <aside
        v-if="session.user && (!articlePage || menu)"
        class="sidebar"
        :class="{ 'mobile-open': menu }"
      >
        <nav aria-label="内容导航">
          <RouterLink to="/" class="primary-nav"><Icon name="book" />最近更新</RouterLink
          ><RouterLink to="/timeline" class="primary-nav"><Icon name="clock" />时间线</RouterLink
          ><RouterLink to="/community" class="primary-nav"><Icon name="user" />用户墙</RouterLink
          ><button class="primary-nav" @click="random"><Icon name="shuffle" />随便看看</button>
          <div class="nav-section-title">常用标签</div>
          <RouterLink
            v-for="tag in navigation"
            :key="tag.id"
            :to="'/tags/' + tag.id"
            class="tag-nav"
            ><strong>{{ tag.name }}</strong
            ><small>{{ tag.description }}</small></RouterLink
          >
          <RouterLink to="/tags" class="all-tags"
            >全部标签 <Icon name="arrow" :size="15"
          /></RouterLink>
          <div v-if="editable() || administrative()" class="draft-nav-section">
            <RouterLink v-if="editable()" to="/drafts" class="primary-nav"
              ><Icon name="edit" :size="18" />我的草稿</RouterLink
            >
            <RouterLink v-if="administrative()" to="/trash" class="primary-nav"
              ><Icon name="trash" :size="18" />回收站</RouterLink
            >
          </div>
          <div class="mobile-account-links">
            <RouterLink v-if="editable()" to="/articles/new" class="primary-nav"
              ><Icon name="plus" :size="18" />写文章</RouterLink
            >
            <RouterLink to="/account" class="primary-nav"
              ><Icon name="user" :size="18" />{{ session.user?.name || '登录 Wiki' }}</RouterLink
            ><RouterLink v-if="administrative()" to="/admin" class="primary-nav"
              >管理 Wiki</RouterLink
            >
          </div>
        </nav>
      </aside>
      <main id="main" tabindex="-1">
        <div v-if="!session.ready && !error" class="loading">正在翻开 Wiki…</div>
        <RouterView v-else-if="session.user || route.path === '/account'" :key="route.path" />
      </main>
    </div>
    <footer class="site-footer"><span>鸟托邦 Wiki</span><span>记得我们说过的话。</span></footer>
  </div>
</template>
