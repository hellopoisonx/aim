<template>
  <a-config-provider>
    <div class="shell">
      <a-layout class="layout">
        <a-layout-header class="header">
          <div class="brand">AIM Desktop</div>
          <a-space>
            <a-tag v-if="session" color="blue">{{ currentAccountName }}</a-tag>
            <a-tag :color="connected ? 'green' : 'gray'">{{ connected ? 'WS 已连接' : '离线/未连接' }}</a-tag>
            <a-select v-if="accounts.length" :model-value="currentUserId" size="small" class="account-select" placeholder="切换账号" :loading="switching" @change="switchAccount">
              <a-option v-for="acct in accounts" :key="acct.user_id" :value="acct.user_id" :label="accountName(acct)">
                {{ accountName(acct) }}{{ acct.logged_in ? '' : '（需登录）' }}
              </a-option>
            </a-select>
            <a-button v-if="loggedIn && !addingAccount" size="small" @click="startAddAccount">添加账号</a-button>
            <a-button v-if="loggedIn" size="small" status="warning" @click="logout">退出当前账号</a-button>
            <a-button size="small" @click="showSettings = true">设置</a-button>
          </a-space>
        </a-layout-header>
        <a-layout-content class="content">
          <div v-if="showAuthPanel" class="auth-panel">
            <a-card :bordered="false" class="auth-card">
              <template #title>{{ authMode === 'login' ? '登录' : '注册' }}{{ addingAccount ? '新账号' : '' }}</template>
              <a-form :model="authForm" layout="vertical" @submit-success="submitAuth">
                <a-form-item field="email" label="邮箱"><a-input v-model="authForm.email" /></a-form-item>
                <a-form-item v-if="authMode === 'register'" field="username" label="昵称"><a-input v-model="authForm.username" /></a-form-item>
                <a-form-item field="password" label="密码"><a-input-password v-model="authForm.password" /></a-form-item>
                <a-space direction="vertical" fill>
                  <a-button html-type="submit" type="primary" long :loading="loading">{{ authMode === 'login' ? '登录' : '注册并登录' }}</a-button>
                  <a-button long @click="authMode = authMode === 'login' ? 'register' : 'login'">切换到{{ authMode === 'login' ? '注册' : '登录' }}</a-button>
                  <a-button v-if="addingAccount && loggedIn" long @click="cancelAddAccount">取消添加账号</a-button>
                  <a-alert v-if="status" :type="statusType" :content="status" />
                </a-space>
              </a-form>
              <template v-if="accounts.length">
                <a-divider />
                <div class="muted account-title">本机账号（缓存隔离）</div>
                <a-list class="list compact" :bordered="false">
                  <a-list-item v-for="acct in accounts" :key="acct.user_id">
                    <a-list-item-meta :title="accountName(acct)" :description="acct.logged_in ? '已保存登录态' : '需要重新登录'" />
                    <template #actions>
                      <a-button size="mini" :disabled="acct.active && loggedIn" @click="switchAccount(acct.user_id)">{{ acct.active && loggedIn ? '当前' : '切换' }}</a-button>
                    </template>
                  </a-list-item>
                </a-list>
              </template>
            </a-card>
          </div>

          <a-layout v-else class="chat-layout">
            <a-layout-sider :width="310" class="sider">
              <a-tabs default-active-key="chats" lazy-load>
                <a-tab-pane key="chats" title="会话">
                  <a-space direction="vertical" fill>
                    <a-button type="primary" long @click="refreshConversations">刷新会话</a-button>
                    <a-button long @click="openDirectChat">发起私聊</a-button>
                    <a-button long @click="openCreateGroup">创建群聊</a-button>
                  </a-space>
                  <a-list class="list" :bordered="false">
                    <a-list-item v-for="c in conversations" :key="c.conversation_id" class="clickable" @click="selectConversation(c)">
                      <a-list-item-meta :title="conversationTitle(c)" :description="conversationListDescription(c)" />
                    </a-list-item>
                  </a-list>
                </a-tab-pane>
                <a-tab-pane key="friends" title="好友">
                  <a-input-search v-model="searchName" placeholder="搜索用户昵称/邮箱" @search="searchUsers" @press-enter="searchUsers" />
                  <a-list class="list" :bordered="false">
                    <a-list-item v-for="u in searchResults" :key="u.id">
                      <a-list-item-meta :title="userName(u)" :description="u.email" class="clickable" @click="openUserDetail(u.id, u)" />
                      <template #actions>
                        <a-button size="mini" @click="addFriend(u.id)">添加</a-button>
                        <a-button size="mini" type="primary" @click="startDirectChat(u.id)">私聊</a-button>
                      </template>
                    </a-list-item>
                  </a-list>
                  <a-divider />
                  <a-button long @click="refreshFriends">刷新好友/申请</a-button>
                  <a-card v-if="friendApplications.length" title="好友申请" :bordered="false" class="compact-card">
                    <a-list class="list compact" :bordered="false">
                      <a-list-item v-for="f in friendApplications" :key="`${f.user_id}-${f.friend_id}`">
                        <a-list-item-meta :title="friendName(f)" :description="friendDescription(f)" class="clickable" @click="openUserDetail(f.friend_id, friendAsUser(f))" />
                        <template #actions>
                          <a-button size="mini" type="primary" @click="acceptFriendApplication(f)">接受</a-button>
                          <a-button size="mini" status="danger" @click="rejectFriendApplication(f)">拒绝</a-button>
                        </template>
                      </a-list-item>
                    </a-list>
                  </a-card>
                  <a-divider v-if="friendApplications.length" />
                  <a-list class="list" :bordered="false">
                    <a-list-item v-for="f in friends" :key="f.friend_id">
                      <a-list-item-meta :title="friendName(f)" :description="friendDescription(f)" class="clickable" @click="openUserDetail(f.friend_id, friendAsUser(f))" />
                      <template #actions>
                        <a-tag size="small" :color="presenceColor(f.friend_id)">{{ presenceText(f.friend_id) }}</a-tag>
                        <a-button v-if="f.status === 'accepted'" size="mini" type="primary" @click="startDirectChat(f.friend_id)">私聊</a-button>
                      </template>
                    </a-list-item>
                  </a-list>
                </a-tab-pane>
              </a-tabs>
            </a-layout-sider>

            <a-layout-content class="messages">
              <template v-if="activeConversation">
                <div class="conversation-title">
                  <div>
                    <strong>{{ conversationTitle(activeConversation) }}</strong>
                    <span class="muted"> {{ conversationDescription(activeConversation) }}</span>
                    <a-tag v-if="directPeerId" size="small" :color="presenceColor(directPeerId)">{{ presenceText(directPeerId) }}</a-tag>
                    <div v-if="activeTypingText" class="muted">{{ activeTypingText }}</div>
                  </div>
                  <a-space>
                    <a-button size="small" @click="loadHistory(true)">更早</a-button>
                    <a-button size="small" @click="showMembers = true">详情</a-button>
                  </a-space>
                </div>
                <div ref="messageListRef" class="message-list">
                  <div v-for="m in visibleMessages" :key="messageKey(m)" class="message" :class="messageClass(m)">
                    <div v-if="m.is_system" class="system-bubble">
                      <span class="system-content">{{ m.content }}</span>
                      <span class="system-time">{{ formatTime(m.created_at) }}</span>
                    </div>
                    <div v-else class="bubble">
                      <div class="meta"><span class="clickable" @click="openUserDetail(m.sender_id, senderAsUser(m))">{{ senderName(m) }}</span> · {{ formatTime(m.created_at) }} · {{ m.status || 'synced' }}</div>
                      <template v-if="messageAttachmentList(m).length">
                        <div
                          v-for="att in messageAttachmentList(m)"
                          :key="att.file_id || messageKey(m)"
                          class="attachment-card clickable-attachment"
                          role="button"
                          tabindex="0"
                          title="点击预览附件"
                          @click="openAttachmentPreview(att)"
                          @keydown.enter.prevent="openAttachmentPreview(att)"
                          @keydown.space.prevent="openAttachmentPreview(att)"
                        >
                          <div class="attachment-thumb" :class="`attachment-thumb-${att.kind}`">
                            <img v-if="att.kind === 'image' && attachmentPreviewURL(att)" :src="attachmentPreviewURL(att)" :alt="attachmentTitle(att)" @error="refreshAttachmentDownloadURL(att.file_id)" />
                            <video v-else-if="att.kind === 'video' && attachmentPreviewURL(att)" :src="attachmentPreviewURL(att)" muted playsinline preload="metadata" @error="refreshAttachmentDownloadURL(att.file_id)"></video>
                            <span v-else class="attachment-thumb-placeholder">{{ attachmentKindIcon(att) }}</span>
                            <span v-if="att.kind === 'video'" class="attachment-play">▶</span>
                          </div>
                          <div class="attachment-info">
                            <div class="attachment-kind">{{ attachmentKindText(att) }}</div>
                            <div class="attachment-name">{{ attachmentTitle(att) }}</div>
                            <div class="muted">{{ attachmentMeta(att) }}</div>
                            <div v-if="att.width || att.height || att.duration_ms" class="muted">{{ attachmentDetailMeta(att) }}</div>
                          </div>
                        </div>
                      </template>
                      <div v-else v-html="renderMessageContent(m)" @click="handleMentionClick"></div>
                      <div v-if="messageReadSummary(m)" class="meta clickable" @click="openReadDetails(m)">{{ messageReadSummary(m) }}</div>
                    </div>
                  </div>
                </div>
                <div class="composer-wrapper">
                  <div v-if="showMentionPopup" class="mention-popup">
                    <div class="mention-popup-title">选择成员</div>
                    <div v-for="m in mentionCandidates" :key="m.user_id" class="mention-popup-item" @mousedown.prevent="selectMention(m)">
                      {{ memberName(m) }} <span class="muted">{{ m.email ? m.email : m.user_id }}</span>
                    </div>
                    <div v-if="!mentionCandidates.length" class="mention-popup-empty">无匹配成员</div>
                  </div>
                  <div class="composer">
                    <div v-if="activeTypingText" class="muted typing-line">{{ activeTypingText }}</div>
                    <a-textarea ref="draftTextareaRef" v-model="draft" :auto-size="{ minRows: 1, maxRows: 4 }" placeholder="输入消息，@ 提及成员" @input="onDraftInput" @keydown="onDraftKeydown" @focus="notifyTyping" />
                    <a-button @click="sendAttachment">附件</a-button>
                    <a-button type="primary" :disabled="!draft.trim()" @click="send">发送</a-button>
                  </div>
                </div>
              </template>
              <a-empty v-else description="选择一个会话开始聊天" />
            </a-layout-content>
          </a-layout>
        </a-layout-content>
      </a-layout>

      <a-modal v-model:visible="showSettings" title="设置" @ok="saveSettings">
        <a-form :model="config" layout="vertical">
          <a-form-item label="Gateway URL"><a-input v-model="config.gateway_url" /></a-form-item>
          <a-form-item label="WebSocket URL"><a-input v-model="config.ws_url" /></a-form-item>
        </a-form>
      </a-modal>

      <a-modal v-model:visible="showDirectChat" title="发起私聊" :width="520" @ok="createDirectChat">
        <a-form :model="directChatForm" layout="vertical">
          <a-form-item label="选择联系人">
            <a-input-search v-model="directChatSearch" placeholder="搜索用户昵称/邮箱，回车提交" @search="searchDirectChatUsers" @press-enter="searchDirectChatUsers" />
            <a-list class="list compact" :bordered="false">
              <a-list-item v-for="u in directChatCandidates" :key="u.id" class="clickable" @click="selectedDirectUserId = u.id">
                <a-list-item-meta :title="userName(u)" :description="u.email" />
                <template #actions>
                  <a-radio :model-value="selectedDirectUserId" :value="u.id" @change="() => { selectedDirectUserId = u.id }">选择</a-radio>
                </template>
              </a-list-item>
            </a-list>
          </a-form-item>
        </a-form>
      </a-modal>

      <a-modal v-model:visible="showCreateGroup" title="创建群聊" :width="520" @ok="createGroup">
        <a-form :model="createGroupForm" layout="vertical">
          <a-form-item label="群名称"><a-input v-model="createGroupForm.name" placeholder="输入群名称" /></a-form-item>
          <a-form-item label="群头像"><a-input v-model="createGroupForm.avatar" placeholder="可选头像 URL" /></a-form-item>
          <a-form-item label="添加成员">
            <a-input-search v-model="createMemberSearch" placeholder="搜索用户昵称/邮箱" @search="searchCreateGroupUsers" @press-enter="searchCreateGroupUsers" />
            <div class="selected-tags" v-if="createGroupForm.member_ids.length">
              <a-tag v-for="id in createGroupForm.member_ids" :key="id" closable @close="removeCreateGroupMember(id)">{{ createMemberLabels[id] || '已选择用户' }}</a-tag>
            </div>
            <a-list class="list compact" :bordered="false">
              <a-list-item v-for="u in createMemberCandidates" :key="u.id">
                <a-list-item-meta :title="userName(u)" :description="u.email" class="clickable" @click="openUserDetail(u.id, u)" />
                <template #actions>
                  <a-checkbox :model-value="isCreateMemberSelected(u.id)" @change="() => toggleCreateGroupMember(u)">选择</a-checkbox>
                </template>
              </a-list-item>
            </a-list>
          </a-form-item>
        </a-form>
      </a-modal>

      <a-drawer v-model:visible="showMembers" title="会话详情" :width="460" @before-open="openMembersDrawer">
        <template v-if="activeConversation">
          <a-space direction="vertical" fill>
            <a-card v-if="isGroupConversation" title="群资料" :bordered="false">
              <a-form :model="groupEditForm" layout="vertical">
                <a-form-item label="群名称"><a-input v-model="groupEditForm.name" :disabled="!canManageGroup" /></a-form-item>
                <a-form-item label="群头像"><a-input v-model="groupEditForm.avatar" :disabled="!canManageGroup" /></a-form-item>
                <a-button v-if="canManageGroup" type="primary" long @click="updateGroupInfo">保存群资料</a-button>
              </a-form>
            </a-card>

            <a-card v-if="isGroupConversation && canManageGroup" title="添加成员" :bordered="false">
              <a-input-search v-model="addMemberSearch" placeholder="搜索用户昵称/邮箱" @search="searchAddMemberUsers" @press-enter="searchAddMemberUsers" />
              <div class="selected-tags" v-if="addMemberIds.length">
                <a-tag v-for="id in addMemberIds" :key="id" closable @close="removePendingMember(id)">{{ addMemberLabels[id] || '已选择用户' }}</a-tag>
              </div>
              <a-list class="list compact" :bordered="false">
                <a-list-item v-for="u in addMemberCandidates" :key="u.id">
                  <a-list-item-meta :title="userName(u)" :description="u.email" class="clickable" @click="openUserDetail(u.id, u)" />
                  <template #actions>
                    <a-checkbox :model-value="isPendingMember(u.id)" @change="() => togglePendingMember(u)">选择</a-checkbox>
                  </template>
                </a-list-item>
              </a-list>
              <a-button type="primary" long :disabled="!addMemberIds.length" @click="addGroupMembers">添加选中成员</a-button>
            </a-card>

            <a-card title="成员" :bordered="false">
              <a-list :bordered="false">
                <a-list-item v-for="m in members" :key="m.user_id">
                  <a-list-item-meta :title="memberName(m)" :description="memberDescription(m)" class="clickable" @click="openUserDetail(m.user_id, memberAsUser(m))" />
                  <template #actions>
                    <a-tag size="small" :color="presenceColor(m.user_id)">{{ presenceText(m.user_id) }}</a-tag>
                    <a-button v-if="canGrantAdmin(m)" size="mini" @click="grantGroupAdmin(m)">设管理员</a-button>
                    <a-button v-if="canRevokeAdmin(m)" size="mini" @click="revokeGroupAdmin(m)">取消管理员</a-button>
                    <a-button v-if="canTransferOwner(m)" size="mini" status="warning" @click="transferGroupOwner(m)">转让群主</a-button>
                    <a-button v-if="canRemoveMember(m)" size="mini" status="danger" @click="removeGroupMember(m)">移除</a-button>
                  </template>
                </a-list-item>
              </a-list>
            </a-card>

            <a-button v-if="isGroupConversation && canDismissGroup" status="danger" long @click="dismissGroup">解散群聊</a-button>
            <a-button v-else-if="isGroupConversation" status="warning" long @click="leaveGroup">退出群聊</a-button>
          </a-space>
        </template>
      </a-drawer>

      <a-drawer v-model:visible="showUserDetail" title="用户详情" :width="360">
        <a-descriptions v-if="selectedUser" :column="1" bordered>
          <a-descriptions-item label="昵称">{{ userName(selectedUser) }}</a-descriptions-item>
          <a-descriptions-item label="邮箱">{{ selectedUser.email || '-' }}</a-descriptions-item>
          <a-descriptions-item label="用户 ID">{{ selectedUser.id }}</a-descriptions-item>
          <a-descriptions-item label="状态"><a-tag :color="presenceColor(selectedUser.id)">{{ presenceText(selectedUser.id) }}</a-tag></a-descriptions-item>
        </a-descriptions>
        <a-empty v-else description="暂无用户信息" />
        <template v-if="selectedUser && selectedUser.id !== currentUserId">
          <a-divider />
          <a-button type="primary" long @click="startDirectChat(selectedUser.id)">发起私聊</a-button>
        </template>
      </a-drawer>

      <a-modal v-model:visible="showReadDetails" :title="readDetailsTitle" :footer="false" :width="520">
        <a-list :bordered="false">
          <a-list-item v-for="d in selectedReadDetails" :key="d.user_id">
            <a-list-item-meta :title="d.display_name" :description="d.email || d.user_id" class="clickable" @click="openUserDetail(d.user_id, { id: d.user_id, nickname: d.display_name, email: d.email, avatar: d.avatar, display_name: d.display_name })" />
            <template #actions>
              <a-tag :color="d.is_read ? 'green' : 'gray'">{{ d.is_read ? '已读' : '未读' }}</a-tag>
              <span class="muted">{{ formatTime(d.updated_at) }}</span>
            </template>
          </a-list-item>
        </a-list>
      </a-modal>
    </div>
  </a-config-provider>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Message, Modal } from '@arco-design/web-vue'
