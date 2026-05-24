<template>
  <a-config-provider>
    <div class="shell">
      <a-layout class="layout">
        <a-layout-header class="header">
          <div class="brand">AIM Desktop</div>
          <a-space>
            <a-tag :color="connected ? 'green' : 'gray'">{{ connected ? 'WS 已连接' : '离线/未连接' }}</a-tag>
            <a-button size="small" @click="showSettings = true">设置</a-button>
          </a-space>
        </a-layout-header>
        <a-layout-content class="content">
          <div v-if="!loggedIn" class="auth-panel">
            <a-card :bordered="false" class="auth-card">
              <template #title>{{ authMode === 'login' ? '登录' : '注册' }}</template>
              <a-form :model="authForm" layout="vertical" @submit-success="submitAuth">
                <a-form-item field="email" label="邮箱"><a-input v-model="authForm.email" /></a-form-item>
                <a-form-item v-if="authMode === 'register'" field="username" label="昵称"><a-input v-model="authForm.username" /></a-form-item>
                <a-form-item field="password" label="密码"><a-input-password v-model="authForm.password" /></a-form-item>
                <a-space direction="vertical" fill>
                  <a-button html-type="submit" type="primary" long :loading="loading">{{ authMode === 'login' ? '登录' : '注册并登录' }}</a-button>
                  <a-button long @click="authMode = authMode === 'login' ? 'register' : 'login'">切换到{{ authMode === 'login' ? '注册' : '登录' }}</a-button>
                  <a-alert v-if="status" :type="statusType" :content="status" />
                </a-space>
              </a-form>
            </a-card>
          </div>

          <a-layout v-else class="chat-layout">
            <a-layout-sider :width="310" class="sider">
              <a-tabs default-active-key="chats" lazy-load>
                <a-tab-pane key="chats" title="会话">
                  <a-space direction="vertical" fill>
                    <a-button type="primary" long @click="refreshConversations">刷新会话</a-button>
                    <a-button long @click="openCreateGroup">创建群聊</a-button>
                  </a-space>
                  <a-list class="list" :bordered="false">
                    <a-list-item v-for="c in conversations" :key="c.conversation_id" class="clickable" @click="selectConversation(c)">
                      <a-list-item-meta :title="conversationTitle(c)" :description="conversationDescription(c)" />
                    </a-list-item>
                  </a-list>
                </a-tab-pane>
                <a-tab-pane key="friends" title="好友">
                  <a-input-search v-model="searchName" placeholder="搜索用户昵称/邮箱" @search="searchUsers" />
                  <a-list class="list" :bordered="false">
                    <a-list-item v-for="u in searchResults" :key="u.id">
                      <a-list-item-meta :title="userName(u)" :description="u.email" />
                      <template #actions><a-button size="mini" @click="addFriend(u.id)">添加</a-button></template>
                    </a-list-item>
                  </a-list>
                  <a-divider />
                  <a-button long @click="refreshFriends">刷新好友/申请</a-button>
                  <a-list class="list" :bordered="false">
                    <a-list-item v-for="f in friends" :key="f.friend_id">
                      <a-list-item-meta :title="friendName(f)" :description="friendDescription(f)" />
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
                  </div>
                  <a-space>
                    <a-button size="small" @click="loadHistory(true)">更早</a-button>
                    <a-button size="small" @click="showMembers = true">详情</a-button>
                  </a-space>
                </div>
                <div class="message-list">
                  <div v-for="m in messages" :key="messageKey(m)" class="message" :class="{ mine: m.sender_id === currentUserId }">
                    <div class="bubble">
                      <div class="meta">{{ senderName(m) }} · {{ formatTime(m.created_at) }} · {{ m.status || 'synced' }}</div>
                      <div>{{ m.content }}</div>
                    </div>
                  </div>
                </div>
                <div class="composer">
                  <a-textarea v-model="draft" :auto-size="{ minRows: 1, maxRows: 4 }" placeholder="输入消息" @input="notifyTyping" />
                  <a-button type="primary" :disabled="!draft.trim()" @click="send">发送</a-button>
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

      <a-modal v-model:visible="showCreateGroup" title="创建群聊" :width="520" @ok="createGroup">
        <a-form :model="createGroupForm" layout="vertical">
          <a-form-item label="群名称"><a-input v-model="createGroupForm.name" placeholder="输入群名称" /></a-form-item>
          <a-form-item label="群头像"><a-input v-model="createGroupForm.avatar" placeholder="可选头像 URL" /></a-form-item>
          <a-form-item label="添加成员">
            <a-input-search v-model="createMemberSearch" placeholder="搜索用户昵称/邮箱" @search="searchCreateGroupUsers" />
            <div class="selected-tags" v-if="createGroupForm.member_ids.length">
              <a-tag v-for="id in createGroupForm.member_ids" :key="id" closable @close="removeCreateGroupMember(id)">{{ createMemberLabels[id] || '已选择用户' }}</a-tag>
            </div>
            <a-list class="list compact" :bordered="false">
              <a-list-item v-for="u in createMemberCandidates" :key="u.id">
                <a-list-item-meta :title="userName(u)" :description="u.email" />
                <template #actions>
                  <a-button size="mini" :type="isCreateMemberSelected(u.id) ? 'primary' : 'secondary'" @click="toggleCreateGroupMember(u)">{{ isCreateMemberSelected(u.id) ? '已选' : '选择' }}</a-button>
                </template>
              </a-list-item>
            </a-list>
          </a-form-item>
        </a-form>
      </a-modal>

      <a-drawer v-model:visible="showMembers" title="会话详情" :width="420" @before-open="openMembersDrawer">
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
              <a-input-search v-model="addMemberSearch" placeholder="搜索用户昵称/邮箱" @search="searchAddMemberUsers" />
              <div class="selected-tags" v-if="addMemberIds.length">
                <a-tag v-for="id in addMemberIds" :key="id" closable @close="removePendingMember(id)">{{ addMemberLabels[id] || '已选择用户' }}</a-tag>
              </div>
              <a-list class="list compact" :bordered="false">
                <a-list-item v-for="u in addMemberCandidates" :key="u.id">
                  <a-list-item-meta :title="userName(u)" :description="u.email" />
                  <template #actions>
                    <a-button size="mini" :type="isPendingMember(u.id) ? 'primary' : 'secondary'" @click="togglePendingMember(u)">{{ isPendingMember(u.id) ? '已选' : '选择' }}</a-button>
                  </template>
                </a-list-item>
              </a-list>
              <a-button type="primary" long :disabled="!addMemberIds.length" @click="addGroupMembers">添加选中成员</a-button>
            </a-card>

            <a-card title="成员" :bordered="false">
              <a-list :bordered="false">
                <a-list-item v-for="m in members" :key="m.user_id">
                  <a-list-item-meta :title="memberName(m)" :description="memberDescription(m)" />
                  <template #actions>
                    <a-button v-if="canRemoveMember(m)" size="mini" status="danger" @click="removeGroupMember(m)">移除</a-button>
                  </template>
                </a-list-item>
              </a-list>
            </a-card>

            <a-button v-if="isGroupConversation && canManageGroup" status="danger" long @click="dismissGroup">解散群聊</a-button>
            <a-button v-else-if="isGroupConversation" status="warning" long @click="leaveGroup">退出群聊</a-button>
          </a-space>
        </template>
      </a-drawer>
    </div>
  </a-config-provider>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Message, Modal } from '@arco-design/web-vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import * as api from '../wailsjs/go/main/App'
