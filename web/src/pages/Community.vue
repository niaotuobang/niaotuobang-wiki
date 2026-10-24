<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, message } from '../api'
import Avatar from '../components/Avatar.vue'

interface Member {
  id: string
  name: string
  avatar_media_id?: string
  article_count: number
}
const members = ref<Member[]>([])
const loading = ref(true)
const error = ref('')
async function load() {
  loading.value = true
  error.value = ''
  try {
    members.value = (await api<{ items: Member[] }>('/community')).items
  } catch (e) {
    error.value = message(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <div class="page-heading community-heading">
    <div>
      <h1>用户墙</h1>
      <p class="page-description">故事背后，是我们。最近更新过文章的群友排在前面。</p>
    </div>
    <span v-if="!loading && !error" class="community-total">{{ members.length }} 位群友</span>
  </div>
  <p v-if="error" class="notice error" role="alert">
    {{ error }}<button @click="load">重试</button>
  </p>
  <p v-else-if="loading" class="loading" role="status">正在找齐群友…</p>
  <div v-else-if="!members.length" class="empty-state">
    <h2>还没有群友</h2>
    <p>加入之后，就会在这里相见。</p>
  </div>
  <template v-else>
    <ul class="community-wall" aria-label="群友，按最近更新文章时间排序">
      <li v-for="member in members" :key="member.id" class="community-member">
        <Avatar :name="member.name" :media-id="member.avatar_media_id" :size="64" />
        <h2>{{ member.name }}</h2>
        <p>
          参与 <span>{{ member.article_count }}</span> 篇文章
        </p>
      </li>
    </ul>
    <p class="community-note">新建、修改都算参与，同一篇只计一次。</p>
  </template>
</template>

<style scoped>
.community-total {
  flex-shrink: 0;
  color: var(--muted);
  font-size: 14px;
}
.community-wall {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px 24px;
  margin: 32px 0 0;
  padding: 0;
  list-style: none;
}
.community-member {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-width: 0;
  padding: 28px 12px;
  text-align: center;
}
.community-member h2 {
  flex: 1;
  max-width: 100%;
  margin: 18px 0 6px;
  overflow-wrap: anywhere;
  font-size: 17px;
  font-weight: 600;
  line-height: 1.5;
}
.community-member p {
  margin: 0;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.6;
}
.community-member p span {
  color: var(--text);
  font-variant-numeric: tabular-nums;
}
.community-note {
  margin: 32px 0 0;
  padding-top: 20px;
  border-top: 1px solid var(--line);
  color: var(--muted);
  font-size: 12px;
  line-height: 1.7;
}
@media (max-width: 600px) {
  .community-heading {
    align-items: flex-start;
  }
  .community-wall {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px 12px;
    margin-top: 16px;
  }
  .community-member {
    padding: 24px 8px;
  }
}
</style>