import { BrowserOpenURL, EventsOn } from '../wailsjs/runtime/runtime'
import * as api from '../wailsjs/go/main/App'
import type { main } from '../wailsjs/go/models'

type AttachmentContent = {
  schema?: string
  file_id?: string
  kind?: string
  original?: { name?: string; mime?: string; size?: number; sha256?: string }
  thumbnail_file_id?: string
  parse_status?: string
  duration_ms?: number
  width?: number
  height?: number
  metadata?: unknown
}

const loading = ref(false)
const switching = ref(false)
const connected = ref(false)
const authMode = ref<'login' | 'register'>('login')
const status = ref('')
const statusType = ref<'info' | 'success' | 'warning' | 'error'>('info')
const showSettings = ref(false)
const showMembers = ref(false)
const showDirectChat = ref(false)
const showCreateGroup = ref(false)
const showUserDetail = ref(false)
const showReadDetails = ref(false)
const addingAccount = ref(false)
const authForm = reactive({ email: '', password: '', username: '' })
const config = reactive({ gateway_url: 'http://localhost:8888', ws_url: 'ws://localhost:8888/ws' })
const session = ref<main.SessionInfo | null>(null)
const accounts = ref<main.AccountView[]>([])
const conversations = ref<main.ConversationView[]>([])
const activeConversation = ref<main.ConversationView | null>(null)
const messages = ref<main.MessageView[]>([])
const friends = ref<main.FriendView[]>([])
const friendApplications = ref<main.FriendView[]>([])
const searchName = ref('')
const searchResults = ref<main.UserView[]>([])
const directChatSearch = ref('')
const directChatCandidates = ref<main.UserView[]>([])
const selectedDirectUserId = ref('')
const directChatForm = reactive({})
const members = ref<main.MemberView[]>([])
const draft = ref('')
const pendingMentions = reactive<Map<string, string>>(new Map()) // displayName → userId
const showMentionPopup = ref(false)
const mentionSearch = ref('')
const mentionStartPos = ref(-1) // cursor position when @ was typed
const selectedUser = ref<main.UserView | null>(null)
const selectedReadMessage = ref<main.MessageView | null>(null)
const selectedReadDetails = ref<Array<main.MessageReadDetailView & { display_name: string }>>([])
const readDetailsTitle = ref('已读详情')
const presenceByUserId = reactive<Record<string, main.PresenceView>>({})
const typingByConversation = reactive<Record<string, Record<string, number>>>({})
const readStatesByConversation = reactive<Record<string, Record<string, main.ReadStateView>>>({})
const attachmentByFileId = reactive<Record<string, main.AttachmentView>>({})
const attachmentDownloadByFileId = reactive<Record<string, main.AttachmentDownloadView>>({})
const typingTick = ref(Date.now())
const messageListRef = ref<HTMLElement | null>(null)
const draftTextareaRef = ref<any>(null)
let typingTimer: number | undefined
let readReceiptTimer: number | undefined
let messageFlushTimer: number | undefined
let pendingMessages: main.MessageView[] = []
let scrollTimer: number | undefined
const attachmentRefreshInFlight = new Set<string>()
const attachmentRefreshTimers: Record<string, number> = {}
const attachmentRefreshFailures: Record<string, number> = {}
const attachmentDownloadInFlight = new Set<string>()