import type { main } from '../wailsjs/go/models'

const loading = ref(false)
const connected = ref(false)
const authMode = ref<'login' | 'register'>('login')
const status = ref('')
const statusType = ref<'info' | 'success' | 'warning' | 'error'>('info')
const showSettings = ref(false)
const showMembers = ref(false)
const showCreateGroup = ref(false)
const authForm = reactive({ email: '', password: '', username: '' })
const config = reactive({ gateway_url: 'http://localhost:8888', ws_url: 'ws://localhost:8888/ws' })
const session = ref<main.SessionInfo | null>(null)
const conversations = ref<main.ConversationView[]>([])
const activeConversation = ref<main.ConversationView | null>(null)
const messages = ref<main.MessageView[]>([])
const friends = ref<main.FriendView[]>([])
const searchName = ref('')
const searchResults = ref<main.UserView[]>([])
const members = ref<main.MemberView[]>([])
const draft = ref('')
const currentUserId = computed(() => session.value?.user_id || '')
const loggedIn = computed(() => !!session.value?.access_token)
const isGroupConversation = computed(() => activeConversation.value?.conversation_type === 'group')
const canManageGroup = computed(() => !!activeConversation.value && activeConversation.value.creator_id === currentUserId.value)

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
  try {
    Object.assign(config, await api.GetConfig())
    const auto = await api.AutoLogin()
    if (auto?.access_token) {
      session.value = auto
      await loadCachedData()
      await refreshConversations()
      await refreshFriends()
    }
  } catch (e) {
    console.warn(e)
  }
  EventsOn('ws:connection', (payload: any) => { connected.value = !!payload?.connected })
  EventsOn('ws:message', (payload: main.MessageView) => mergeMessage(payload))
  EventsOn('ws:server-ack', (payload: any) => {
    const msg = messages.value.find(m => m.client_msg_id === payload.client_msg_id)
    if (msg) {
      msg.status = payload.status === 1 ? 'accepted' : 'rejected'
      if (payload.message_id) msg.message_id = payload.message_id
    }
  })
  EventsOn('ws:token-expired', async () => { await refreshToken() })
})

