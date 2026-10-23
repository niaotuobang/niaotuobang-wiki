import type { Article } from './types'
interface ListState {
  items: Article[]
  total: number
  cursor: string
}
const lists = new Map<string, ListState>()
export function readListState(key: string) {
  return lists.get(key)
}
export function saveListState(key: string, state: ListState) {
  lists.delete(key)
  lists.set(key, state)
  while (lists.size > 3) lists.delete(lists.keys().next().value!)
}
export function clearListState() {
  lists.clear()
}