const currentUserId = computed(() => session.value?.user_id || '')
const loggedIn = computed(() => !!session.value?.access_token)
const showAuthPanel = computed(() => !loggedIn.value || addingAccount.value)
const currentAccountName = computed(() => safeName(session.value?.nickname, session.value?.email) || '当前账号')
const isGroupConversation = computed(() => activeConversation.value?.conversation_type === 'group')
const directPeerId = computed(() => activeConversation.value && activeConversation.value.conversation_type === 'direct' ? activeConversation.value.member_ids?.find(id => id !== currentUserId.value) || '' : '')
const currentMember = computed(() => members.value.find(m => m.user_id === currentUserId.value))
const currentRole = computed(() => activeConversation.value?.creator_id === currentUserId.value ? 'owner' : (currentMember.value?.role || 'member'))
const canManageGroup = computed(() => isGroupConversation.value && (currentRole.value === 'owner' || currentRole.value === 'admin'))
const canDismissGroup = computed(() => isGroupConversation.value && currentRole.value === 'owner')
const activeTypingText = computed(() => typingTextForConversation(activeConversation.value?.conversation_id || ''))
const visibleMessages = computed(() => messages.value)
const mentionCandidates = computed(() => {
  if (!mentionSearch.value) return members.value.filter(m => m.user_id !== currentUserId.value)
  const q = mentionSearch.value.toLowerCase()
  return members.value.filter(m => m.user_id !== currentUserId.value && (memberName(m).toLowerCase().includes(q) || (m.email || '').toLowerCase().includes(q)))
})