async function submitAuth() {
  loading.value = true
  status.value = ''
  try {
    if (authMode.value === 'register') {
      await api.Register({ email: authForm.email, password: authForm.password, username: authForm.username, avatar: '' })
    }
    session.value = await api.Login({ email: authForm.email, password: authForm.password })
    statusType.value = 'success'; status.value = '登录成功'
    await refreshConversations(); await refreshFriends()
  } catch (e: any) {
    statusType.value = 'error'; status.value = String(e)
  } finally { loading.value = false }
}
async function saveSettings() { await api.SaveConfig(config); Message.success('已保存') }
async function refreshToken() { try { session.value = await api.RefreshToken() } catch { session.value = null } }
async function loadCachedData() {
  try { conversations.value = await api.GetCachedConversations() } catch {}
  try { friends.value = await api.GetCachedFriends() } catch {}
}
async function refreshConversations() {
  try { conversations.value = await api.ListConversations() }
  catch { conversations.value = await api.GetCachedConversations() }
}
async function refreshFriends() {
  try { friends.value = await api.ListFriends() }
  catch { friends.value = await api.GetCachedFriends() }
}
async function searchUsers() { searchResults.value = searchName.value ? await api.SearchUsers(searchName.value) : [] }
async function addFriend(id: string) { await api.AddFriend(id); Message.success('已发送申请') }
async function selectConversation(c: main.ConversationView) { activeConversation.value = c; messages.value = await api.GetCachedMessages(c.conversation_id, 80); await loadHistory(false) }
async function loadHistory(older: boolean) {
  if (!activeConversation.value) return
  const oldest = older && messages.value.length ? messages.value[0] : null
  const res = await api.GetConversationHistory(activeConversation.value.conversation_id, oldest?.created_at || 0, oldest?.message_id || '', 50)
  res.messages?.forEach(mergeMessage)
}
async function openMembersDrawer() {
  if (!activeConversation.value) return
  groupEditForm.name = activeConversation.value.name || activeConversation.value.display_name || ''
  groupEditForm.avatar = activeConversation.value.avatar || ''
  await loadMembers()
}
async function loadMembers() {
  if (!activeConversation.value) return
  try { members.value = await api.GetCachedConversationMembers(activeConversation.value.conversation_id) } catch {}
  try { members.value = await api.GetConversationMembers(activeConversation.value.conversation_id) } catch (e) { if (!members.value.length) Message.error(String(e)) }
}
async function send() {
  if (!activeConversation.value || !draft.value.trim()) return
  const content = draft.value; draft.value = ''
  const msg = await api.SendMessage(activeConversation.value.conversation_id, 'text', content, [])
  mergeMessage(msg)
}
let lastTyping = 0
function notifyTyping() { const now = Date.now(); if (activeConversation.value && now - lastTyping > 1500) { lastTyping = now; api.SendTyping(activeConversation.value.conversation_id).catch(() => {}) } }
function mergeMessage(m: main.MessageView) {
  const idx = messages.value.findIndex(x => (m.message_id && x.message_id === m.message_id) || (m.client_msg_id && x.client_msg_id === m.client_msg_id))
  if (idx >= 0) messages.value[idx] = { ...messages.value[idx], ...m }
  else messages.value.push(m)
  messages.value.sort(compareMessages)
}
function compareMessages(a: main.MessageView, b: main.MessageView) { return (a.created_at || 0) - (b.created_at || 0) || compareSnowflakeID(a.message_id, b.message_id) }
function compareSnowflakeID(a?: string, b?: string) {
  if (!a && !b) return 0
  if (!a) return -1
  if (!b) return 1
  try {
    const av = BigInt(a), bv = BigInt(b)
    return av < bv ? -1 : av > bv ? 1 : 0
  } catch { return a.localeCompare(b) }
}
function messageKey(m: main.MessageView) { return m.message_id || m.client_msg_id }
function formatTime(ts?: number) { return ts ? new Date(ts > 1e12 ? ts : ts * 1000).toLocaleString() : '' }

function safeName(...names: Array<string | undefined>) { return names.find(v => v && v.trim())?.trim() || '' }
function userName(u: main.UserView) { return safeName(u.display_name, u.nickname, u.email) || '未知用户' }
function friendName(f: main.FriendView) { return safeName(f.display_name, f.email) || '未知用户' }
function memberName(m: main.MemberView) { return safeName(m.display_name, m.email) || '未知用户' }
function senderName(m: main.MessageView) { return safeName(m.sender_info?.display_name, m.sender_info?.name, m.sender_info?.email) || '未知用户' }
function conversationTitle(c: main.ConversationView) { return safeName(c.display_name, c.name) || '未命名会话' }
function conversationDescription(c: main.ConversationView) { return c.conversation_type === 'group' ? '群聊' : c.conversation_type === 'direct' ? '单聊' : '会话' }
function friendDescription(f: main.FriendView) { return `${friendStatusText(f.status)}${f.email ? ' · ' + f.email : ''}` }
function friendStatusText(status?: string) { return status === 'accepted' ? '已添加' : status === 'pending' ? '待确认' : status || '好友' }
function memberDescription(m: main.MemberView) { return m.role || '成员' }

