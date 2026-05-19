<script lang="ts" setup>
import { ElAvatar, ElBadge, ElEmpty } from 'element-plus'
import type { Conversation } from './types'

interface Props {
  conversations: Conversation[]
  activeConversationId: number | null
  currentUserLabel: string
  connected: boolean
  loading?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
})

const emit = defineEmits<{
  select: [conversationId: number]
  logout: []
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
</style>