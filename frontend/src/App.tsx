import {
  BellIcon,
  LogOutIcon,
  MessageSquareIcon,
  MoonIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  SunIcon,
  UsersIcon,
  WifiIcon,
  WifiOffIcon,
} from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { AuthView } from "@/components/auth-view"
import { ContactsPane } from "@/components/contacts-pane"
import {
  CreateConversationDialog,
  GroupSettingsDialog,
} from "@/components/group-dialogs"
import { MessagePane } from "@/components/message-pane"
import { SearchDialog } from "@/components/search-dialog"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  aimApi,
  getSession,
  refreshSessionToken,
  subscribeSession,
} from "@/lib/api"
import type {
  PushMessagePayload,
  PushPresencePayload,
  PushReadReceiptPayload,
  PushTypingPayload,
} from "@/lib/proto"
import type {
  AttachmentContentV1,
  AuthSession,
  ChatMessage,
  ConversationItem,
  EntityId,
  FriendTagItem,
  FriendshipItem,
  MessageItem,
  ReadStateItem,
  SystemNotificationItem,
  UserInfo,
} from "@/lib/types"
import {
  cn,
  compareId,
  formatTimestamp,
  hasId,
  initialsFromName,
  maxId,
} from "@/lib/utils"
import { type WsConnectionStatus, aimWsClient } from "@/lib/ws-client"

interface HistoryCursor {
  createdAt: number
  id: EntityId
  hasMore: boolean
}

function mergeMessages(
  existing: ChatMessage[],
  incoming: ChatMessage[]
): ChatMessage[] {
  const byId = new Map<EntityId, ChatMessage>()
  const byClientId = new Map<string, ChatMessage>()

  for (const item of existing) {
    if (hasId(item.id)) byId.set(item.id, item)
    if (item.client_msg_id) byClientId.set(item.client_msg_id, item)
  }

  for (const msg of incoming) {
    const prevByClient = msg.client_msg_id
      ? byClientId.get(msg.client_msg_id)
      : undefined
    const prevById = hasId(msg.id) ? byId.get(msg.id) : undefined
    const merged: ChatMessage = {
      ...(prevByClient ?? prevById ?? msg),
      ...msg,
      local_status:
        msg.local_status === "sending" || msg.local_status === "failed"
          ? msg.local_status
          : prevByClient?.local_status === "sending"
            ? "sent"
            : msg.local_status,
    }
    if (hasId(merged.id)) {
      byId.set(merged.id, merged)
    }
    if (merged.client_msg_id) {
      byClientId.set(merged.client_msg_id, merged)
    }
  }

  const combined = new Map<string, ChatMessage>()
  for (const item of byId.values()) {
    combined.set(`id:${item.id}`, item)
  }
  for (const item of byClientId.values()) {
    if (hasId(item.id) && byId.has(item.id)) continue
    combined.set(`client:${item.client_msg_id}`, item)
  }

  return Array.from(combined.values()).sort((a, b) => {
    if (a.created_at !== b.created_at) return a.created_at - b.created_at
    return compareId(a.id, b.id)
  })
}

function toChatMessage(item: MessageItem): ChatMessage {
  return {
    ...item,
    local_status: "received",
  }
}

