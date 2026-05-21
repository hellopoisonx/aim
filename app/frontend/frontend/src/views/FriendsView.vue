<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { ElAvatar, ElButton, ElEmpty, ElTabs, ElTabPane, ElBadge } from 'element-plus'
import {
  AcceptFriend,
  CreateDirectConversation,
  GetUserById,
  ListFriendApplications,
  ListFriends,
  RejectFriend,
} from '../../wailsjs/go/main/App'
import type { FriendItem, FriendRequest } from '../components/types'

// ─── Props & Emits ────────────────────────────────────────────────────────

interface Props {
  currentUserId: number
  onlineUserIds?: Set<number>
}

const props = withDefaults(defineProps<Props>(), {
  onlineUserIds: () => new Set(),
})

const emit = defineEmits<{
  'start-conversation': [conversationId: number, friendId: number, title: string, avatar: string]
  back: []
}>()

// ─── State ────────────────────────────────────────────────────────────────

const activeTab = ref<'friends' | 'applications'>('friends')
const friends = ref<FriendItem[]>([])
const applications = ref<FriendRequest[]>([])
const loading = ref(false)
const actionLoading = ref<Set<number>>(new Set())
const creatingConversation = ref<Set<number>>(new Set())

// ─── Computed ─────────────────────────────────────────────────────────────

const pendingCount = computed(() => applications.value.length)

const onlineFriends = computed(() =>
  friends.value.filter((f) => props.onlineUserIds.has(f.friendId)),
)

const offlineFriends = computed(() =>
  friends.value.filter((f) => !props.onlineUserIds.has(f.friendId)),
)

// ─── Data loading ─────────────────────────────────────────────────────────

async function loadAll() {
  loading.value = true
  try {
    const [friendsResp, appsResp] = await Promise.all([
      ListFriends(),
      ListFriendApplications(),
    ])

    // Map FriendshipItem to FriendItem, resolving user info
    const friendItems: FriendItem[] = await Promise.all(
      (friendsResp?.friends ?? []).map(async (item: { user_id: number; friend_id: number; status: string; created_at: number; updated_at: number }) => {
        const otherId = item.user_id === props.currentUserId ? item.friend_id : item.user_id
        let email = ''
        let avatar = ''
        try {
          const userResp = await GetUserById(otherId)
          email = userResp?.user?.email ?? ''
          avatar = userResp?.user?.avatar ?? ''
        } catch {
          email = `用户 ${otherId}`
        }
        return {
          id: otherId,
          userId: item.user_id,
          friendId: item.friend_id,
          email,
          avatar,
          isOnline: props.onlineUserIds.has(otherId),
        }
      }),
    )

    friends.value = friendItems

    // Map FriendshipItem to FriendRequest, resolving user emails
    const appItems: FriendRequest[] = await Promise.all(
      (appsResp?.applications ?? [])
        .filter((item: { status: string }) => item.status === 'pending')
        .map(async (item: { user_id: number; friend_id: number; status: string; created_at: number; updated_at: number }, idx: number) => {
          let userEmail = ''
          try {
            const userResp = await GetUserById(item.user_id)
            userEmail = userResp?.user?.email ?? ''
          } catch {
            userEmail = `用户 ${item.user_id}`
          }
          return {
            id: idx,
            userId: item.user_id,
            friendId: item.friend_id,
            status: item.status,
            createdAt: item.created_at,
            updatedAt: item.updated_at,
            userEmail,
          }
        }),
    )

    applications.value = appItems
  } catch {
    // Silently handle — UI shows empty state
  } finally {
    loading.value = false
  }
}

// ─── Actions ──────────────────────────────────────────────────────────────

async function handleAccept(userId: number) {
  if (actionLoading.value.has(userId)) return
  actionLoading.value.add(userId)
  try {
    await AcceptFriend(userId)
    applications.value = applications.value.filter((a) => a.userId !== userId)
    // Reload friends to reflect the new friend
    await loadAll()
  } catch {
    // Error handled silently
  } finally {
    actionLoading.value.delete(userId)
  }
}

async function handleReject(userId: number) {
  if (actionLoading.value.has(userId)) return
  actionLoading.value.add(userId)
  try {
    await RejectFriend(userId)
    applications.value = applications.value.filter((a) => a.userId !== userId)
  } catch {
    // Error handled silently
  } finally {
    actionLoading.value.delete(userId)
  }
}

async function handleStartChat(friend: FriendItem) {
  if (creatingConversation.value.has(friend.id)) return
  creatingConversation.value.add(friend.id)
  try {
    const resp = await CreateDirectConversation(friend.id)
    emit('start-conversation', resp.conversation_id, friend.id, friend.email, friend.avatar)
  } catch {
    // Error handled silently
  } finally {
    creatingConversation.value.delete(friend.id)
  }
}

function formatEmail(email: string): string {
  const idx = email.indexOf('@')
  return idx > 0 ? email.slice(0, idx) : email
}

// ─── Lifecycle ────────────────────────────────────────────────────────────

onMounted(() => {
  loadAll()
})
</script>