function openCreateGroup() {
  showCreateGroup.value = true
  createGroupForm.name = ''
  createGroupForm.avatar = ''
  createGroupForm.member_ids = []
  createMemberSearch.value = ''
  createMemberCandidates.value = []
}
async function searchCreateGroupUsers() { createMemberCandidates.value = createMemberSearch.value ? await api.SearchUsers(createMemberSearch.value) : [] }
function isCreateMemberSelected(id: string) { return createGroupForm.member_ids.includes(id) }
function toggleCreateGroupMember(u: main.UserView) {
  createMemberLabels[u.id] = userName(u)
  createGroupForm.member_ids = isCreateMemberSelected(u.id) ? createGroupForm.member_ids.filter(id => id !== u.id) : [...createGroupForm.member_ids, u.id]
}
function removeCreateGroupMember(id: string) { createGroupForm.member_ids = createGroupForm.member_ids.filter(v => v !== id) }
async function createGroup() {
  if (!createGroupForm.name.trim()) { Message.warning('请输入群名称'); return false }
  if (!createGroupForm.member_ids.length) { Message.warning('请选择成员'); return false }
  const conv = await api.CreateGroup({ name: createGroupForm.name.trim(), avatar: createGroupForm.avatar.trim(), member_ids: createGroupForm.member_ids })
  mergeConversation(conv)
  activeConversation.value = conv
  Message.success('群聊已创建')
}
function mergeConversation(c: main.ConversationView) {
  const idx = conversations.value.findIndex(x => x.conversation_id === c.conversation_id)
  if (idx >= 0) conversations.value[idx] = { ...conversations.value[idx], ...c }
  else conversations.value.unshift(c)
}
async function updateGroupInfo() {
  if (!activeConversation.value) return
  const updated = await api.UpdateGroupInfo(activeConversation.value.conversation_id, { name: groupEditForm.name.trim(), avatar: groupEditForm.avatar.trim() })
  mergeConversation(updated)
  activeConversation.value = { ...activeConversation.value, ...updated }
  Message.success('群资料已更新')
}
async function searchAddMemberUsers() { addMemberCandidates.value = addMemberSearch.value ? await api.SearchUsers(addMemberSearch.value) : [] }
function isPendingMember(id: string) { return addMemberIds.value.includes(id) }
function togglePendingMember(u: main.UserView) {
  addMemberLabels[u.id] = userName(u)
  addMemberIds.value = isPendingMember(u.id) ? addMemberIds.value.filter(id => id !== u.id) : [...addMemberIds.value, u.id]
}
function removePendingMember(id: string) { addMemberIds.value = addMemberIds.value.filter(v => v !== id) }
async function addGroupMembers() {
  if (!activeConversation.value || !addMemberIds.value.length) return
  const updated = await api.AddGroupMembers(activeConversation.value.conversation_id, addMemberIds.value)
  mergeConversation(updated)
  activeConversation.value = { ...activeConversation.value, ...updated }
  addMemberIds.value = []
  addMemberCandidates.value = []
  addMemberSearch.value = ''
  await loadMembers()
  Message.success('已添加成员')
}
function canRemoveMember(m: main.MemberView) { return isGroupConversation.value && canManageGroup.value && m.user_id !== currentUserId.value && m.role !== 'owner' && m.role !== 'creator' }
function removeGroupMember(m: main.MemberView) {
  if (!activeConversation.value) return
  Modal.confirm({ title: '移除成员', content: `确定移除 ${memberName(m)} 吗？`, onOk: async () => { await api.RemoveGroupMember(activeConversation.value!.conversation_id, m.user_id); await loadMembers(); Message.success('已移除成员') } })
}
function leaveGroup() {
  if (!activeConversation.value) return
  Modal.confirm({ title: '退出群聊', content: `确定退出 ${conversationTitle(activeConversation.value)} 吗？`, onOk: async () => { await api.LeaveGroup(activeConversation.value!.conversation_id); activeConversation.value = null; messages.value = []; await refreshConversations(); Message.success('已退出群聊') } })
}
function dismissGroup() {
  if (!activeConversation.value) return
  Modal.confirm({ title: '解散群聊', content: `确定解散 ${conversationTitle(activeConversation.value)} 吗？`, onOk: async () => { await api.DismissGroup(activeConversation.value!.conversation_id); activeConversation.value = null; messages.value = []; await refreshConversations(); showMembers.value = false; Message.success('已解散群聊') } })
}
</script>
