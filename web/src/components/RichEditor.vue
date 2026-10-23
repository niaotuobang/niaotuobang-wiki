<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import { Extension, Node, mergeAttributes } from '@tiptap/core'
import { Plugin } from '@tiptap/pm/state'
import StarterKit from '@tiptap/starter-kit'
import Image from '@tiptap/extension-image'
import Placeholder from '@tiptap/extension-placeholder'
import type { DocNode } from '../types'
const props = defineProps<{ modelValue: DocNode }>()
const emit = defineEmits<{ change: [DocNode]; files: [File[]] }>()
const nodeTypesWithIDs = [
  'paragraph',
  'heading',
  'blockquote',
  'listItem',
  'bulletList',
  'orderedList',
  'codeBlock',
  'image',
  'video',
  'horizontalRule',
  'hardBreak',
]
const ids = Extension.create({
  name: 'blockIDs',
  addGlobalAttributes() {
    return [
      {
        types: nodeTypesWithIDs,
        attributes: { id: { default: null } },
      },
    ]
  },
  addProseMirrorPlugins() {
    return [
      new Plugin({
        appendTransaction(transactions, _old, state) {
          if (!transactions.some((t) => t.docChanged)) return
          const tr = state.tr
          const seen = new Set<string>()
          state.doc.descendants((n, pos) => {
            if (!nodeTypesWithIDs.includes(n.type.name)) return
            let id = n.attrs.id as string | null
            if (!id || seen.has(id)) {
              id = crypto.randomUUID()
              tr.setNodeMarkup(pos, undefined, { ...n.attrs, id })
            }
            seen.add(id)
          })
          return tr.docChanged ? tr : undefined
        },
      }),
    ]
  },
})
function parseMediaID(element: HTMLElement) {
  return (
    element.getAttribute('data-media-id') ||
    element.getAttribute('mediaid') ||
    element.getAttribute('src')?.match(/^\/media\/([A-Za-z0-9_-]+)$/)?.[1] ||
    null
  )
}
const mediaImage = Image.extend({
  addAttributes() {
    return { ...this.parent?.(), mediaId: { default: null, parseHTML: parseMediaID } }
  },
  renderHTML({ node, HTMLAttributes }) {
    return [
      'img',
      mergeAttributes(HTMLAttributes, {
        src: '/media/' + node.attrs.mediaId,
        'data-media-id': node.attrs.mediaId,
      }),
    ]
  },
})
const video = Node.create({
  name: 'video',
  group: 'block',
  atom: true,
  addAttributes() {
    return { mediaId: { default: null, parseHTML: parseMediaID }, title: { default: '' } }
  },
  parseHTML() {
    return [{ tag: 'video' }]
  },
  renderHTML({ node }) {
    return [
      'video',
      {
        src: '/media/' + node.attrs.mediaId,
        controls: 'true',
        preload: 'metadata',
        title: node.attrs.title,
      },
    ]
  },
})
const editor = useEditor({
  content: props.modelValue as any,
  extensions: [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      link: { openOnClick: false, autolink: false },
    }),
    mediaImage,
    video,
    ids,
    Placeholder.configure({ placeholder: '把记得的，慢慢写下来…' }),
  ],
  editorProps: {
    attributes: { 'aria-label': '文章正文', role: 'textbox', 'aria-multiline': 'true' },
    handlePaste(_view, event) {
      const files = Array.from(event.clipboardData?.files || [])
      if (files.length) {
        event.preventDefault()
        emit('files', files)
        return true
      }
      return false
    },
    handleDrop(_view, event) {
      const files = Array.from(event.dataTransfer?.files || [])
      if (files.length) {
        event.preventDefault()
        emit('files', files)
        return true
      }
      return false
    },
  },
  onUpdate: ({ editor }) => emit('change', editor.getJSON() as DocNode),
})
watch(
  () => props.modelValue,
  (value) => {
    if (editor.value && JSON.stringify(editor.value.getJSON()) !== JSON.stringify(value))
      editor.value.commands.setContent(value as any, { emitUpdate: false })
  },
)
function command(name: string) {
  const e = editor.value
  if (!e) return
  const c = e.chain().focus()
  switch (name) {
    case 'bold':
      c.toggleBold().run()
      break
    case 'italic':
      c.toggleItalic().run()
      break
    case 'heading':
      c.toggleHeading({ level: 2 }).run()
      break
    case 'list':
      c.toggleBulletList().run()
      break
    case 'quote':
      c.toggleBlockquote().run()
      break
    case 'undo':
      c.undo().run()
      break
    case 'redo':
      c.redo().run()
      break
  }
}
function insertMedia(id: string, isVideo: boolean, name: string) {
  editor.value
    ?.chain()
    .focus()
    .insertContent({
      type: isVideo ? 'video' : 'image',
      attrs: {
        mediaId: id,
        alt: isVideo ? undefined : name,
        title: isVideo ? name : undefined,
        src: isVideo ? undefined : '/media/' + id,
      },
    })
    .run()
}
function link(href: string, title?: string) {
  const e = editor.value
  if (!e) return
  if (e.state.selection.empty && title)
    e.chain()
      .focus()
      .insertContent({ type: 'text', text: title, marks: [{ type: 'link', attrs: { href } }] })
      .run()
  else e.chain().focus().extendMarkRange('link').setLink({ href }).run()
}
defineExpose({ command, insertMedia, link, active: (name: string) => editor.value?.isActive(name) })
onBeforeUnmount(() => editor.value?.destroy())
</script>
<template><EditorContent :editor="editor" class="rich-editor" /></template>