<template>
  <div class="friends-view">
    <!-- Header -->
    <div class="fv-header">
      <button class="fv-back" title="返回聊天" @click="emit('back')">← 返回</button>
      <h2 class="fv-title">好友管理</h2>
    </div>

    <!-- Tabs -->
    <ElTabs v-model="activeTab" class="fv-tabs">
      <ElTabPane label="我的好友" name="friends">
        <!-- Loading -->
        <div v-if="loading" class="fv-loading">
          <div v-for="n in 4" :key="n" class="fv-skeleton-item" />
        </div>

        <!-- Empty -->
        <ElEmpty
          v-else-if="friends.length === 0"
          description="还没有好友，去搜索用户添加吧"
          :image-size="60"
          class="fv-empty"
        />

        <!-- Friends list -->
        <div v-else class="fv-list">
          <!-- Online -->
          <template v-if="onlineFriends.length > 0">
            <div class="fv-section-label">在线 — {{ onlineFriends.length }}</div>
            <div
              v-for="friend in onlineFriends"
              :key="friend.id"
              class="fv-item"
              role="button"
              tabindex="0"
              @click="handleStartChat(friend)"
              @keydown.enter="handleStartChat(friend)"
            >
              <ElBadge :value="''" class="fv-online-badge" :is-dot="true" type="success">
                <ElAvatar :src="friend.avatar" :size="40">
                  {{ formatEmail(friend.email).slice(0, 1) }}
                </ElAvatar>
              </ElBadge>
              <div class="fv-body">
                <span class="fv-name">{{ formatEmail(friend.email) }}</span>
                <span class="fv-email">{{ friend.email }}</span>
              </div>
              <ElButton
                size="small"
                type="primary"
                :loading="creatingConversation.has(friend.id)"
                plain
                @click.stop="handleStartChat(friend)"
              >
                发消息
              </ElButton>
            </div>
          </template>

          <!-- Offline -->
          <template v-if="offlineFriends.length > 0">
            <div class="fv-section-label">离线 — {{ offlineFriends.length }}</div>
            <div
              v-for="friend in offlineFriends"
              :key="friend.id"
              class="fv-item fv-item--offline"
              role="button"
              tabindex="0"
              @click="handleStartChat(friend)"
              @keydown.enter="handleStartChat(friend)"
            >
              <ElAvatar :src="friend.avatar" :size="40">
                {{ formatEmail(friend.email).slice(0, 1) }}
              </ElAvatar>
              <div class="fv-body">
                <span class="fv-name">{{ formatEmail(friend.email) }}</span>
                <span class="fv-email">{{ friend.email }}</span>
              </div>
              <ElButton
                size="small"
                type="primary"
                :loading="creatingConversation.has(friend.id)"
                plain
                @click.stop="handleStartChat(friend)"
              >
                发消息
              </ElButton>
            </div>
          </template>
        </div>
      </ElTabPane>

      <ElTabPane name="applications">
        <template #label>
          <span>好友申请</span>
          <ElBadge
            v-if="pendingCount > 0"
            :value="pendingCount"
            :max="99"
            class="fv-tab-badge"
          />
        </template>

        <!-- Loading -->
        <div v-if="loading" class="fv-loading">
          <div v-for="n in 3" :key="n" class="fv-skeleton-item" />
        </div>

        <!-- Empty -->
        <ElEmpty
          v-else-if="applications.length === 0"
          description="没有待处理的好友申请"
          :image-size="60"
          class="fv-empty"
        />

        <!-- Application list -->
        <div v-else class="fv-list">
          <div
            v-for="app in applications"
            :key="app.userId"
            class="fv-item fv-app-item"
          >
            <ElAvatar :size="40">
              {{ (app.userEmail ?? '?').slice(0, 1) }}
            </ElAvatar>
            <div class="fv-body">
              <span class="fv-name">{{ formatEmail(app.userEmail ?? '') }}</span>
              <span class="fv-email">{{ app.userEmail ?? '' }}</span>
            </div>
            <div class="fv-app-actions">
              <ElButton
                size="small"
                type="primary"
                :loading="actionLoading.has(app.userId)"
                @click="handleAccept(app.userId)"
              >
                接受
              </ElButton>
              <ElButton
                size="small"
                type="danger"
                :loading="actionLoading.has(app.userId)"
                plain
                @click="handleReject(app.userId)"
              >
                拒绝
              </ElButton>
            </div>
          </div>
        </div>
      </ElTabPane>
    </ElTabs>
  </div>
</template>

<style scoped>
.friends-view {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--aim-surface);
}

.fv-header {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-4);
  border-bottom: 1px solid var(--aim-border);
  flex-shrink: 0;
}

.fv-back {
  background: none;
  border: 1px solid var(--aim-border);
  color: var(--aim-text-muted);
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
  transition: border-color 0.15s, color 0.15s;
}

.fv-back:hover {
  border-color: var(--aim-primary);
  color: var(--aim-primary);
}

.fv-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--aim-text);
  margin: 0;
}

.fv-tabs {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.fv-tabs :deep(.el-tabs__content) {
  flex: 1;
  overflow-y: auto;
}

.fv-tab-badge {
  margin-left: 6px;
}

.fv-loading {
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.fv-skeleton-item {
  height: 52px;
  background: var(--aim-surface-2);
  border-radius: 6px;
  animation: skeleton-pulse 1.4s ease-in-out infinite;
}

@keyframes skeleton-pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.fv-empty {
  padding: var(--space-8) 0;
}

.fv-list {
  display: flex;
  flex-direction: column;
  padding: var(--space-2) 0;
}

.fv-section-label {
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--aim-text-muted);
  padding: var(--space-2) var(--space-4);
  margin-top: var(--space-1);
}

.fv-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  cursor: pointer;
  transition: background 0.15s;
}

.fv-item:hover {
  background: var(--aim-surface-2);
}

.fv-item--offline {
  opacity: 0.65;
}

.fv-online-badge :deep(.el-badge__content) {
  background-color: var(--aim-primary);
}

.fv-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.fv-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--aim-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fv-email {
  font-size: 11px;
  color: var(--aim-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fv-app-item {
  cursor: default;
}

.fv-app-actions {
  display: flex;
  gap: var(--space-2);
  flex-shrink: 0;
}
</style>