// --- Mention input handling ---
function onDraftInput(value: string) {
  notifyTyping()
  const el = draftTextareaRef.value?.$el?.querySelector('textarea') as HTMLTextAreaElement | undefined
  const cursorPos = el?.selectionStart ?? value.length
  // Find the last @ before cursor that starts a mention segment
  const textBeforeCursor = value.slice(0, cursorPos)
  const atIdx = textBeforeCursor.lastIndexOf('@')
  if (atIdx >= 0 && (atIdx === 0 || /[\s\n]/.test(value[atIdx - 1]))) {
    const afterAt = textBeforeCursor.slice(atIdx + 1)
    // If no space/newline between @ and cursor, it's a mention in progress
    if (!afterAt.includes(' ') && !afterAt.includes('\n')) {
      mentionStartPos.value = atIdx
      mentionSearch.value = afterAt
      showMentionPopup.value = members.value.length > 0
      return
    }
  }
  showMentionPopup.value = false
  mentionSearch.value = ''
}

function onDraftKeydown(e: KeyboardEvent) {
  notifyTyping()
  if (e.key === 'Escape' && showMentionPopup.value) {
    showMentionPopup.value = false
    e.preventDefault()
    return
  }
}

function selectMention(m: main.MemberView) {
  const name = memberName(m)
  const before = draft.value.slice(0, mentionStartPos.value)
  const after = draft.value.slice(draftTextareaRef.value?.$el?.querySelector('textarea')?.selectionStart ?? draft.value.length)
  draft.value = before + '@' + name + ' ' + after
  pendingMentions.set(name, m.user_id)
  showMentionPopup.value = false
  mentionSearch.value = ''
  nextTick(() => { const el = draftTextareaRef.value?.$el?.querySelector('textarea') as HTMLTextAreaElement | undefined; if (el) el.focus() })
}

function closeMentionPopupOnClickOutside(e: MouseEvent) {
  if (!showMentionPopup.value) return
  const target = e.target as HTMLElement
  if (target.closest('.mention-popup')) return
  showMentionPopup.value = false
  mentionSearch.value = ''
}


function typingTextForConversation(conversationID: string) {
  typingTick.value
  if (!conversationID) return ''
  const entries = typingByConversation[conversationID] || {}
  const now = Date.now()
  Object.entries(entries).forEach(([uid, until]) => { if (until <= now) delete entries[uid] })
  const names = Object.entries(entries).filter(([uid, until]) => uid !== currentUserId.value && until > now).map(([uid]) => displayNameByUserId(uid))
  return names.length ? `${names.slice(0, 3).join('、')} 正在输入…` : ''
}

const createGroupForm = reactive({ name: '', avatar: '', member_ids: [] as string[] })
const createMemberSearch = ref('')
const createMemberCandidates = ref<main.UserView[]>([])
const createMemberLabels = reactive<Record<string, string>>({})
const groupEditForm = reactive({ name: '', avatar: '' })
const addMemberSearch = ref('')
const addMemberCandidates = ref<main.UserView[]>([])
const addMemberIds = ref<string[]>([])
const addMemberLabels = reactive<Record<string, string>>({})

onMounted(async () => {
  typingTimer = window.setInterval(() => { typingTick.value = Date.now() }, 1000)
  document.addEventListener('click', closeMentionPopupOnClickOutside)
  registerRuntimeEvents()

  try {
    Object.assign(config, await api.GetConfig())
    await reloadAccounts()
    const auto = await api.AutoLogin()
    if (auto?.user_id) session.value = auto
    if (auto?.access_token) {
      await loadCachedData()
      await refreshConversations()
      await refreshFriends()
    }
  } catch (e) {
    console.warn(e)
  }
})

function registerRuntimeEvents() {
  EventsOn('ws:connection', (payload: any) => { connected.value = !!payload?.connected })
  EventsOn('ws:message', (payload: main.MessageView) => { queueMessage(payload); if (payload.conversation_id === activeConversation.value?.conversation_id) scheduleReadReceipt() })
  EventsOn('ws:server-ack', (payload: any) => {
    const msg = messages.value.find(m => m.client_msg_id === payload.client_msg_id)
    if (msg) {
      msg.status = payload.status === 1 ? 'accepted' : 'rejected'
      if (payload.message_id) msg.message_id = payload.message_id
      sendReadReceiptLatest()
    }
  })
  EventsOn('ws:presence', (payload: main.PresenceView) => { if (payload?.user_id) presenceByUserId[payload.user_id] = payload })
  EventsOn('ws:typing', (payload: any) => {
    if (!payload?.conversation_id || !payload?.user_id || payload.user_id === currentUserId.value) return
    typingByConversation[payload.conversation_id] = { ...(typingByConversation[payload.conversation_id] || {}), [payload.user_id]: Date.now() + 6000 }
    typingTick.value = Date.now()
  })
  EventsOn('ws:read-receipt', (payload: any) => applyReadReceipt(payload))
  EventsOn('ws:friend-application', async () => { await refreshFriends() })
  EventsOn('ws:token-expired', async () => { await refreshToken() })
}

onBeforeUnmount(() => {
  document.removeEventListener('click', closeMentionPopupOnClickOutside)
  if (typingTimer) window.clearInterval(typingTimer)
  if (readReceiptTimer) window.clearTimeout(readReceiptTimer)
  if (messageFlushTimer) window.clearTimeout(messageFlushTimer)
  Object.values(attachmentRefreshTimers).forEach(timer => window.clearTimeout(timer))
  if (scrollTimer) window.clearTimeout(scrollTimer)
})

