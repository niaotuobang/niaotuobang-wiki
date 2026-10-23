<script setup lang="ts">
withDefaults(defineProps<{ name: string; size?: number }>(), { size: 20 })
const paths: Record<string, string> = {
  search: 'm21 21-4.3-4.3 M19 11a8 8 0 1 1-16 0 8 8 0 0 1 16 0',
  plus: 'M12 5v14M5 12h14',
  menu: 'M4 6h16M4 12h16M4 18h16',
  close: 'm6 6 12 12M6 18 18 6',
  back: 'm12 5-7 7 7 7M5 12h15',
  clock: 'M12 8v4l3 2M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
  edit: 'm15 5 4 4M4 20l4-1L20 7a2.8 2.8 0 0 0-4-4L4 15z',
  tag: 'M20 13 11 22 2 13V3h10l8 8zM7 7h.01',
  arrow: 'M5 12h14m-6-6 6 6-6 6',
  image: 'M4 3h16v18H4zM4 16l5-5 4 4 3-3 4 4M9 7h.01',
  video: 'M3 5h13v14H3zM16 10l6-4v12l-6-4',
  link: 'm10 13 4-4M8 16l-2 2a4 4 0 0 1-6-6l5-5a4 4 0 0 1 6 0M16 8l2-2a4 4 0 0 1 6 6l-5 5a4 4 0 0 1-6 0',
  check: 'm5 12 4 4L19 6',
  chevron: 'm8 4 8 8-8 8',
  book: 'M3 3h7l2 2 2-2h7v17h-7l-2 2-2-2H3zM12 5v17',
  user: 'M20 21v-2a6 6 0 0 0-6-6h-4a6 6 0 0 0-6 6v2M16 5a4 4 0 1 1-8 0 4 4 0 0 1 8 0',
  shuffle: 'M3 5h3l12 14h3M17 15l4 4-4 3M3 19h3l4-5M14 9l4-4h3m-4-3 4 3-4 4',
  save: 'M5 3h12l4 4v14H3V3h2M7 3v6h10V3M7 21v-8h10v8',
  trash: 'M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7',
}
</script>
<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.6"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="paths[name] || paths.book" />
  </svg>
</template>