export function App() {
  const [session, setSessionState] = useState<AuthSession | null>(() =>
    getSession()
  )
  const [currentUser, setCurrentUser] = useState<UserInfo | null>(null)
  const [wsStatus, setWsStatus] = useState<WsConnectionStatus>("disconnected")
  const [activeNav, setActiveNav] = useState<"chat" | "contacts">("chat")
  const [darkMode, setDarkMode] = useState(false)

  const [conversations, setConversations] = useState<ConversationItem[]>([])
  const [convFilter, setConvFilter] = useState("")
  const [activeConvId, setActiveConvId] = useState<EntityId | null>(null)
  const [messagesByConv, setMessagesByConv] = useState<
    Record<EntityId, ChatMessage[]>
  >({})
  const [cursorsByConv, setCursorsByConv] = useState<
    Record<EntityId, HistoryCursor>
  >({})
  const [readStatesByConv, setReadStatesByConv] = useState<
    Record<EntityId, ReadStateItem[]>
  >({})
  const [loadingHistoryConvId, setLoadingHistoryConvId] = useState<
    EntityId | null
  >(null)

  const [friends, setFriends] = useState<FriendshipItem[]>([])
  const [applications, setApplications] = useState<FriendshipItem[]>([])
  const [friendTags, setFriendTags] = useState<FriendTagItem[]>([])
  const [presenceMap, setPresenceMap] = useState<Record<EntityId, string>>({})
  const [typingByConv, setTypingByConv] = useState<Record<EntityId, EntityId[]>>({})
  const [notifications, setNotifications] = useState<SystemNotificationItem[]>(
    []
  )

  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [groupSettingsOpen, setGroupSettingsOpen] = useState(false)
  const [searchDialogOpen, setSearchDialogOpen] = useState(false)
  const [searchScoped, setSearchScoped] = useState(false)

  const activeConvIdRef = useRef<EntityId | null>(null)
  activeConvIdRef.current = activeConvId

  useEffect(() => {
    return subscribeSession((next) => {
      setSessionState(next)
      if (!next) {
        aimWsClient.disconnect()
        setCurrentUser(null)
        setConversations([])
        setMessagesByConv({})
        setReadStatesByConv({})
        setFriends([])
        setApplications([])
      }
    })
  }, [])

  useEffect(() => {
    document.documentElement.classList.toggle("dark", darkMode)
  }, [darkMode])

  const refreshContactsData = useCallback(async () => {
    if (!getSession()) return
    const [friendsRes, appsRes, tagsRes, presenceRes] =
      await Promise.allSettled([
        aimApi.listFriends(),
        aimApi.listFriendApplications(),
        aimApi.listFriendTags(),
        aimApi.getFriendsPresence(),
      ])
    if (friendsRes.status === "fulfilled") setFriends(friendsRes.value)
    if (appsRes.status === "fulfilled") setApplications(appsRes.value)
    if (tagsRes.status === "fulfilled") setFriendTags(tagsRes.value)
    if (presenceRes.status === "fulfilled") {
      const nextPresence: Record<EntityId, string> = {}
      for (const item of presenceRes.value) {
        nextPresence[item.user_id] = item.status
      }
      setPresenceMap(nextPresence)
    }
  }, [])

  const fetchConversationSnapshot = useCallback(
    async (conversationId: EntityId) => {
      const res = await aimApi.getConversationHistory({
        conversationId,
        limit: 50,
      })
      const incoming = (res.messages ?? []).map(toChatMessage)
      setMessagesByConv((prev) => ({
        ...prev,
        [conversationId]: mergeMessages(prev[conversationId] ?? [], incoming),
      }))
      setCursorsByConv((prev) => ({
        ...prev,
        [conversationId]: {
          createdAt: res.next_cursor_created_at,
          id: res.next_cursor_id,
          hasMore: Boolean(res.has_more),
        },
      }))
      if (res.read_states) {
        setReadStatesByConv((prev) => ({
          ...prev,
          [conversationId]: res.read_states,
        }))
      }
      return incoming
    },
    []
  )

  const syncFullWorkspace = useCallback(async () => {
    const active = getSession()
    if (!active) return
    aimApi
      .getUserById(active.userId)
      .then(setCurrentUser)
      .catch(() => { })

    try {
      const convList = await aimApi.listConversations()
      setConversations(convList)
      const firstId = convList[0]?.conversation_id ?? null
      setActiveConvId((prev) => prev ?? firstId)

      await Promise.allSettled(
        convList
          .slice(0, 12)
          .map((c) => fetchConversationSnapshot(c.conversation_id))
      )
      await refreshContactsData()
    } catch { }
  }, [fetchConversationSnapshot, refreshContactsData])

  useEffect(() => {
    if (!session) return
    const nowSec = Math.floor(Date.now() / 1000)
    if (session.expiresAt > 0 && session.expiresAt <= nowSec + 15) {
      void refreshSessionToken()
      return
    }
    aimWsClient.connect()
    void syncFullWorkspace()
  }, [session?.userId, syncFullWorkspace])

  useEffect(() => {
    if (!session) return
    const nowSec = Math.floor(Date.now() / 1000)
    const delayMs = Math.max(5000, (session.expiresAt - nowSec - 60) * 1000)
    const timer = setTimeout(() => {
      void refreshSessionToken()
    }, delayMs)
    return () => clearTimeout(timer)
  }, [session?.accessToken, session?.expiresAt])

  useEffect(() => {
    function handlePushMessage(payload: PushMessagePayload) {
      const isAttachmentParseUpdate =
        payload.is_system &&
        !hasId(payload.sender_id) &&
        (payload.message_type === "image" ||
          payload.message_type === "video" ||
          payload.message_type === "audio")

      if (isAttachmentParseUpdate) {
        try {
          const parsed = JSON.parse(payload.content) as AttachmentContentV1
          if (!parsed.file_id) return
          setMessagesByConv((prev) => {
            const list = prev[payload.conversation_id]
            if (!list) return prev
            const updated = list.map((msg) => {
              try {
                const existing = JSON.parse(msg.content) as AttachmentContentV1
                if (existing.file_id === parsed.file_id) {
                  return { ...msg, content: payload.content }
                }
              } catch { }
              return msg
            })
            return { ...prev, [payload.conversation_id]: updated }
          })
        } catch { }
        return
      }

      const myId = getSession()?.userId ?? "0"
      const incomingMsg: ChatMessage = {
        id: payload.message_id,
        conversation_id: payload.conversation_id,
        sender_id: payload.sender_id,
        sender_info: payload.sender_info ?? { name: "", email: "" },
        message_type: payload.message_type,
        content: payload.content,
        client_msg_id: payload.client_msg_id,
        created_at: payload.sent_at || Date.now(),
        is_system: payload.is_system,
        mentions: payload.mentions ?? [],
        read_details: [],
        local_status: payload.sender_id === myId ? "sent" : "received",
      }

      setMessagesByConv((prev) => ({
        ...prev,
        [payload.conversation_id]: mergeMessages(
          prev[payload.conversation_id] ?? [],
          [incomingMsg]
        ),
      }))

      if (
        payload.is_system ||
        !conversations.some(
          (c) => c.conversation_id === payload.conversation_id
        )
      ) {
        aimApi
          .listConversations()
          .then(setConversations)
          .catch(() => { })
      }

      if (
        activeConvIdRef.current === payload.conversation_id &&
        hasId(payload.message_id) &&
        payload.sender_id !== myId
      ) {
        aimWsClient.sendReadReceipt(payload.conversation_id, payload.message_id)
        setReadStatesByConv((prev) => {
          const current = prev[payload.conversation_id] ?? []
          const filtered = current.filter((r) => r.user_id !== myId)
          return {
            ...prev,
            [payload.conversation_id]: [
              ...filtered,
              {
                user_id: myId,
                last_read_message_id: payload.message_id,
                updated_at: Date.now(),
              },
            ],
          }
        })
      }
    }

    function handlePushPresence(payload: PushPresencePayload) {
      setPresenceMap((prev) => ({
        ...prev,
        [payload.user_id]: payload.status,
      }))
    }

    function handlePushTyping(payload: PushTypingPayload) {
      const myId = getSession()?.userId ?? "0"
      if (payload.user_id === myId) return
      setTypingByConv((prev) => {
        const list = prev[payload.conversation_id] ?? []
        if (list.includes(payload.user_id)) return prev
        return {
          ...prev,
          [payload.conversation_id]: [...list, payload.user_id],
        }
      })
      setTimeout(() => {
        setTypingByConv((prev) => {
          const list = prev[payload.conversation_id] ?? []
          return {
            ...prev,
            [payload.conversation_id]: list.filter(
              (id) => id !== payload.user_id
            ),
          }
        })
      }, 4000)
    }

    function handlePushReadReceipt(payload: PushReadReceiptPayload) {
      setReadStatesByConv((prev) => {
        const list = prev[payload.conversation_id] ?? []
        const existing = list.find((r) => r.user_id === payload.user_id)
        const nextLastRead = maxId(
          existing?.last_read_message_id ?? "0",
          payload.last_read_message_id
        )
        const nextItem: ReadStateItem = {
          ...existing,
          user_id: payload.user_id,
          last_read_message_id: nextLastRead,
          updated_at: Math.max(existing?.updated_at ?? 0, payload.updated_at),
        }
        return {
          ...prev,
          [payload.conversation_id]: [
            ...list.filter((r) => r.user_id !== payload.user_id),
            nextItem,
          ],
        }
      })
    }

    aimWsClient.setHandlers({
      onStatusChange: setWsStatus,
      onReconnected: () => {
        void syncFullWorkspace()
      },
      onPushMessage: handlePushMessage,
      onPushPresence: handlePushPresence,
      onPushTyping: handlePushTyping,
      onPushReadReceipt: handlePushReadReceipt,
      onPushFriendApplication: () => {
        void refreshContactsData()
      },
      onPushNotification: (notif) => {
        setNotifications((prev) => [
          {
            id: `${Date.now()}-${Math.random()}`,
            ...notif,
            received_at: Date.now(),
          },
          ...prev.slice(0, 29),
        ])
      },
    })
  }, [conversations, refreshContactsData, syncFullWorkspace])

  useEffect(() => {
    if (!activeConvId || !session) return
    const list = messagesByConv[activeConvId]
    if (!list) {
      setLoadingHistoryConvId(activeConvId)
      fetchConversationSnapshot(activeConvId)
        .catch(() => { })
        .finally(() => setLoadingHistoryConvId(null))
      return
    }
    const maxMsgId = list.reduce(
      (acc, m) => (compareId(m.id, acc) > 0 ? m.id : acc),
      "0"
    )
    const myRead =
      readStatesByConv[activeConvId]?.find((r) => r.user_id === session.userId)
        ?.last_read_message_id ?? "0"
    if (compareId(maxMsgId, myRead) > 0) {
      aimWsClient.sendReadReceipt(activeConvId, maxMsgId)
      setReadStatesByConv((prev) => {
        const current = prev[activeConvId] ?? []
        return {
          ...prev,
          [activeConvId]: [
            ...current.filter((r) => r.user_id !== session.userId),
            {
              user_id: session.userId,
              last_read_message_id: maxMsgId,
              updated_at: Date.now(),
            },
          ],
        }
      })
    }
  }, [
    activeConvId,
    fetchConversationSnapshot,
    messagesByConv,
    readStatesByConv,
    session,
  ])

  const unreadByConv = useMemo(() => {
    const out: Record<EntityId, number> = {}
    if (!session) return out
    for (const conv of conversations) {
      const msgs = messagesByConv[conv.conversation_id] ?? []
      const myRead =
        readStatesByConv[conv.conversation_id]?.find(
          (r) => r.user_id === session.userId
        )?.last_read_message_id ?? "0"
      out[conv.conversation_id] = msgs.filter(
        (m) => compareId(m.id, myRead) > 0 && m.sender_id !== session.userId
      ).length
    }
    return out
  }, [conversations, messagesByConv, readStatesByConv, session])

  const totalUnread = useMemo(
    () => Object.values(unreadByConv).reduce((a, b) => a + b, 0),
    [unreadByConv]
  )

  const pendingFriendAppsCount = useMemo(() => {
    if (!session) return 0
    return applications.filter(
      (a) => a.status === "pending" && a.friend_id === session.userId
    ).length
  }, [applications, session])

  if (!session) {
    return <AuthView onAuthed={setSessionState} />
  }

  const activeConversation =
    conversations.find((c) => c.conversation_id === activeConvId) ?? null

  const filteredConversations = conversations.filter((c) => {
    if (!convFilter.trim()) return true
    const q = convFilter.trim().toLowerCase()
    return (
      (c.name || "").toLowerCase().includes(q) ||
      String(c.conversation_id).includes(q)
    )
  })

  async function handleLoadOlderHistory() {
    if (!activeConvId) return
    const cursor = cursorsByConv[activeConvId]
    if (!cursor?.hasMore) return
    setLoadingHistoryConvId(activeConvId)
    try {
      const res = await aimApi.getConversationHistory({
        conversationId: activeConvId,
        cursorCreatedAt: cursor.createdAt,
        cursorId: cursor.id,
        limit: 50,
      })
      const older = (res.messages ?? []).map(toChatMessage)
      setMessagesByConv((prev) => ({
        ...prev,
        [activeConvId]: mergeMessages(prev[activeConvId] ?? [], older),
      }))
      setCursorsByConv((prev) => ({
        ...prev,
        [activeConvId]: {
          createdAt: res.next_cursor_created_at,
          id: res.next_cursor_id,
          hasMore: Boolean(res.has_more),
        },
      }))
    } finally {
      setLoadingHistoryConvId(null)
    }
  }

  async function handleSendMessage(input: {
    messageType: string
    content: string
    mentions: string[]
    retryClientMsgId?: string
  }) {
    if (!activeConvId || !session) return
    const targetConvId = activeConvId
    const clientMsgId =
      input.retryClientMsgId ?? `d1-${crypto.randomUUID()}`

    const optimistic: ChatMessage = {
      id: "0",
      conversation_id: targetConvId,
      sender_id: session.userId,
      sender_info: {
        name: currentUser?.nickname || session.email || `用户 #${session.userId}`,
        email: currentUser?.email || session.email || "",
      },
      message_type: input.messageType,
      content: input.content,
      client_msg_id: clientMsgId,
      created_at: Date.now(),
      mentions: input.mentions,
      read_details: [],
      local_status: "sending",
    }

    setMessagesByConv((prev) => ({
      ...prev,
      [targetConvId]: mergeMessages(prev[targetConvId] ?? [], [optimistic]),
    }))

    try {
      const ack = await aimWsClient.sendChatMessage({
        conversation_id: targetConvId,
        message_type: input.messageType,
        content: input.content,
        client_msg_id: clientMsgId,
        mentions: input.mentions,
      })
      setMessagesByConv((prev) => ({
        ...prev,
        [targetConvId]: mergeMessages(prev[targetConvId] ?? [], [
          {
            ...optimistic,
            id: hasId(ack.message_id) ? ack.message_id : optimistic.id,
            local_status: "sent",
          },
        ]),
      }))
    } catch (err) {
      setMessagesByConv((prev) => ({
        ...prev,
        [targetConvId]: mergeMessages(prev[targetConvId] ?? [], [
          {
            ...optimistic,
            local_status: "failed",
            error_msg: err instanceof Error ? err.message : "发送失败",
          },
        ]),
      }))
    }
  }

  async function handleStartDirectChat(friendUserId: EntityId, friendName?: string) {
    const existing = conversations.find(
      (c) =>
        c.conversation_type === "direct" &&
        c.member_ids?.includes(friendUserId)
    )
    if (existing) {
      setActiveConvId(existing.conversation_id)
      setActiveNav("chat")
      return
    }
    const created = await aimApi.createConversation({
      conversation_type: "direct",
      member_ids: [friendUserId],
      name: friendName,
    })
    setConversations((prev) => [
      created,
      ...prev.filter((c) => c.conversation_id !== created.conversation_id),
    ])
    setActiveConvId(created.conversation_id)
    setActiveNav("chat")
  }

  return (
    <div className="h-screen w-screen flex overflow-hidden bg-background text-foreground">
      <aside className="w-16 border-r bg-muted/40 flex flex-col items-center justify-between py-4 shrink-0">
        <div className="flex flex-col items-center gap-3">
          <Popover>
            <PopoverTrigger asChild>
              <button type="button" className="relative rounded-full focus:outline-none">
                <Avatar className="size-10 border">
                  <AvatarImage src={currentUser?.avatar} />
                  <AvatarFallback>
                    {initialsFromName(
                      currentUser?.nickname || session.email || String(session.userId)
                    )}
                  </AvatarFallback>
                </Avatar>
                <span
                  className={cn(
                    "absolute bottom-0 right-0 size-2.5 rounded-full ring-2 ring-background",
                    wsStatus === "connected"
                      ? "bg-emerald-500"
                      : wsStatus === "connecting" || wsStatus === "reconnecting"
                        ? "bg-amber-500"
                        : "bg-destructive"
                  )}
                />
              </button>
            </PopoverTrigger>
            <PopoverContent side="right" align="start" className="w-64 space-y-3">
              <div className="space-y-1">
                <div className="text-sm font-semibold">
                  {currentUser?.nickname || `用户 #${session.userId}`}
                </div>
                <div className="text-xs text-muted-foreground">
                  UID: {session.userId} · {currentUser?.email || session.email}
                </div>
                <div className="text-[11px] text-muted-foreground truncate">
                  设备: {session.deviceId}
                </div>
              </div>
              <div className="flex items-center justify-between pt-2 border-t">
                <Badge
                  variant={wsStatus === "connected" ? "default" : "secondary"}
                  className="text-[10px]"
                >
                  WS: {wsStatus}
                </Badge>
                <Button
                  size="sm"
                  variant="destructive"
                  className="h-7 text-xs"
                  onClick={() => void aimApi.logout()}
                >
                  <LogOutIcon className="size-3.5" />
                  退出登录
                </Button>
              </div>
            </PopoverContent>
          </Popover>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant={activeNav === "chat" ? "default" : "ghost"}
                size="icon"
                className="relative"
                onClick={() => setActiveNav("chat")}
              >
                <MessageSquareIcon className="size-4" />
                {totalUnread > 0 ? (
                  <span className="absolute -top-1 -right-1 min-w-4 h-4 rounded-full bg-destructive text-destructive-foreground text-[10px] px-1 flex items-center justify-center">
                    {totalUnread > 99 ? "99+" : totalUnread}
                  </span>
                ) : null}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">消息会话</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant={activeNav === "contacts" ? "default" : "ghost"}
                size="icon"
                className="relative"
                onClick={() => setActiveNav("contacts")}
              >
                <UsersIcon className="size-4" />
                {pendingFriendAppsCount > 0 ? (
                  <span className="absolute -top-1 -right-1 min-w-4 h-4 rounded-full bg-destructive text-destructive-foreground text-[10px] px-1 flex items-center justify-center">
                    {pendingFriendAppsCount}
                  </span>
                ) : null}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">通讯录与好友</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => {
                  setSearchScoped(false)
                  setSearchDialogOpen(true)
                }}
              >
                <SearchIcon className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">全局搜索</TooltipContent>
          </Tooltip>
        </div>

        <div className="flex flex-col items-center gap-2">
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="ghost" size="icon" className="relative">
                <BellIcon className="size-4" />
                {notifications.length > 0 ? (
                  <span className="absolute top-1.5 right-1.5 size-2 rounded-full bg-primary" />
                ) : null}
              </Button>
            </PopoverTrigger>
            <PopoverContent side="right" align="end" className="w-72 p-3 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold">系统通知</span>
                {notifications.length > 0 ? (
                  <button
                    type="button"
                    className="text-[11px] text-muted-foreground hover:text-foreground"
                    onClick={() => setNotifications([])}
                  >
                    清空
                  </button>
                ) : null}
              </div>
              {notifications.length === 0 ? (
                <div className="py-6 text-center text-xs text-muted-foreground">
                  暂无系统通知
                </div>
              ) : (
                <div className="max-h-60 overflow-y-auto space-y-2">
                  {notifications.map((n) => (
                    <div key={n.id} className="rounded-md border p-2 text-xs space-y-1">
                      <div className="font-medium">{n.title || n.notification_type}</div>
                      <div className="text-muted-foreground break-words">{n.body}</div>
                      <div className="text-[10px] text-muted-foreground">
                        {formatTimestamp(n.received_at)}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </PopoverContent>
          </Popover>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setDarkMode((prev) => !prev)}
              >
                {darkMode ? (
                  <SunIcon className="size-4" />
                ) : (
                  <MoonIcon className="size-4" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="right">切换主题</TooltipContent>
          </Tooltip>
        </div>
      </aside>

      {activeNav === "contacts" ? (
        <ContactsPane
          currentUserId={session.userId}
          friends={friends}
          applications={applications}
          tags={friendTags}
          presenceMap={presenceMap}
          onRefreshContacts={refreshContactsData}
          onStartDirectChat={handleStartDirectChat}
        />
      ) : (
        <div className="flex-1 flex h-full overflow-hidden">
          <div className="w-72 border-r flex flex-col bg-card shrink-0">
            <div className="h-14 border-b px-3.5 flex items-center justify-between gap-2 shrink-0">
              <div className="flex items-center gap-2 min-w-0">
                <span className="text-sm font-semibold">会话</span>
                <span
                  className="inline-flex items-center gap-1 text-[11px] text-muted-foreground"
                  title={`WebSocket: ${wsStatus}`}
                >
                  {wsStatus === "connected" ? (
                    <WifiIcon className="size-3 text-emerald-500" />
                  ) : (
                    <WifiOffIcon className="size-3 text-amber-500" />
                  )}
                </span>
              </div>
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  onClick={() => void syncFullWorkspace()}
                  title="同步会话与状态"
                >
                  <RefreshCwIcon className="size-3.5" />
                </Button>
                <Button
                  variant="outline"
                  size="icon"
                  className="size-7"
                  onClick={() => setCreateDialogOpen(true)}
                  title="新建单聊/群聊"
                >
                  <PlusIcon className="size-3.5" />
                </Button>
              </div>
            </div>

            <div className="p-2.5 border-b">
              <Input
                value={convFilter}
                onChange={(e) => setConvFilter(e.target.value)}
                placeholder="筛选会话..."
                className="h-8 text-xs"
              />
            </div>

            <ScrollArea className="flex-1">
              {filteredConversations.length === 0 ? (
                <div className="p-6 text-center text-xs text-muted-foreground">
                  暂无会话，点击右上角 + 发起聊天
                </div>
              ) : (
                <div className="p-2 space-y-1">
                  {filteredConversations.map((conv) => {
                    const msgs = messagesByConv[conv.conversation_id] ?? []
                    const lastMsg = msgs[msgs.length - 1]
                    const unread = unreadByConv[conv.conversation_id] ?? 0
                    const isSelected = conv.conversation_id === activeConvId
                    const isGroup = conv.conversation_type === "group"
                    const peerId = !isGroup
                      ? conv.member_ids?.find((id) => id !== session.userId)
                      : undefined
                    const peerOnline = peerId
                      ? presenceMap[peerId] === "online"
                      : false
                    const title =
                      conv.name ||
                      (isGroup
                        ? `群聊 #${conv.conversation_id}`
                        : `单聊 #${peerId ?? conv.conversation_id}`)

                    let preview = "暂无消息"
                    if (lastMsg) {
                      if (
                        lastMsg.message_type === "image" ||
                        lastMsg.message_type === "video" ||
                        lastMsg.message_type === "audio" ||
                        lastMsg.message_type === "file"
                      ) {
                        preview = `[附件: ${lastMsg.message_type}]`
                      } else {
                        preview = lastMsg.content
                      }
                    }

                    return (
                      <button
                        key={conv.conversation_id}
                        type="button"
                        onClick={() => setActiveConvId(conv.conversation_id)}
                        className={cn(
                          "w-full flex items-center gap-3 rounded-lg px-2.5 py-2 text-left transition-colors",
                          isSelected
                            ? "bg-accent text-accent-foreground"
                            : "hover:bg-accent/50"
                        )}
                      >
                        <div className="relative shrink-0">
                          <Avatar className="size-9">
                            <AvatarImage src={conv.avatar} />
                            <AvatarFallback>
                              {initialsFromName(title, isGroup ? "G" : "D")}
                            </AvatarFallback>
                          </Avatar>
                          {!isGroup ? (
                            <span
                              className={cn(
                                "absolute bottom-0 right-0 size-2.5 rounded-full ring-2 ring-background",
                                peerOnline ? "bg-emerald-500" : "bg-zinc-400"
                              )}
                            />
                          ) : null}
                        </div>

                        <div className="flex-1 min-w-0">
                          <div className="flex items-center justify-between gap-1">
                            <span className="text-xs font-medium truncate">
                              {title}
                            </span>
                            {lastMsg ? (
                              <span className="text-[10px] text-muted-foreground shrink-0">
                                {formatTimestamp(lastMsg.created_at)}
                              </span>
                            ) : null}
                          </div>
                          <div className="flex items-center justify-between gap-1 mt-0.5">
                            <span className="text-[11px] text-muted-foreground truncate">
                              {preview}
                            </span>
                            {unread > 0 ? (
                              <Badge
                                variant="destructive"
                                className="h-4 min-w-4 px-1 text-[10px]"
                              >
                                {unread > 99 ? "99+" : unread}
                              </Badge>
                            ) : null}
                          </div>
                        </div>
                      </button>
                    )
                  })}
                </div>
              )}
            </ScrollArea>
          </div>

          <MessagePane
            conversation={activeConversation}
            currentUserId={session.userId}
            messages={
              activeConvId ? (messagesByConv[activeConvId] ?? []) : []
            }
            hasMoreHistory={
              activeConvId
                ? Boolean(cursorsByConv[activeConvId]?.hasMore)
                : false
            }
            loadingHistory={loadingHistoryConvId === activeConvId}
            readStates={
              activeConvId ? (readStatesByConv[activeConvId] ?? []) : []
            }
            typingUserIds={
              activeConvId ? (typingByConv[activeConvId] ?? []) : []
            }
            presenceMap={presenceMap}
            onLoadOlderHistory={handleLoadOlderHistory}
            onSendMessage={handleSendMessage}
            onTyping={() => {
              if (activeConvId) aimWsClient.sendTyping(activeConvId)
            }}
            onOpenSearch={() => {
              setSearchScoped(true)
              setSearchDialogOpen(true)
            }}
            onOpenGroupSettings={() => setGroupSettingsOpen(true)}
          />
        </div>
      )}

      <CreateConversationDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        currentUserId={session.userId}
        friends={friends}
        onCreated={(created) => {
          setConversations((prev) => [
            created,
            ...prev.filter((c) => c.conversation_id !== created.conversation_id),
          ])
          setActiveConvId(created.conversation_id)
          setActiveNav("chat")
        }}
      />

      {activeConversation && activeConversation.conversation_type === "group" ? (
        <GroupSettingsDialog
          open={groupSettingsOpen}
          onOpenChange={setGroupSettingsOpen}
          conversation={activeConversation}
          currentUserId={session.userId}
          friends={friends}
          onConversationUpdated={(updated) => {
            setConversations((prev) =>
              prev.map((c) =>
                c.conversation_id === updated.conversation_id ? updated : c
              )
            )
          }}
          onConversationRemoved={(removedId) => {
            setConversations((prev) =>
              prev.filter((c) => c.conversation_id !== removedId)
            )
            if (activeConvId === removedId) {
              setActiveConvId(null)
            }
          }}
        />
      ) : null}

      <SearchDialog
        open={searchDialogOpen}
        onOpenChange={setSearchDialogOpen}
        scopedConversation={searchScoped ? activeConversation : null}
        onSelectConversation={(convId) => {
          setActiveConvId(convId)
          setActiveNav("chat")
        }}
        onStartDirectChat={handleStartDirectChat}
        onFriendRequested={() => void refreshContactsData()}
      />
    </div>
  )
}
