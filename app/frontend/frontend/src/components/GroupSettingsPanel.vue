<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import {
  ElAvatar,
  ElButton,
  ElCheckbox,
  ElCheckboxGroup,
  ElDrawer,
  ElInput,
  ElMessage,
  ElPopconfirm,
} from 'element-plus'
import {
  AddGroupMembers,
  DismissGroup,
  GetConversationMembers,
  LeaveGroup,
  ListFriends,
  RemoveGroupMember,
  UpdateGroupInfo,
} from '../../wailsjs/go/main/App'
import type { Conversation, FriendItem, GroupMemberItem } from './types'

interface Props {
  visible: boolean
  conversation: Conversation | null
  currentUserId: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  updated: [conversation: Conversation]
  left: [conversationId: string]
  dismissed: [conversationId: string]
}>()

const members = ref<GroupMemberItem[]>([])
const loading = ref(false)
const savingName = ref(false)
const actionLoading = ref(false)

const editName = ref('')
const friends = ref<FriendItem[]>([])
const selectedFriendIds = ref<string[]>([])
const showAddMembers = ref(false)

const isGroup = computed(() => props.conversation?.conversationType === 'group')
const isCreator = computed(
  () => isGroup.value && props.conversation?.creatorId === props.currentUserId,
)

const drawerVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
})

function formatEmail(email: string): string {
  const idx = email.indexOf('@')
  return idx > 0 ? email.slice(0, idx) : email
}

function mapMember(item: { user_id: string; email: string; avatar: string; role: string; joined_at: number }): GroupMemberItem {
  return {
    userId: item.user_id,
    email: item.email,
    avatar: item.avatar ?? '',
    role: item.role,
    joinedAt: item.joined_at,
  }
}

async function loadMembers() {
  if (!props.conversation || !isGroup.value) return
  loading.value = true
  try {
    const resp = await GetConversationMembers(props.conversation.id)
    members.value = (resp?.members ?? []).map(mapMember)
    editName.value = props.conversation.title
  } catch (err) {
    const msg = err instanceof Error ? err.message : '加载群成员失败'
    ElMessage.error(msg)
  } finally {
    loading.value = false
  }
}

async function loadFriendsForAdd() {
  try {
    const resp = await ListFriends()
    const memberIdSet = new Set(members.value.map((m) => m.userId))
    memberIdSet.add(props.currentUserId)

    const items: FriendItem[] = []
    for (const item of resp?.friends ?? []) {
      const otherId = item.user_id === props.currentUserId ? item.friend_id : item.user_id
      if (memberIdSet.has(otherId)) continue
      items.push({
        id: otherId,
        userId: item.user_id,
        friendId: item.friend_id,
        email: `用户 ${otherId}`,
        avatar: '',
        isOnline: false,
      })
    }
    friends.value = items
  } catch {
    friends.value = []
  }
}

async function handleSaveName() {
  if (!props.conversation) return
  const name = editName.value.trim()
  if (!name) {
    ElMessage.warning('群名称不能为空')
    return
  }
  savingName.value = true
  try {
    const resp = await UpdateGroupInfo(props.conversation.id, { name: name })
    const updated: Conversation = {
      ...props.conversation,
      title: resp?.name || name,
      avatar: resp?.avatar || props.conversation.avatar,
      creatorId: resp?.creator_id || props.conversation.creatorId,
    }
    emit('updated', updated)
    ElMessage.success('群名称已更新')
  } catch (err) {
    const msg = err instanceof Error ? err.message : '更新群信息失败'
    ElMessage.error(msg)
  } finally {
    savingName.value = false
  }
}

async function handleAddMembers() {
  if (!props.conversation || selectedFriendIds.value.length === 0) return
  actionLoading.value = true
  try {
    const resp = await AddGroupMembers(props.conversation.id, {
      member_ids: selectedFriendIds.value,
    })
    const updated: Conversation = {
      ...props.conversation,
      memberIds: resp?.member_ids ?? props.conversation.memberIds,
    }
    emit('updated', updated)
    selectedFriendIds.value = []
    showAddMembers.value = false
    await loadMembers()
    ElMessage.success('已添加群成员')
  } catch (err) {
    const msg = err instanceof Error ? err.message : '添加群成员失败'
    ElMessage.error(msg)
  } finally {
    actionLoading.value = false
  }
}

async function handleRemoveMember(userId: string) {
  if (!props.conversation) return
  actionLoading.value = true
  try {
    await RemoveGroupMember(props.conversation.id, userId)
    members.value = members.value.filter((m) => m.userId !== userId)
    ElMessage.success('已移除群成员')
  } catch (err) {
    const msg = err instanceof Error ? err.message : '移除群成员失败'
    ElMessage.error(msg)
  } finally {
    actionLoading.value = false
  }
}

async function handleLeave() {
  if (!props.conversation) return
  actionLoading.value = true
  try {
    await LeaveGroup(props.conversation.id)
    emit('left', props.conversation.id)
    drawerVisible.value = false
    ElMessage.success('已退出群聊')
  } catch (err) {
    const msg = err instanceof Error ? err.message : '退出群聊失败'
    ElMessage.error(msg)
  } finally {
    actionLoading.value = false
  }
}

