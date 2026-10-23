<script lang="ts">
import { defineComponent, h, ref, type PropType, type VNode } from 'vue'
import { RouterLink } from 'vue-router'
import type { DocNode } from '../types'
export default defineComponent({
  props: { document: { type: Object as PropType<DocNode>, required: true } },
  setup(props) {
    const lightbox = ref(''),
      broken = ref<Record<string, boolean>>({})
    function render(n: DocNode): VNode | string {
      if (n.type === 'text') {
        let node: VNode | string = n.text || ''
        for (const mark of n.marks || []) {
          const tags: Record<string, string> = {
            bold: 'strong',
            italic: 'em',
            strike: 's',
            code: 'code',
            underline: 'u',
          }
          if (mark.type === 'link') {
            const href = String(mark.attrs?.href || '')
            const child: VNode | string = node
            node = href.startsWith('/articles/')
              ? h(RouterLink, { to: href }, () => child)
              : h('a', { href, target: '_blank', rel: 'noopener noreferrer' }, [child])
          } else if (tags[mark.type]) node = h(tags[mark.type], [node])
        }
        return node
      }
      const children = (n.content || []).map(render)
      if (n.type === 'image') {
        const url = '/media/' + n.attrs?.mediaId
        return h('figure', { class: 'article-image' }, [
          h(
            'button',
            {
              class: 'image-open',
              onClick: () => (lightbox.value = url),
              'aria-label': '放大图片',
            },
            [h('img', { src: url, alt: n.attrs?.alt || '', loading: 'lazy' })],
          ),
          n.attrs?.alt ? h('figcaption', String(n.attrs.alt)) : null,
        ])
      }
      if (n.type === 'video') {
        const id = String(n.attrs?.mediaId)
        return h('figure', { class: 'article-video' }, [
          broken.value[id]
            ? h('p', '当前浏览器无法播放这个视频，可以下载后查看。')
            : h('video', {
                src: '/media/' + id,
                controls: true,
                preload: 'metadata',
                playsinline: true,
                onError: () => (broken.value[id] = true),
              }),
          h('figcaption', [
            String(n.attrs?.title || '视频'),
            ' ',
            h('a', { href: '/media/' + id, target: '_blank', rel: 'noopener' }, '下载原视频'),
          ]),
        ])
      }
      if (n.type === 'doc') return h('div', { class: 'article-prose' }, children)
      const tag: Record<string, string> = {
        paragraph: 'p',
        heading: 'h' + Math.min(3, Number(n.attrs?.level || 2)),
        bulletList: 'ul',
        orderedList: 'ol',
        listItem: 'li',
        blockquote: 'blockquote',
        hardBreak: 'br',
        horizontalRule: 'hr',
        codeBlock: 'pre',
      }
      return h(
        tag[n.type] || 'p',
        {
          id: n.attrs?.id,
          start: n.type === 'orderedList' ? Number(n.attrs?.start || 1) : undefined,
        },
        children,
      )
    }
    return () =>
      h('div', [
        render(props.document),
        lightbox.value
          ? h(
              'div',
              {
                class: 'lightbox',
                role: 'dialog',
                'aria-modal': 'true',
                'aria-label': '原图',
                onClick: () => (lightbox.value = ''),
              },
              [
                h(
                  'button',
                  {
                    class: 'lightbox-close',
                    'aria-label': '关闭原图',
                    onClick: () => (lightbox.value = ''),
                  },
                  '关闭 ×',
                ),
                h('img', { src: lightbox.value, alt: '放大图片' }),
              ],
            )
          : null,
      ])
  },
})
</script>
