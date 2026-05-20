<script lang="ts" setup>
import { ElAvatar, ElBadge, ElEmpty, ElInput } from 'element-plus'
import { ref, watch } from 'vue'
import type { Conversation, SearchUserItem } from './types'

interface Props {
  conversations: Conversation[]
  activeConversationId: number | null
  currentUserLabel: string
  connected: boolean
  loading?: boolean
  historyLoading?: boolean
  searchKeyword?: string
  searchResults?: SearchUserItem[]
  searchLoading?: boolean
  createLoading?: boolean
  creatingUserId?: number | null
  searchError?: string
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  historyLoading: false,
  searchKeyword: '',
  searchResults: () => [],
  searchLoading: false,
  createLoading: false,
  creatingUserId: null,
  searchError: '',
})

const emit = defineEmits<{
  select: [conversationId: number]
  logout: []
  'search-user': [keyword: string]
  'start-direct': [userId: number]
}>()

function formatTime(isoString: string | undefined): string {
  if (!isoString) return ''
  const date = new Date(isoString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  if (diffSec < 60) return '刚刚'
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}分钟前`
  if (diffSec < 86400) return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  return date.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

const localSearchKeyword = ref(props.searchKeyword)

watch(() => props.searchKeyword, (value) => {
  localSearchKeyword.value = value
})

let searchTimer: ReturnType<typeof setTimeout> | null = null

function onSearchInput(value: string) {
  localSearchKeyword.value = value
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    emit('search-user', value)
  }, 300)
}

function clearSearch() {
  localSearchKeyword.value = ''
  emit('search-user', '')
}

function getDisplayName(user: SearchUserItem): string {
  const idx = user.email.indexOf('@')
  return idx > 0 ? user.email.substring(0, idx) : user.email
}
</script>

<template>
  <aside class="conversation-list">
    <!-- Header -->
    <div class="cl-header">
      <div class="cl-identity">
        <span class="cl-label">{{ currentUserLabel }}</span>
        <span class="cl-status" :class="connected ? 'status-online' : 'status-offline'">
          {{ connected ? '在线' : '离线' }}
        </span>
      </div>
      <button class="cl-logout" title="退出登录" @click="emit('logout')">退出</button>
    </div>

    <!-- Search -->
    <div class="cl-search">
      <ElInput
        :model-value="localSearchKeyword"
        placeholder="搜索用户..."
        clearable
        size="small"
        @input="onSearchInput"
        @clear="clearSearch"
      />
      <div v-if="searchLoading" class="cl-search-tip">搜索中...</div>
      <div v-else-if="searchError" class="cl-search-tip cl-search-error">{{ searchError }}</div>
      <div v-else-if="searchResults.length > 0" class="cl-search-results">
        <div
          v-for="user in searchResults"
          :key="user.id"
          class="cl-search-item"
          role="button"
          tabindex="0"
          :class="{ 'cl-search-item--loading': createLoading && creatingUserId === user.id }"
          @click="!createLoading && emit('start-direct', user.id)"
          @keydown.enter="!createLoading && emit('start-direct', user.id)"
        >
          <ElAvatar :src="user.avatar" :size="28" class="cl-search-avatar">
            {{ getDisplayName(user).slice(0, 1) }}
          </ElAvatar>
          <span class="cl-search-name">{{ getDisplayName(user) }}</span>
          <span v-if="createLoading && creatingUserId === user.id" class="cl-search-action">创建中</span>
        </div>
      </div>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading" class="cl-loading">
      <div v-for="n in 4" :key="n" class="cl-skeleton-item" />
    </div>

    <!-- Empty state -->
    <ElEmpty
      v-else-if="conversations.length === 0"
      description="暂无会话"
      :image-size="80"
      class="cl-empty"
    />

    <!-- Conversation items -->
    <ul v-else class="cl-items">
      <li
        v-for="conv in conversations"
        :key="conv.id"
        class="cl-item"
        :class="{ 'cl-item--active': conv.id === activeConversationId }"
        role="button"
        tabindex="0"
        @click="emit('select', conv.id)"
        @keydown.enter="emit('select', conv.id)"
      >
        <!-- Avatar with online badge -->
        <ElBadge :value="conv.unreadCount" :hidden="conv.unreadCount === 0" :max="99" class="cl-badge">
          <ElAvatar :src="conv.avatar" :size="42" class="cl-avatar">
            {{ conv.title.slice(0, 1) }}
          </ElAvatar>
        </ElBadge>

        <!-- Text content -->
        <div class="cl-body">
          <div class="cl-title-row">
            <span class="cl-title">{{ conv.title }}</span>
            <span v-if="conv.lastMessageAt" class="cl-time">{{ formatTime(conv.lastMessageAt) }}</span>
          </div>
          <p class="cl-preview">{{ conv.lastMessage || '暂无消息' }}</p>
        </div>
      </li>
    </ul>
  </aside>
</template>

<style scoped>
.conversation-list {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--aim-surface);
}

/* Header */
.cl-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4);
  border-bottom: 1px solid var(--aim-border);
  flex-shrink: 0;
}

.cl-identity {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.cl-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--aim-text);
}

.cl-status {
  font-size: 10px;
  font-weight: 500;
}

.status-online { color: var(--aim-primary); }
.status-offline { color: var(--aim-text-muted); }

.cl-logout {
  background: none;
  border: 1px solid var(--aim-border);
  color: var(--aim-text-muted);
  font-size: 10px;
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
  transition: border-color 0.15s, color 0.15s;
}

.cl-logout:hover {
  border-color: var(--aim-danger);
  color: var(--aim-danger);
}

/* Loading skeletons */
.cl-loading {
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.cl-skeleton-item {
  height: 56px;
  background: var(--aim-surface-2);
  border-radius: 6px;
  animation: skeleton-pulse 1.4s ease-in-out infinite;
}

@keyframes skeleton-pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

/* Empty */
.cl-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* Items */
.cl-items {
  list-style: none;
  overflow-y: auto;
  flex: 1;
  padding: var(--space-2) 0;
}

.cl-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  cursor: pointer;
  transition: background 0.15s;
  border-left: 2px solid transparent;
}

.cl-item:hover {
  background: var(--aim-surface-2);
}

.cl-item--active {
  background: rgba(0, 212, 170, 0.08);
  border-left-color: var(--aim-primary);
}

.cl-badge {
  flex-shrink: 0;
}

.cl-avatar {
  background: var(--aim-surface-2);
  color: var(--aim-primary);
  font-weight: 700;
  font-size: 15px;
  border: 1px solid var(--aim-border);
}

.cl-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.cl-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.cl-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--aim-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.cl-time {
  font-size: 10px;
  color: var(--aim-text-muted);
  flex-shrink: 0;
}

.cl-preview {
  font-size: 11px;
  color: var(--aim-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin: 0;
}

/* Search */
.cl-search {
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--aim-border);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.cl-search-tip {
  font-size: 10px;
  color: var(--aim-text-muted);
  padding: 2px 0;
}

.cl-search-error {
  color: var(--aim-danger);
}

.cl-search-results {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 200px;
  overflow-y: auto;
}

.cl-search-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 6px var(--space-2);
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s;
}

.cl-search-item:hover {
  background: var(--aim-surface-2);
}

.cl-search-item--loading {
  opacity: 0.72;
  cursor: progress;
}

.cl-search-action {
  margin-left: auto;
  font-size: 10px;
  color: var(--aim-text-muted);
}

.cl-search-avatar {
  flex-shrink: 0;
  background: var(--aim-surface-2);
  color: var(--aim-primary);
  font-weight: 700;
  font-size: 12px;
  border: 1px solid var(--aim-border);
}

.cl-search-name {
  font-size: 12px;
  color: var(--aim-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