async function handleDismiss() {
  if (!props.conversation) return
  actionLoading.value = true
  try {
    await DismissGroup(props.conversation.id)
    emit('dismissed', props.conversation.id)
    drawerVisible.value = false
    ElMessage.success('群聊已解散')
  } catch (err) {
    const msg = err instanceof Error ? err.message : '解散群聊失败'
    ElMessage.error(msg)
  } finally {
    actionLoading.value = false
  }
}

watch(
  () => [props.visible, props.conversation?.id] as const,
  ([visible]) => {
    if (visible && props.conversation) {
      showAddMembers.value = false
      selectedFriendIds.value = []
      loadMembers()
    }
  },
)

watch(showAddMembers, (open) => {
  if (open) {
    loadFriendsForAdd()
  }
})
</script>

<template>
  <ElDrawer
    v-model="drawerVisible"
    title="群聊设置"
    direction="rtl"
    size="320px"
    :destroy-on-close="false"
  >
    <div v-if="!conversation || !isGroup" class="gsp-empty">当前会话不是群聊</div>

    <div v-else class="gsp-body">
      <section class="gsp-section">
        <label class="gsp-label">群名称</label>
        <div class="gsp-name-row">
          <ElInput v-model="editName" size="small" :disabled="!isCreator || savingName" />
          <ElButton
            v-if="isCreator"
            size="small"
            type="primary"
            :loading="savingName"
            @click="handleSaveName"
          >
            保存
          </ElButton>
        </div>
      </section>

      <section class="gsp-section">
        <div class="gsp-section-head">
          <span class="gsp-label">群成员（{{ members.length }}）</span>
          <ElButton size="small" link type="primary" @click="showAddMembers = !showAddMembers">
            {{ showAddMembers ? '取消' : '添加成员' }}
          </ElButton>
        </div>

        <div v-if="showAddMembers" class="gsp-add-panel">
          <ElCheckboxGroup v-model="selectedFriendIds" class="gsp-friend-list">
            <ElCheckbox
              v-for="friend in friends"
              :key="friend.id"
              :value="friend.id"
              class="gsp-friend-item"
            >
              {{ formatEmail(friend.email) }}
            </ElCheckbox>
          </ElCheckboxGroup>
          <ElEmpty v-if="friends.length === 0" description="没有可添加的好友" :image-size="48" />
          <ElButton
            type="primary"
            size="small"
            :disabled="selectedFriendIds.length === 0"
            :loading="actionLoading"
            @click="handleAddMembers"
          >
            确认添加
          </ElButton>
        </div>

        <div v-if="loading" class="gsp-loading">加载中...</div>
        <ul v-else class="gsp-member-list">
          <li v-for="member in members" :key="member.userId" class="gsp-member-item">
            <ElAvatar :src="member.avatar" :size="32">
              {{ formatEmail(member.email).slice(0, 1) }}
            </ElAvatar>
            <div class="gsp-member-body">
              <span class="gsp-member-name">{{ formatEmail(member.email) }}</span>
              <span class="gsp-member-role">{{ member.role }}</span>
            </div>
            <ElPopconfirm
              v-if="isCreator && member.userId !== currentUserId"
              title="确定移除该成员？"
              @confirm="handleRemoveMember(member.userId)"
            >
              <template #reference>
                <ElButton size="small" type="danger" link :disabled="actionLoading">移除</ElButton>
              </template>
            </ElPopconfirm>
          </li>
        </ul>
      </section>

      <section class="gsp-actions">
        <ElPopconfirm title="确定退出该群聊？" @confirm="handleLeave">
          <template #reference>
            <ElButton type="warning" plain :loading="actionLoading" style="width: 100%">
              退出群聊
            </ElButton>
          </template>
        </ElPopconfirm>

        <ElPopconfirm
          v-if="isCreator"
          title="解散后所有成员将无法继续聊天，确定解散？"
          @confirm="handleDismiss"
        >
          <template #reference>
            <ElButton type="danger" plain :loading="actionLoading" style="width: 100%; margin-top: 8px">
              解散群聊
            </ElButton>
          </template>
        </ElPopconfirm>
      </section>
    </div>
  </ElDrawer>
</template>

<style scoped>
.gsp-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.gsp-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.gsp-section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.gsp-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--aim-text-muted);
}

.gsp-name-row {
  display: flex;
  gap: var(--space-2);
}

.gsp-add-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2);
  background: var(--aim-surface-2);
  border-radius: 6px;
  border: 1px solid var(--aim-border);
}

.gsp-friend-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 140px;
  overflow-y: auto;
}

.gsp-friend-item {
  margin: 0;
}

.gsp-loading {
  font-size: 12px;
  color: var(--aim-text-muted);
}

.gsp-member-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
}

.gsp-member-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.gsp-member-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.gsp-member-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--aim-text);
}

.gsp-member-role {
  font-size: 10px;
  color: var(--aim-text-muted);
}

.gsp-actions {
  margin-top: var(--space-2);
  padding-top: var(--space-3);
  border-top: 1px solid var(--aim-border);
}

.gsp-empty {
  font-size: 12px;
  color: var(--aim-text-muted);
}
</style>
