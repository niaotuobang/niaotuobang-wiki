export interface Tag {
  id: string
  name: string
  description: string
  show_in_nav: boolean
  nav_order: number
  count: number
}
export interface Mark {
  type: string
  attrs?: Record<string, unknown>
}
export interface DocNode {
  type: string
  text?: string
  attrs?: Record<string, unknown>
  content?: DocNode[]
  marks?: Mark[]
}
export interface Article {
  id: string
  title: string
  document: DocNode
  schema_version: number
  plain_text: string
  tags: Tag[]
  tags_unknown?: boolean
  legacy_archived_at?: string
  media_kinds: string[]
  status: string
  revision: number
  occurred_at: string
  date_precision: string
  author_id: string
  author_name: string
  updated_by: string
  updated_by_name: string
  updated_by_avatar?: string
  created_at: string
  updated_at: string
  change_summary: string
  deleted_at?: string
}
export interface ArticleWrite {
  title: string
  document: DocNode
  tag_ids: string[]
  occurred_at: string
  date_precision: string
  change_summary: string
  expected_revision: number
  status: string
}
export interface User {
  avatar_media_id?: string
  id: string
  name: string
  login: string
  role: 'system_admin' | 'admin' | 'user'
  can_edit: boolean
}
export interface Page<T> {
  items: T[]
  total: number
  next_cursor: string
}
export interface Media {
  id: string
  name: string
  content_type: string
  size: number
  state: string
}
