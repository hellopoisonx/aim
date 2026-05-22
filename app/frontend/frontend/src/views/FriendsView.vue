<script lang="ts" setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ElAvatar, ElButton, ElCheckbox, ElCheckboxGroup, ElEmpty, ElInput, ElMessage, ElTabs, ElTabPane, ElBadge } from 'element-plus'
import {
  AcceptFriend,
  CreateDirectConversation,
  CreateGroup,
  GetUserById,
  ListFriendApplications,
  ListFriends,
  RejectFriend,
} from '../../wailsjs/go/main/App'
import type { FriendItem, FriendRequest } from '../components/types'

// ─── Props & Emits ────────────────────────────────────────────────────────

type FriendsTab = 'friends' | 'applications' | 'create-group'

interface Props {
  currentUserId: string
  onlineUserIds?: Set<string>
  initialTab?: FriendsTab
}

const props = withDefaults(defineProps<Props>(), {
  onlineUserIds: () => new Set(),
  initialTab: 'friends',
})

const emit = defineEmits<{
  'start-conversation': [conversationId: string, friendId: string, title: string, avatar: string]
  'start-group': [conversationId: string, title: string, avatar: string, memberIds: string[]]
  'applications-updated': []
  back: []
}>()

// ─── State ────────────────────────────────────────────────────────────────

const activeTab = ref<FriendsTab>(props.initialTab)
const friends = ref<FriendItem[]>([])
const applications = ref<FriendRequest[]>([])
const loading = ref(false)
const actionLoading = ref<Set<string>>(new Set())
const creatingConversation = ref<Set<string>>(new Set())
const selectedGroupMemberIds = ref<string[]>([])
const groupName = ref('')
const creatingGroup = ref(false)

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
      (friendsResp?.friends ?? []).map(async (item: { user_id: string; friend_id: string; status: string; created_at: number; updated_at: number }) => {
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
        .map(async (item: { user_id: string; friend_id: string; status: string; created_at: number; updated_at: number }) => {
          let userEmail = ''
          try {
            const userResp = await GetUserById(item.user_id)
            userEmail = userResp?.user?.email ?? ''
          } catch {
            userEmail = `用户 ${item.user_id}`
          }
          return {
            id: item.user_id,
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
    emit('applications-updated')
  } catch {
    // Silently handle — UI shows empty state
  } finally {
    loading.value = false
  }
}

watch(
  () => props.initialTab,
  (tab) => {
    activeTab.value = tab
  },
)

// ─── Actions ──────────────────────────────────────────────────────────────

async function handleAccept(id: string) {
  if (actionLoading.value.has(id)) return
  actionLoading.value.add(id)
  try {
    await AcceptFriend(id)
    applications.value = applications.value.filter((a) => a.id !== id)
    emit('applications-updated')
    await loadAll()
  } catch (err) {
    const msg = err instanceof Error ? err.message : '接受好友申请失败'
    ElMessage.error(msg)
  } finally {
    actionLoading.value.delete(id)
  }
}

async function handleReject(id: string) {
  if (actionLoading.value.has(id)) return
  actionLoading.value.add(id)
  try {
    await RejectFriend(id)
    applications.value = applications.value.filter((a) => a.id !== id)
    emit('applications-updated')
  } catch (err) {
    const msg = err instanceof Error ? err.message : '拒绝好友申请失败'
    ElMessage.error(msg)
  } finally {
    actionLoading.value.delete(id)
  }
}

async function handleStartChat(friend: FriendItem) {
  if (creatingConversation.value.has(friend.id)) return
  creatingConversation.value.add(friend.id)
  try {
    const resp = await CreateDirectConversation(friend.id)
    emit('start-conversation', resp.conversation_id, friend.id, friend.email, friend.avatar)
  } catch (err) {
    const msg = err instanceof Error ? err.message : '创建会话失败'
    ElMessage.error(msg)
  } finally {
    creatingConversation.value.delete(friend.id)
  }
}

async function handleCreateGroup() {
  if (selectedGroupMemberIds.value.length === 0) {
    ElMessage.warning('请至少选择一位好友')
    return
  }
  const name = groupName.value.trim() || '未命名群聊'
  creatingGroup.value = true
  try {
    const resp = await CreateGroup({
      member_ids: selectedGroupMemberIds.value,
      name,
      avatar: '',
    })
    emit(
      'start-group',
      resp.conversation_id,
      resp.name || name,
      resp.avatar ?? '',
      resp.member_ids ?? selectedGroupMemberIds.value,
    )
    selectedGroupMemberIds.value = []
    groupName.value = ''
    activeTab.value = 'friends'
  } catch (err) {
    const msg = err instanceof Error ? err.message : '创建群聊失败'
    ElMessage.error(msg)
  } finally {
    creatingGroup.value = false
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
            <div class="fv-section-label fv-online-text">在线 — {{ onlineFriends.length }}</div>
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
                <span class="fv-name fv-online-text">{{ formatEmail(friend.email) }}</span>
                <span class="fv-email fv-online-text">{{ friend.email }}</span>
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

      <ElTabPane label="创建群聊" name="create-group">
        <div class="fv-create-group">
          <label class="fv-create-label">群名称</label>
          <ElInput v-model="groupName" placeholder="输入群名称（可选）" size="small" />

          <label class="fv-create-label">选择成员</label>
          <ElEmpty
            v-if="friends.length === 0"
            description="还没有好友，无法创建群聊"
            :image-size="60"
            class="fv-empty"
          />
          <ElCheckboxGroup v-else v-model="selectedGroupMemberIds" class="fv-group-select">
            <ElCheckbox
              v-for="friend in friends"
              :key="friend.id"
              :value="friend.id"
              class="fv-group-item"
            >
              <div class="fv-group-item-row">
                <ElAvatar :src="friend.avatar" :size="28">
                  {{ formatEmail(friend.email).slice(0, 1) }}
                </ElAvatar>
                <span>{{ formatEmail(friend.email) }}</span>
              </div>
            </ElCheckbox>
          </ElCheckboxGroup>

          <ElButton
            type="primary"
            :loading="creatingGroup"
            :disabled="selectedGroupMemberIds.length === 0"
            style="width: 100%; margin-top: 12px"
            @click="handleCreateGroup"
          >
            创建群聊
          </ElButton>
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
            :key="app.id"
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
                :loading="actionLoading.has(app.id)"
                @click="handleAccept(app.id)"
              >
                接受
              </ElButton>
              <ElButton
                size="small"
                type="danger"
                :loading="actionLoading.has(app.id)"
                plain
                @click="handleReject(app.id)"
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

.fv-online-text {
  color: var(--aim-online);
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

.fv-create-group {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
}

.fv-create-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--aim-text-muted);
  margin-top: var(--space-2);
}

.fv-group-select {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 280px;
  overflow-y: auto;
}

.fv-group-item {
  margin: 0;
  width: 100%;
}

.fv-group-item-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
</style>
