import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import './style.css'
import { session } from './api'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: () => import('./pages/ArticleList.vue') },
    { path: '/tags/:id', component: () => import('./pages/ArticleList.vue') },
    { path: '/search', component: () => import('./pages/ArticleList.vue') },
    { path: '/timeline', component: () => import('./pages/ArticleList.vue') },
    { path: '/tags', component: () => import('./pages/Tags.vue') },
    { path: '/articles/new', component: () => import('./pages/Editor.vue') },
    { path: '/articles/:id/edit', component: () => import('./pages/Editor.vue') },
    { path: '/articles/:id/history', component: () => import('./pages/History.vue') },
    { path: '/articles/:id', component: () => import('./pages/Article.vue') },
    { path: '/drafts', component: () => import('./pages/Drafts.vue') },
    { path: '/account', component: () => import('./pages/Account.vue') },
    { path: '/admin', component: () => import('./pages/Admin.vue') },
    { path: '/:pathMatch(.*)*', component: () => import('./pages/NotFound.vue') },
  ],
  scrollBehavior(to, from, saved) {
    if (saved) return saved
    if (to.path === from.path) return
    return { top: 0 }
  },
})
// Older Matterwiki links use a hash route, invisible to the HTTP server.
const legacy = window.location.hash.match(/^#\/?article\/(\d+)/)
if (legacy) window.location.replace('/articles/legacy-' + legacy[1])
router.afterEach(() => {
  document.title = '鸟托邦 Wiki'
})
router.beforeEach((to) => {
  if (session.ready && !session.user && !session.public_read && to.path !== '/account')
    return { path: '/account' }
})
createApp(App).use(router).mount('#app')