async function submitAuth() {
  loading.value = true
  status.value = ''
  try {
    if (authMode.value === 'register') await api.Register({ email: authForm.email, password: authForm.password, username: authForm.username, avatar: '' })
    resetLocalState()
    session.value = await api.Login({ email: authForm.email, password: authForm.password })
    await reloadAccounts()
    addingAccount.value = false
    statusType.value = 'success'; status.value = '登录成功'
    await refreshConversations(); await refreshFriends()
  } catch (e: any) {
    statusType.value = 'error'; status.value = String(e)
  } finally { loading.value = false }
}
async function saveSettings() { await api.SaveConfig(config); Message.success('已保存') }
async function reloadAccounts() { accounts.value = await api.ListAccounts() }
async function refreshToken() { try { session.value = await api.RefreshToken(); await reloadAccounts() } catch { session.value = null; await reloadAccounts() } }
function resetLocalState() {
  connected.value = false; conversations.value = []; activeConversation.value = null; messages.value = []; friends.value = []; friendApplications.value = []; searchResults.value = []; members.value = []; draft.value = ''; pendingMentions.clear(); showMentionPopup.value = false; mentionSearch.value = ''
  Object.keys(presenceByUserId).forEach(k => delete presenceByUserId[k]); Object.keys(typingByConversation).forEach(k => delete typingByConversation[k]); Object.keys(readStatesByConversation).forEach(k => delete readStatesByConversation[k]); Object.keys(attachmentByFileId).forEach(k => delete attachmentByFileId[k]); Object.keys(attachmentDownloadByFileId).forEach(k => delete attachmentDownloadByFileId[k])
  Object.entries(attachmentRefreshTimers).forEach(([k, timer]) => { window.clearTimeout(timer); delete attachmentRefreshTimers[k] }); Object.keys(attachmentRefreshFailures).forEach(k => delete attachmentRefreshFailures[k]); attachmentRefreshInFlight.clear(); attachmentDownloadInFlight.clear()
}
function startAddAccount() { addingAccount.value = true; authMode.value = 'login'; status.value = ''; Object.assign(authForm, { email: '', password: '', username: '' }) }
function cancelAddAccount() { addingAccount.value = false; status.value = ''; authForm.password = '' }
async function switchAccount(userID: unknown) {
  const id = String(userID || '')
  if (!id || id === currentUserId.value) return
  switching.value = true; status.value = ''
  try {
    resetLocalState(); session.value = await api.SwitchAccount(id); await reloadAccounts(); addingAccount.value = false; authForm.email = session.value?.email || ''; authForm.password = ''
    if (session.value?.access_token) { await loadCachedData(); await refreshConversations(); await refreshFriends() }
  } catch (e: any) { Message.error(String(e)) } finally { switching.value = false }
}
async function logout() { const id = currentUserId.value; await api.Logout(); resetLocalState(); session.value = id ? await api.SwitchAccount(id) : null; await reloadAccounts(); Message.success('已退出当前账号') }
async function loadCachedData() { try { conversations.value = await api.GetCachedConversations() } catch {}; try { friends.value = await api.GetCachedFriends() } catch {} }
async function refreshConversations() { try { conversations.value = await api.ListConversations() } catch { conversations.value = await api.GetCachedConversations() } }
async function refreshFriends() {
  try { friends.value = await api.ListFriends() } catch { friends.value = await api.GetCachedFriends() }
  try { friendApplications.value = await api.ListFriendApplications() } catch {}
  await refreshPresence()
}
async function refreshPresence() { try { (await api.GetFriendsPresence()).forEach(p => { presenceByUserId[p.user_id] = p }) } catch {} }
async function searchUsers() { searchResults.value = searchName.value ? excludeCurrentUser(await api.SearchUsers(searchName.value)) : [] }
function excludeCurrentUser(users: main.UserView[]) { return users.filter(u => u.id && u.id !== currentUserId.value) }
async function addFriend(id: string) { if (id === currentUserId.value) return; await api.AddFriend(id); Message.success('已发送申请') }
async function acceptFriendApplication(f: main.FriendView) { const v = await api.AcceptFriend(f.friend_id); mergeFriend(v); friendApplications.value = friendApplications.value.filter(x => x.friend_id !== f.friend_id); await refreshFriends(); Message.success('已接受') }
async function rejectFriendApplication(f: main.FriendView) { await api.RejectFriend(f.friend_id); friendApplications.value = friendApplications.value.filter(x => x.friend_id !== f.friend_id); Message.success('已拒绝') }
function mergeFriend(f: main.FriendView) { const i = friends.value.findIndex(x => x.friend_id === f.friend_id); if (i >= 0) friends.value[i] = { ...friends.value[i], ...f }; else friends.value.unshift(f) }
async function selectConversation(c: main.ConversationView) {
  activeConversation.value = c; pendingMentions.clear(); showMentionPopup.value = false; draft.value = ''
  messages.value = await api.GetCachedMessages(c.conversation_id, 80)
  resolveAttachmentsForMessages(messages.value)
  scheduleScrollToBottom()
  scheduleReadReceipt()
  await Promise.all([loadMembers(), loadHistory(false)])
  scheduleReadReceipt()
}
async function loadHistory(older: boolean) {
  if (!activeConversation.value) return
  const oldest = older && messages.value.length ? messages.value[0] : null
  const res = await api.GetConversationHistory(activeConversation.value.conversation_id, oldest?.created_at || 0, oldest?.message_id || '', 50)
  res.read_states?.forEach(st => applyReadState(activeConversation.value!.conversation_id, st))
  mergeMessages(res.messages || [])
  scheduleReadReceipt()
}
async function openMembersDrawer() { if (!activeConversation.value) return; groupEditForm.name = activeConversation.value.name || activeConversation.value.display_name || ''; groupEditForm.avatar = activeConversation.value.avatar || ''; await loadMembers() }
async function loadMembers() { if (!activeConversation.value) return; try { members.value = await api.GetCachedConversationMembers(activeConversation.value.conversation_id) } catch {}; try { members.value = await api.GetConversationMembers(activeConversation.value.conversation_id) } catch (e) { if (!members.value.length) Message.error(String(e)) } }
async function send() { if (!activeConversation.value || !draft.value.trim()) return; const content = draft.value; const mentionUserIds = [...pendingMentions.values()]; draft.value = ''; pendingMentions.clear(); showMentionPopup.value = false; lastTyping = 0; const msg = await api.SendMessage(activeConversation.value.conversation_id, 'text', content, mentionUserIds); mergeMessage(msg); scheduleReadReceipt() }
async function sendAttachment() {
  if (!activeConversation.value) return
  try { const msg = await api.ChooseAttachmentAndSend(activeConversation.value.conversation_id); mergeMessage(msg); scheduleReadReceipt() }
  catch (e: any) { if (String(e || '').includes('未选择文件')) return; Message.error(String(e)) }
}
let lastTyping = 0
function notifyTyping() { const now = Date.now(); if (activeConversation.value && now - lastTyping > 1000) { lastTyping = now; api.SendTyping(activeConversation.value.conversation_id).catch(() => {}) } }
function queueMessage(m: main.MessageView) {
  if (m.conversation_id !== activeConversation.value?.conversation_id) return
  pendingMessages.push(m)
  if (messageFlushTimer) return
  messageFlushTimer = window.setTimeout(() => {
    const batch = pendingMessages
    pendingMessages = []
    messageFlushTimer = undefined
    mergeMessages(batch)
  }, 16)
}
function mergeMessages(batch: main.MessageView[]) {
  if (!batch.length) return
  const shouldScroll = isNearMessageBottom()
  const next = [...messages.value]
  batch.forEach(m => mergeMessageInto(next, m))
  next.sort(compareMessages)
  messages.value = next
  resolveAttachmentsForMessages(batch)
  if (shouldScroll) scheduleScrollToBottom()
}
function mergeMessage(m: main.MessageView) { mergeMessages([m]) }
function mergeMessageInto(list: main.MessageView[], m: main.MessageView) { const idx = list.findIndex(x => (m.message_id && x.message_id === m.message_id) || (m.client_msg_id && x.client_msg_id === m.client_msg_id)); if (idx >= 0) list[idx] = Object.assign(list[idx], m); else list.push(m); m.read_details?.forEach(rd => applyReadState(m.conversation_id, { user_id: rd.user_id, last_read_message_id: rd.last_read_message_id, updated_at: rd.updated_at, email: rd.email, avatar: rd.avatar, display_name: rd.display_name })) }
function compareMessages(a: main.MessageView, b: main.MessageView) { return (a.created_at || 0) - (b.created_at || 0) || compareSnowflakeID(a.message_id, b.message_id) }
function compareSnowflakeID(a?: string, b?: string) { if (!a && !b) return 0; if (!a) return -1; if (!b) return 1; try { const av = BigInt(a), bv = BigInt(b); return av < bv ? -1 : av > bv ? 1 : 0 } catch { return a.localeCompare(b) } }
function isIDGTE(a?: string, b?: string) { if (!a || !b) return false; return compareSnowflakeID(a, b) >= 0 }
function messageKey(m: main.MessageView) { return m.message_id || m.client_msg_id }
function formatTime(ts?: number) { return ts ? new Date(ts > 1e12 ? ts : ts * 1000).toLocaleString() : '' }
function messageAttachmentList(m: main.MessageView): AttachmentContent[] { const c = attachmentContent(m); return c ? [c] : [] }
function attachmentContent(m: main.MessageView): AttachmentContent | null { const c = attachmentContentRaw(m); if (!c?.file_id) return c; const current = attachmentByFileId[c.file_id]; return current ? mergeAttachmentInfo(c, current) : c }
function attachmentContentRaw(m: main.MessageView): AttachmentContent | null { if (!['image', 'video', 'audio'].includes(m.message_type)) return null; try { const v = JSON.parse(m.content || '{}') as AttachmentContent; return v.schema === 'aim.attachment.v1' ? v : null } catch { return null } }
function mergeAttachmentInfo(c: AttachmentContent, info: main.AttachmentView): AttachmentContent { return { ...c, kind: info.kind || c.kind, original: { ...(c.original || {}), name: info.original_name || c.original?.name, mime: info.mime || c.original?.mime, size: info.size || c.original?.size, sha256: info.sha256 || c.original?.sha256 }, parse_status: info.parse_status || c.parse_status, thumbnail_file_id: info.thumbnail_object_key || c.thumbnail_file_id, duration_ms: info.duration_ms || c.duration_ms, width: info.width || c.width, height: info.height || c.height, metadata: info.metadata || c.metadata } }
function resolveAttachmentsForMessages(list: main.MessageView[]) { list.forEach(m => { const c = attachmentContentRaw(m); if (!c?.file_id) return; const current = attachmentByFileId[c.file_id]; if (!current || (current.parse_status !== 'ready' && current.parse_status !== 'failed')) scheduleAttachmentRefresh(c.file_id); ensureAttachmentDownloadURL(c.file_id) }) }
function scheduleAttachmentRefresh(fileID: string, delay = 0) { if (!fileID || attachmentRefreshTimers[fileID] || (attachmentRefreshFailures[fileID] || 0) >= 5) return; attachmentRefreshTimers[fileID] = window.setTimeout(() => { delete attachmentRefreshTimers[fileID]; refreshAttachment(fileID) }, delay) }
async function refreshAttachment(fileID: string) { if (attachmentRefreshInFlight.has(fileID)) return; attachmentRefreshInFlight.add(fileID); const accountID = currentUserId.value; try { const info = await api.GetAttachment(fileID); if (accountID !== currentUserId.value) return; attachmentByFileId[fileID] = info; attachmentRefreshFailures[fileID] = 0; if (info.parse_status !== 'ready' && info.parse_status !== 'failed' && attachmentStillVisible(fileID)) scheduleAttachmentRefresh(fileID, 3000) } catch { if (accountID !== currentUserId.value) return; attachmentRefreshFailures[fileID] = (attachmentRefreshFailures[fileID] || 0) + 1; if (attachmentStillVisible(fileID)) scheduleAttachmentRefresh(fileID, 5000) } finally { attachmentRefreshInFlight.delete(fileID) } }
function attachmentStillVisible(fileID: string) { return messages.value.some(m => attachmentContentRaw(m)?.file_id === fileID) }
function attachmentPreviewURL(att: AttachmentContent) { const fileID = att.file_id || ''; if (!fileID) return ''; ensureAttachmentDownloadURL(fileID); return attachmentDownloadByFileId[fileID]?.url || '' }
async function ensureAttachmentDownloadURL(fileID: string, force = false) { if (!fileID) return; const cached = attachmentDownloadByFileId[fileID]; if (!force && cached?.url && (!cached.expires_at || cached.expires_at > Date.now() + 60_000)) return; if (attachmentDownloadInFlight.has(fileID)) return; attachmentDownloadInFlight.add(fileID); const accountID = currentUserId.value; try { const download = await api.GetAttachmentDownload(fileID); if (accountID !== currentUserId.value) return; attachmentDownloadByFileId[fileID] = download } catch (e) { console.warn('获取附件下载地址失败', e) } finally { attachmentDownloadInFlight.delete(fileID) } }
function refreshAttachmentDownloadURL(fileID?: string) { if (!fileID) return; delete attachmentDownloadByFileId[fileID]; ensureAttachmentDownloadURL(fileID, true) }
async function openAttachmentPreview(att: AttachmentContent) {
  const fileID = att.file_id || ''
  if (!fileID) return
  const previewWindow = window.open('', `aim-attachment-${fileID}`, 'width=920,height=720')
  if (previewWindow) writeAttachmentPreviewWindow(previewWindow, att, '', '正在加载附件…')
  await ensureAttachmentDownloadURL(fileID)
  const url = attachmentDownloadByFileId[fileID]?.url || ''
  if (!url) {
    if (previewWindow) writeAttachmentPreviewWindow(previewWindow, att, '', '附件下载地址获取失败，请稍后重试')
    else Message.error('附件下载地址获取失败，请稍后重试')
    return
  }
  if (previewWindow) writeAttachmentPreviewWindow(previewWindow, att, url)
  else BrowserOpenURL(url)
}
function writeAttachmentPreviewWindow(target: Window, att: AttachmentContent, url: string, message = '') {
  const title = escapeHTML(attachmentTitle(att))
  const meta = escapeHTML([attachmentKindText(att), attachmentMeta(att), attachmentDetailMeta(att)].filter(Boolean).join(' · '))
  const safeURL = escapeAttr(url)
  const media = message ? `<div class="state">${escapeHTML(message)}</div>` : attachmentPreviewHTML(att, safeURL, title)
  target.document.open()
  target.document.write(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>${title}</title><style>html,body{margin:0;width:100%;height:100%;background:#111827;color:#f9fafb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}.page{height:100%;display:flex;flex-direction:column}.bar{padding:12px 16px;background:#1f2937;border-bottom:1px solid rgba(255,255,255,.12)}.title{font-weight:700;word-break:break-all}.meta{margin-top:4px;color:#9ca3af;font-size:12px}.stage{flex:1;min-height:0;display:flex;align-items:center;justify-content:center;padding:18px}.stage img,.stage video{max-width:100%;max-height:100%;border-radius:10px;box-shadow:0 10px 36px rgba(0,0,0,.35)}.stage audio{width:min(720px,90vw)}.state{color:#d1d5db}.audio-box{display:flex;flex-direction:column;align-items:center;gap:18px}.audio-icon{font-size:72px}.actions{display:flex;gap:10px;margin-top:10px}.actions a{color:#93c5fd;text-decoration:none}</style></head><body><div class="page"><div class="bar"><div class="title">${title}</div><div class="meta">${meta}</div>${safeURL ? `<div class="actions"><a href="${safeURL}" target="_blank" rel="noopener noreferrer">在浏览器打开</a></div>` : ''}</div><div class="stage">${media}</div></div></body></html>`)
  target.document.close()
  target.focus()
}
function attachmentPreviewHTML(att: AttachmentContent, url: string, title: string) {
  if (att.kind === 'image') return `<img src="${url}" alt="${title}" />`
  if (att.kind === 'video') return `<video src="${url}" controls autoplay playsinline></video>`
  if (att.kind === 'audio') return `<div class="audio-box"><div class="audio-icon">🎧</div><audio src="${url}" controls autoplay></audio></div>`
  return `<a href="${url}" target="_blank" rel="noopener noreferrer">打开附件</a>`
}
function attachmentTitle(att: AttachmentContent) { return att.original?.name || att.file_id || '附件' }
function attachmentKindText(att: AttachmentContent) { return att.kind === 'image' ? '图片' : att.kind === 'video' ? '视频' : att.kind === 'audio' ? '音频' : '附件' }
function attachmentKindIcon(att: AttachmentContent) { return att.kind === 'image' ? '🖼️' : att.kind === 'video' ? '🎬' : att.kind === 'audio' ? '🎧' : '📎' }
function attachmentMeta(att: AttachmentContent) { return `${att.original?.mime || '-'} · ${formatBytes(att.original?.size || 0)} · ${att.parse_status || 'pending'}` }
function attachmentDetailMeta(att: AttachmentContent) { const parts = []; if (att.width || att.height) parts.push(`${att.width || 0}×${att.height || 0}`); if (att.duration_ms) parts.push(formatDuration(att.duration_ms)); return parts.join(' · ') }
function formatDuration(ms: number) { const total = Math.max(0, Math.round(ms / 1000)); const m = Math.floor(total / 60); const s = total % 60; return `${m}:${String(s).padStart(2, '0')}` }
function escapeHTML(s: string) { return s.replace(/[&<>"']/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch] || ch)) }
function escapeAttr(s: string) { return escapeHTML(s) }
function formatBytes(size: number) { if (!size) return '0 B'; const units = ['B', 'KB', 'MB', 'GB']; let n = size, i = 0; while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ } return `${n.toFixed(i ? 1 : 0)} ${units[i]}` }

function renderMessageContent(m: main.MessageView): string {
  const text = m.content || ''
  const mentionIds = m.mentions || []
  if (!mentionIds.length) return escapeHTML(text)
  // Build a map: userId → displayName for all mentioned users
  const mentionMap = new Map<string, string>()
  for (const uid of mentionIds) {
    mentionMap.set(uid, displayNameByUserId(uid))
  }
  // Build regex: match @displayName for any mentioned user
  const patterns: string[] = []
  for (const [uid, name] of mentionMap) {
    if (name) patterns.push(escapeRegex(name))
  }
  if (!patterns.length) return escapeHTML(text)
  const re = new RegExp('@(' + patterns.join('|') + ')', 'g')
  // Replace @name with <span class="mention-tag" data-uid="...">@name</span>

  const parts: string[] = []
  let lastIdx = 0
  for (const match of text.matchAll(re)) {
    const matched = match[0] // e.g. @Alice
    const matchIdx = match.index!
    // Find which userId this name corresponds to
    const matchedName = match[1]
    let matchedUid = ''
    for (const [uid, name] of mentionMap) {
      if (name === matchedName) { matchedUid = uid; break }
    }
    // Add escaped text before this match
    parts.push(escapeHTML(text.slice(lastIdx, matchIdx)))
    // Add mention tag
    parts.push(`<span class="mention-tag" data-uid="${escapeAttr(matchedUid)}">${escapeHTML(matched)}</span>`)
    lastIdx = matchIdx + matched.length
  }
  parts.push(escapeHTML(text.slice(lastIdx)))
  return parts.join('')
}

function escapeRegex(s: string) { return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') }

function handleMentionClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.classList.contains('mention-tag')) return
  const uid = target.getAttribute('data-uid')
  if (uid) openUserDetail(uid)
}


function safeName(...names: Array<string | undefined>) { return names.find(v => v && v.trim())?.trim() || '' }
function accountName(a: main.AccountView) { return safeName(a.display_name, a.nickname, a.email) || a.user_id || '未知账号' }
function userName(u: main.UserView) { return safeName(u.display_name, u.nickname, u.email) || '未知用户' }
function friendName(f: main.FriendView) { return safeName(f.display_name, f.email) || '未知用户' }
function memberName(m: main.MemberView) { return safeName(m.display_name, m.email) || '未知用户' }
function senderName(m: main.MessageView) { return safeName(m.sender_info?.display_name, m.sender_info?.name, m.sender_info?.email) || '未知用户' }
function conversationTitle(c: main.ConversationView) { return safeName(c.display_name, c.name) || '未命名会话' }
function conversationDescription(c: main.ConversationView) { return c.conversation_type === 'group' ? '群聊' : c.conversation_type === 'direct' ? '单聊' : '会话' }
function conversationListDescription(c: main.ConversationView) { const typing = typingTextForConversation(c.conversation_id); return typing || conversationDescription(c) }
function messageClass(m: main.MessageView) { return { mine: !m.is_system && m.sender_id === currentUserId.value, system: !!m.is_system } }
function friendDescription(f: main.FriendView) { return `${friendStatusText(f.status)}${f.email ? ' · ' + f.email : ''}` }
function friendStatusText(status?: string) { return status === 'accepted' ? '已添加' : status === 'pending' ? '待确认' : status || '好友' }
function roleText(role?: string) { return role === 'owner' ? '群主' : role === 'admin' ? '管理员' : '成员' }
function memberDescription(m: main.MemberView) { return `${roleText(m.role)}${m.email ? ' · ' + m.email : ''}` }
function displayNameByUserId(id: string) { const m = members.value.find(x => x.user_id === id); if (m) return memberName(m); const f = friends.value.find(x => x.friend_id === id); if (f) return friendName(f); const u = directChatCandidates.value.find(x => x.id === id) || createMemberCandidates.value.find(x => x.id === id) || addMemberCandidates.value.find(x => x.id === id); if (u) return userName(u); return presenceByUserId[id]?.display_name || id }
function directConversationName(peerID: string) { return `${currentAccountName.value} | ${displayNameByUserId(peerID)}` }
function presenceText(id?: string) { if (!id) return '离线'; const s = presenceByUserId[id]?.status; return s === 'online' ? '在线' : '离线' }
function presenceColor(id?: string) { return id && presenceByUserId[id]?.status === 'online' ? 'green' : 'gray' }
function friendAsUser(f: main.FriendView): main.UserView { return { id: f.friend_id, nickname: f.display_name, email: f.email, avatar: f.avatar, display_name: f.display_name } }
function memberAsUser(m: main.MemberView): main.UserView { return { id: m.user_id, nickname: m.display_name, email: m.email, avatar: m.avatar, display_name: m.display_name } }
function senderAsUser(m: main.MessageView): main.UserView { return { id: m.sender_id, nickname: senderName(m), email: m.sender_info?.email || '', avatar: '', display_name: senderName(m) } }
async function openUserDetail(id?: string, fallback?: main.UserView) { if (!id) return; selectedUser.value = fallback || null; showUserDetail.value = true; try { selectedUser.value = await api.GetUserByID(id) } catch { if (!selectedUser.value && fallback) selectedUser.value = fallback } }

function openDirectChat() { showDirectChat.value = true; directChatSearch.value = ''; directChatCandidates.value = []; selectedDirectUserId.value = '' }
async function searchDirectChatUsers() { directChatCandidates.value = directChatSearch.value ? excludeCurrentUser(await api.SearchUsers(directChatSearch.value)) : [] }
async function createDirectChat() {
  if (!selectedDirectUserId.value) { Message.warning('请选择私聊对象'); return false }
  await startDirectChat(selectedDirectUserId.value)
}
async function startDirectChat(id: string) {
  if (!id || id === currentUserId.value) { Message.warning('不能和自己发起私聊'); return }
  const existing = conversations.value.find(c => c.conversation_type === 'direct' && c.member_ids?.includes(id))
  const conv = existing || await api.CreateConversation({ conversation_type: 'direct', member_ids: [id], name: directConversationName(id), avatar: '' })
  mergeConversation(conv)
  showDirectChat.value = false
  showUserDetail.value = false
  await selectConversation(conv)
}

function openCreateGroup() { showCreateGroup.value = true; createGroupForm.name = ''; createGroupForm.avatar = ''; createGroupForm.member_ids = []; createMemberSearch.value = ''; createMemberCandidates.value = [] }
async function searchCreateGroupUsers() { createMemberCandidates.value = createMemberSearch.value ? excludeCurrentUser(await api.SearchUsers(createMemberSearch.value)) : [] }
function isCreateMemberSelected(id: string) { return createGroupForm.member_ids.includes(id) }
function toggleCreateGroupMember(u: main.UserView) { if (u.id === currentUserId.value) return; createMemberLabels[u.id] = userName(u); createGroupForm.member_ids = isCreateMemberSelected(u.id) ? createGroupForm.member_ids.filter(id => id !== u.id) : [...createGroupForm.member_ids, u.id] }
function removeCreateGroupMember(id: string) { createGroupForm.member_ids = createGroupForm.member_ids.filter(v => v !== id) }
async function createGroup() { const memberIDs = createGroupForm.member_ids.filter(id => id !== currentUserId.value); if (!createGroupForm.name.trim()) { Message.warning('请输入群名称'); return false }; if (!memberIDs.length) { Message.warning('请选择成员'); return false }; const conv = await api.CreateGroup({ name: createGroupForm.name.trim(), avatar: createGroupForm.avatar.trim(), member_ids: memberIDs }); mergeConversation(conv); activeConversation.value = conv; showCreateGroup.value = false; Message.success('群聊已创建') }
function mergeConversation(c: main.ConversationView) { const idx = conversations.value.findIndex(x => x.conversation_id === c.conversation_id); if (idx >= 0) conversations.value[idx] = { ...conversations.value[idx], ...c }; else conversations.value.unshift(c) }
async function updateGroupInfo() { if (!activeConversation.value) return; const updated = await api.UpdateGroupInfo(activeConversation.value.conversation_id, { name: groupEditForm.name.trim(), avatar: groupEditForm.avatar.trim() }); mergeConversation(updated); activeConversation.value = { ...activeConversation.value, ...updated }; Message.success('群资料已更新') }
async function searchAddMemberUsers() {
  const existingIDs = new Set(members.value.map(m => m.user_id))
  addMemberCandidates.value = addMemberSearch.value ? excludeCurrentUser(await api.SearchUsers(addMemberSearch.value)).filter(u => !existingIDs.has(u.id)) : []
}
function isPendingMember(id: string) { return addMemberIds.value.includes(id) }
function togglePendingMember(u: main.UserView) { if (u.id === currentUserId.value || members.value.some(m => m.user_id === u.id)) return; addMemberLabels[u.id] = userName(u); addMemberIds.value = isPendingMember(u.id) ? addMemberIds.value.filter(id => id !== u.id) : [...addMemberIds.value, u.id] }
function removePendingMember(id: string) { addMemberIds.value = addMemberIds.value.filter(v => v !== id) }
async function addGroupMembers() { if (!activeConversation.value || !addMemberIds.value.length) return; const updated = await api.AddGroupMembers(activeConversation.value.conversation_id, addMemberIds.value); mergeConversation(updated); activeConversation.value = { ...activeConversation.value, ...updated }; addMemberIds.value = []; addMemberCandidates.value = []; addMemberSearch.value = ''; await loadMembers(); Message.success('已添加成员') }
function isSelf(m: main.MemberView) { return m.user_id === currentUserId.value }
function canGrantAdmin(m: main.MemberView) { return isGroupConversation.value && currentRole.value === 'owner' && !isSelf(m) && m.role === 'member' }
function canRevokeAdmin(m: main.MemberView) { return isGroupConversation.value && currentRole.value === 'owner' && !isSelf(m) && m.role === 'admin' }
function canTransferOwner(m: main.MemberView) { return isGroupConversation.value && currentRole.value === 'owner' && !isSelf(m) && m.role !== 'owner' }
function canRemoveMember(m: main.MemberView) { return isGroupConversation.value && !isSelf(m) && m.role !== 'owner' && (currentRole.value === 'owner' || (currentRole.value === 'admin' && m.role === 'member')) }
function grantGroupAdmin(m: main.MemberView) { if (!activeConversation.value) return; Modal.confirm({ title: '授予管理员', content: `确定将 ${memberName(m)} 设为管理员吗？`, onOk: async () => { await api.GrantGroupAdmin(activeConversation.value!.conversation_id, m.user_id); await loadMembers(); Message.success('已设为管理员') } }) }
function revokeGroupAdmin(m: main.MemberView) { if (!activeConversation.value) return; Modal.confirm({ title: '取消管理员', content: `确定取消 ${memberName(m)} 的管理员吗？`, onOk: async () => { await api.RevokeGroupAdmin(activeConversation.value!.conversation_id, m.user_id); await loadMembers(); Message.success('已取消管理员') } }) }
function transferGroupOwner(m: main.MemberView) { if (!activeConversation.value) return; Modal.confirm({ title: '转让群主', content: `确定将群主转让给 ${memberName(m)} 吗？转让后你将变为管理员。`, onOk: async () => { const updated = await api.TransferGroupOwner(activeConversation.value!.conversation_id, m.user_id); mergeConversation(updated); activeConversation.value = { ...activeConversation.value!, ...updated }; await loadMembers(); Message.success('已转让群主') } }) }
function removeGroupMember(m: main.MemberView) { if (!activeConversation.value) return; Modal.confirm({ title: '移除成员', content: `确定移除 ${memberName(m)} 吗？`, onOk: async () => { await api.RemoveGroupMember(activeConversation.value!.conversation_id, m.user_id); await loadMembers(); Message.success('已移除成员') } }) }
function leaveGroup() { if (!activeConversation.value) return; Modal.confirm({ title: '退出群聊', content: `确定退出 ${conversationTitle(activeConversation.value)} 吗？`, onOk: async () => { await api.LeaveGroup(activeConversation.value!.conversation_id); activeConversation.value = null; messages.value = []; await refreshConversations(); Message.success('已退出群聊') } }) }
function dismissGroup() { if (!activeConversation.value) return; Modal.confirm({ title: '解散群聊', content: `确定解散 ${conversationTitle(activeConversation.value)} 吗？`, onOk: async () => { await api.DismissGroup(activeConversation.value!.conversation_id); activeConversation.value = null; messages.value = []; await refreshConversations(); showMembers.value = false; Message.success('已解散群聊') } }) }

function applyReadState(cid: string, st: main.ReadStateView) { if (!cid || !st.user_id) return; readStatesByConversation[cid] = { ...(readStatesByConversation[cid] || {}), [st.user_id]: st } }
function applyReadReceipt(r: any) {
  if (!r?.conversation_id) return
  applyReadState(r.conversation_id, { user_id: r.user_id, last_read_message_id: r.last_read_message_id, updated_at: r.updated_at, email: '', avatar: '', display_name: displayNameByUserId(r.user_id) })
  const selectedMessage = selectedReadMessage.value
  if (showReadDetails.value && selectedMessage && selectedMessage.conversation_id === r.conversation_id) {
    selectedReadDetails.value = readDetailsForMessage(selectedMessage)
    readDetailsTitle.value = messageReadSummary(selectedMessage) || '已读详情'
  }
  messages.value = [...messages.value]
}
function readDetailsForMessage(m: main.MessageView) {
  if (!m.message_id || m.is_system) return []
  const ids = members.value.length ? members.value.map(x => x.user_id) : (activeConversation.value?.member_ids || [])
  return ids.filter(id => id !== m.sender_id).map(id => {
    const member = members.value.find(x => x.user_id === id)
    const state = readStatesByConversation[m.conversation_id]?.[id]
    const detail = m.read_details?.find(d => d.user_id === id)
    const last = detail?.last_read_message_id || state?.last_read_message_id || ''
    const isRead = !!detail?.is_read || isIDGTE(last, m.message_id)
    return { user_id: id, is_read: isRead, last_read_message_id: last, updated_at: detail?.updated_at || state?.updated_at || 0, email: detail?.email || member?.email || state?.email || '', avatar: detail?.avatar || member?.avatar || state?.avatar || '', display_name: safeName(detail?.display_name, member?.display_name, state?.display_name, id) }
  })
}
function messageReadSummary(m: main.MessageView) { const d = readDetailsForMessage(m); if (!d.length) return ''; const read = d.filter(x => x.is_read).length; return `已读 ${read} / 未读 ${d.length - read}` }
function openReadDetails(m: main.MessageView) { selectedReadMessage.value = m; selectedReadDetails.value = readDetailsForMessage(m); readDetailsTitle.value = messageReadSummary(m) || '已读详情'; showReadDetails.value = true }
function scheduleReadReceipt() { if (readReceiptTimer) return; readReceiptTimer = window.setTimeout(() => { readReceiptTimer = undefined; sendReadReceiptLatest() }, 250) }
function isNearMessageBottom() { const el = messageListRef.value; return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 120 }
function scheduleScrollToBottom() { if (scrollTimer) window.clearTimeout(scrollTimer); scrollTimer = window.setTimeout(async () => { await nextTick(); const el = messageListRef.value; if (el) el.scrollTop = el.scrollHeight }, 0) }
async function sendReadReceiptLatest() { if (!activeConversation.value) return; const latest = [...messages.value].reverse().find(m => m.conversation_id === activeConversation.value?.conversation_id && m.message_id && !m.is_system); if (!latest?.message_id) return; applyReadState(activeConversation.value.conversation_id, { user_id: currentUserId.value, last_read_message_id: latest.message_id, updated_at: Date.now(), email: session.value?.email || '', avatar: session.value?.avatar || '', display_name: currentAccountName.value }); try { await api.SendReadReceipt(activeConversation.value.conversation_id, latest.message_id) } catch {} }
</script>
