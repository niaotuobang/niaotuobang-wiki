import { reactive } from 'vue'
import type { Tag, User } from './types'
import { clearListState } from './listState'
export const session = reactive({
  user: null as User | null,
  csrf_token: '',
  demo: false,
  setup_needed: false,
  public_read: false,
  ready: false,
})
export const catalog = reactive({ tags: [] as Tag[] })
export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message)
  }
}
export async function api<T = any>(
  path: string,
  options: { method?: string; body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  const response = await fetch('/api/v1' + path, {
    method: options.method || 'GET',
    credentials: 'same-origin',
    signal: options.signal,
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': session.csrf_token },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  })
  const data = await response.json()
  if (response.status === 401 && !path.startsWith('/auth/')) {
    clearPrivateState()
  }
  if (!response.ok)
    throw new ApiError(
      data.error?.code || 'REQUEST_FAILED',
      data.error?.message || '请求失败，请重试',
      response.status,
    )
  return data
}
export async function refreshSession() {
  const previous = session.user?.id
  Object.assign(session, await api('/me'))
  if (previous !== session.user?.id) {
    clearListState()
    catalog.tags = []
  }
  if (!session.user && !session.public_read) catalog.tags = []
  session.ready = true
}
export async function refreshTags() {
  catalog.tags = (await api<{ items: Tag[] }>('/tags')).items
}
export function message(error: unknown) {
  return error instanceof Error ? error.message : '操作失败，请重试'
}
export function dateLabel(value: string, precision?: string) {
  if (!value) return '未填写发生时间'
  if (precision === 'year') return value + ' 年'
  if (precision === 'month') {
    const [y, m] = value.split('-')
    return `${y} 年 ${Number(m)} 月`
  }
  if (precision === 'day') {
    const [y, m, d] = value.split('-')
    return `${y} 年 ${Number(m)} 月 ${Number(d)} 日`
  }
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(new Date(value))
}
export function editable() {
  return !!session.user && (session.user.role !== 'user' || session.user.can_edit)
}

export function administrative() {
  return session.user?.role === 'admin' || session.user?.role === 'system_admin'
}
export function clearPrivateState() {
  session.user = null
  session.csrf_token = ''
  session.public_read = false
  catalog.tags = []
  clearListState()
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const k = localStorage.key(i)
    if (k?.startsWith('wiki-draft:')) localStorage.removeItem(k)
  }
}
