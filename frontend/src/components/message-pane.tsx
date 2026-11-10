import {
  AlertCircleIcon,
  AtSignIcon,
  CheckCheckIcon,
  CheckIcon,
  ClockIcon,
  DownloadIcon,
  FileIcon,
  HistoryIcon,
  ImageIcon,
  Loader2Icon,
  PaperclipIcon,
  RefreshCwIcon,
  SearchIcon,
  SendIcon,
  Settings2Icon,
  UsersIcon,
} from "lucide-react"
import {
  type ChangeEvent,
  type SyntheticEvent,
  type KeyboardEvent,
  useEffect,
  useRef,
  useState,
} from "react"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Textarea } from "@/components/ui/textarea"
import { aimApi, parseApiJson } from "@/lib/api"
import type {
  AttachmentContentV1,
  ChatMessage,
  ConversationItem,
  EntityId,
  MemberDetailItem,
  ReadStateItem,
  SystemMessageEvent,
} from "@/lib/types"
import {
  cn,
  compareId,
  formatBytes,
  formatTimestamp,
  hasId,
  initialsFromName,
} from "@/lib/utils"

interface MessagePaneProps {
  conversation: ConversationItem | null
  currentUserId: EntityId
  messages: ChatMessage[]
  hasMoreHistory: boolean
  loadingHistory: boolean
  readStates: ReadStateItem[]
  typingUserIds: EntityId[]
  presenceMap: Record<EntityId, string>
  onLoadOlderHistory: () => Promise<void>
  onSendMessage: (input: {
    messageType: string
    content: string
    mentions: string[]
    retryClientMsgId?: string
  }) => Promise<void>
  onTyping: () => void
  onOpenSearch: () => void
  onOpenGroupSettings: () => void
}

function parseAttachmentContent(raw: string): AttachmentContentV1 | null {
  try {
    const parsed = JSON.parse(raw) as AttachmentContentV1
    if (parsed && parsed.file_id && parsed.original) {
      return parsed
    }
    return null
  } catch {
    return null
  }
}

function formatSystemMessage(raw: string): string {
  try {
    const parsed = parseApiJson<SystemMessageEvent>(raw)
    const targets = parsed.target_user_ids?.map((id) => `#${id}`).join(", ") ?? ""
    switch (parsed.event) {
      case "member_joined":
        return `用户 #${parsed.operator_id ?? ""} 邀请 ${targets} 加入了群聊`
      case "member_left":
        return `成员 #${parsed.operator_id ?? targets} 退出了群聊`
      case "member_removed":
        return `管理员 #${parsed.operator_id ?? ""} 将 ${targets} 移出了群聊`
      case "group_renamed":
        return `群名称已更新${parsed.name ? `为「${parsed.name}」` : ""}`
      case "group_avatar_changed":
        return "群头像已更新"
      case "group_dismissed":
        return "该群聊已被群主解散"
      default:
        return raw
    }
  } catch {
    return raw
  }
}

function AttachmentBubble({ content }: { content: string }) {
  const meta = parseAttachmentContent(content)
  const [downloadUrl, setDownloadUrl] = useState<string | null>(null)
  const [loadingUrl, setLoadingUrl] = useState(false)
  const [errorText, setErrorText] = useState<string | null>(null)

  useEffect(() => {
    if (!meta?.file_id) return
    if (meta.kind === "image" || meta.kind === "video" || meta.kind === "audio") {
      setLoadingUrl(true)
      aimApi
        .downloadAttachment(meta.file_id)
        .then((res) => setDownloadUrl(res.url))
        .catch(() => setErrorText("无法获取媒体链接"))
        .finally(() => setLoadingUrl(false))
    }
  }, [meta?.file_id, meta?.kind])

  if (!meta) {
    return <p className="text-sm whitespace-pre-wrap break-words">{content}</p>
  }

  async function handleDownload() {
    if (!meta) return
    setLoadingUrl(true)
    setErrorText(null)
    try {
      const res = await aimApi.downloadAttachment(meta.file_id)
      window.open(res.url, "_blank", "noopener,noreferrer")
    } catch (err) {
      setErrorText(err instanceof Error ? err.message : "下载失败")
    } finally {
      setLoadingUrl(false)
    }
  }

  return (
    <div className="space-y-2 max-w-xs sm:max-w-sm">
      {meta.kind === "image" ? (
        <div className="rounded-lg overflow-hidden border bg-muted/40 min-h-24 flex items-center justify-center">
          {downloadUrl ? (
            <a href={downloadUrl} target="_blank" rel="noreferrer">
              <img
                src={downloadUrl}
                alt={meta.original.name}
                className="max-h-64 w-auto object-contain"
              />
            </a>
          ) : (
            <div className="p-4 flex items-center gap-2 text-xs text-muted-foreground">
              <ImageIcon className="size-4" />
              <span>{loadingUrl ? "正在加载图片..." : meta.original.name}</span>
            </div>
          )}
        </div>
      ) : null}

      {meta.kind === "video" && downloadUrl ? (
        <video
          controls
          src={downloadUrl}
          className="max-h-64 rounded-lg border bg-black"
        />
      ) : null}

      {meta.kind === "audio" && downloadUrl ? (
        <audio controls src={downloadUrl} className="w-full" />
      ) : null}

      <div className="flex items-center justify-between gap-3 rounded-md border bg-background/80 px-3 py-2 text-foreground">
        <div className="flex items-center gap-2 min-w-0">
          <FileIcon className="size-4 shrink-0 text-muted-foreground" />
          <div className="min-w-0">
            <div className="text-xs font-medium truncate">
              {meta.original.name}
            </div>
            <div className="text-[11px] text-muted-foreground flex items-center gap-1.5">
              <span>{formatBytes(meta.original.size)}</span>
              {meta.width && meta.height ? (
                <span>
                  · {meta.width}×{meta.height}
                </span>
              ) : null}
              {meta.parse_status ? (
                <Badge variant="outline" className="h-4 px-1 text-[10px]">
                  {meta.parse_status}
                </Badge>
              ) : null}
            </div>
          </div>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7 shrink-0"
          disabled={loadingUrl}
          onClick={() => void handleDownload()}
          title="下载附件"
        >
          <DownloadIcon className="size-3.5" />
        </Button>
      </div>
      {errorText ? (
        <div className="text-[11px] text-destructive">{errorText}</div>
      ) : null}
    </div>
  )
}

export function MessagePane({
  conversation,
  currentUserId,
  messages,
  hasMoreHistory,
  loadingHistory,
  readStates,
  typingUserIds,
  presenceMap,
  onLoadOlderHistory,
  onSendMessage,
  onTyping,
  onOpenSearch,
  onOpenGroupSettings,
}: MessagePaneProps) {
  const [draft, setDraft] = useState("")
  const [mentions, setMentions] = useState<string[]>([])
  const [groupMembers, setGroupMembers] = useState<MemberDetailItem[]>([])
  const [uploading, setUploading] = useState(false)
  const [composerError, setComposerError] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const bottomRef = useRef<HTMLDivElement | null>(null)
  const lastTypingSentRef = useRef(0)

  useEffect(() => {
    setDraft("")
    setMentions([])
    setComposerError(null)
    if (conversation?.conversation_type === "group") {
      aimApi
        .getConversationMembers(conversation.conversation_id)
        .then(setGroupMembers)
        .catch(() => setGroupMembers([]))
    } else {
      setGroupMembers([])
    }
  }, [conversation?.conversation_id, conversation?.conversation_type])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [conversation?.conversation_id, messages.length])

  if (!conversation) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-muted/20 p-8 text-center">
        <div className="max-w-xs space-y-2">
          <h3 className="text-base font-medium">选择一个会话开始聊天</h3>
          <p className="text-xs text-muted-foreground">
            可在左侧会话列表切换单聊或群聊，也可以从通讯录向好友发起会话
          </p>
        </div>
      </div>
    )
  }

  const isGroup = conversation.conversation_type === "group"
  const peerId = !isGroup
    ? conversation.member_ids?.find((id) => id !== currentUserId)
    : undefined
  const peerOnline = peerId ? presenceMap[peerId] === "online" : false

  function handleDraftChange(value: string) {
    setDraft(value)
    const now = Date.now()
    if (value.trim() && now - lastTypingSentRef.current >= 2500) {
      lastTypingSentRef.current = now
      onTyping()
    }
  }

  async function submitTextMessage() {
    const trimmed = draft.trim()
    if (!trimmed) return
    setComposerError(null)
    const textToSend = trimmed
    const mentionsToSend = [...mentions]
    setDraft("")
    setMentions([])
    try {
      await onSendMessage({
        messageType: "text",
        content: textToSend,
        mentions: mentionsToSend,
      })
    } catch (err) {
      setComposerError(err instanceof Error ? err.message : "发送失败")
    }
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault()
      void submitTextMessage()
    }
  }

  function handleFormSubmit(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault()
    void submitTextMessage()
  }

  async function handleFileSelect(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (!file || !conversation) return
    setUploading(true)
    setComposerError(null)
    try {
      const attachmentContent = await aimApi.uploadFileForConversation(
        conversation.conversation_id,
        file
      )
      await onSendMessage({
        messageType: attachmentContent.kind,
        content: JSON.stringify(attachmentContent),
        mentions: [],
      })
    } catch (err) {
      setComposerError(err instanceof Error ? err.message : "上传附件失败")
    } finally {
      setUploading(false)
    }
  }

  function toggleMention(userId: EntityId, displayName: string) {
    const idStr = String(userId)
    if (mentions.includes(idStr)) {
      setMentions((prev) => prev.filter((item) => item !== idStr))
      return
    }
    setMentions((prev) => [...prev, idStr])
    if (!draft.includes(`@${displayName}`)) {
      setDraft((prev) => `${prev}${prev.endsWith(" ") || !prev ? "" : " "}@${displayName} `)
    }
  }

  return (
    <div className="flex-1 flex flex-col h-full bg-background overflow-hidden">
      <header className="h-14 border-b px-5 flex items-center justify-between shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          <Avatar className="size-9">
            <AvatarImage src={conversation.avatar} />
            <AvatarFallback>
              {initialsFromName(conversation.name, isGroup ? "G" : "D")}
            </AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-semibold truncate">
                {conversation.name || `会话 #${conversation.conversation_id}`}
              </h2>
              <Badge variant="outline" className="text-[10px] h-4 px-1.5">
                {isGroup ? "群聊" : "单聊"}
              </Badge>
            </div>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              {isGroup ? (
                <span>
                  {conversation.member_ids?.length || groupMembers.length || 1} 名成员
                </span>
              ) : (
                <span className="flex items-center gap-1">
                  <span
                    className={cn(
                      "size-2 rounded-full",
                      peerOnline ? "bg-emerald-500" : "bg-zinc-400"
                    )}
                  />
                  {peerOnline ? "在线" : "离线"}
                </span>
              )}
              {typingUserIds.length > 0 ? (
                <span className="text-primary font-medium">
                  ·{" "}
                  {isGroup
                    ? `${typingUserIds.map((id) => `#${id}`).join(", ")} 正在输入...`
                    : "对方正在输入..."}
                </span>
              ) : null}
            </div>
          </div>
        </div>

        <div className="flex items-center gap-1.5">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={onOpenSearch}
            title="搜索会话消息"
          >
            <SearchIcon className="size-4" />
          </Button>
          {isGroup ? (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={onOpenGroupSettings}
              title="群聊管理"
            >
              <Settings2Icon className="size-4" />
            </Button>
          ) : null}
        </div>
      </header>

      <ScrollArea className="flex-1 px-5 py-4">
        <div className="space-y-4">
          {hasMoreHistory ? (
            <div className="flex justify-center">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-7 text-xs gap-1.5"
                disabled={loadingHistory}
                onClick={() => void onLoadOlderHistory()}
              >
                <HistoryIcon className="size-3.5" />
                {loadingHistory ? "加载中..." : "加载更早历史消息"}
              </Button>
            </div>
          ) : null}

          {messages.length === 0 ? (
            <div className="py-20 text-center text-xs text-muted-foreground">
              暂无消息记录，发送第一条消息开始对话
            </div>
          ) : null}

          {messages.map((msg) => {
            const isSystemNotice =
              msg.is_system &&
              (msg.message_type === "system" || !hasId(msg.sender_id))
            if (isSystemNotice) {
              return (
                <div
                  key={hasId(msg.id) ? msg.id : msg.client_msg_id}
                  className="flex justify-center my-2"
                >
                  <div className="rounded-full bg-muted px-3 py-1 text-[11px] text-muted-foreground">
                    {formatSystemMessage(msg.content)} ·{" "}
                    {formatTimestamp(msg.created_at)}
                  </div>
                </div>
              )
            }

            const isMine = msg.sender_id === currentUserId
            const senderName =
              msg.sender_info?.name ||
              (isMine ? "我" : `用户 #${msg.sender_id}`)
            const isAttachment =
              msg.message_type === "image" ||
              msg.message_type === "video" ||
              msg.message_type === "audio" ||
              msg.message_type === "file"

            const peerReads = readStates.filter(
              (rs) =>
                rs.user_id !== currentUserId &&
                hasId(msg.id) &&
                compareId(rs.last_read_message_id, msg.id) >= 0
            )
            const detailReads = (msg.read_details ?? []).filter(
              (rd) => rd.user_id !== currentUserId && rd.is_read
            )
            const readUserMap = new Map<
              EntityId,
              { user_id: EntityId; name: string }
            >()
            for (const r of peerReads) {
              readUserMap.set(r.user_id, {
                user_id: r.user_id,
                name: r.name || `用户 #${r.user_id}`,
              })
            }
            for (const d of detailReads) {
              readUserMap.set(d.user_id, {
                user_id: d.user_id,
                name: d.name || `用户 #${d.user_id}`,
              })
            }
            const readUsers = Array.from(readUserMap.values())

            return (
              <div
                key={hasId(msg.id) ? `id-${msg.id}` : `client-${msg.client_msg_id}`}
                className={cn(
                  "flex items-start gap-2.5",
                  isMine ? "flex-row-reverse" : "flex-row"
                )}
              >
                <Avatar className="size-8 shrink-0 mt-0.5">
                  <AvatarFallback>{initialsFromName(senderName)}</AvatarFallback>
                </Avatar>

                <div
                  className={cn(
                    "flex flex-col max-w-[75%] gap-1",
                    isMine ? "items-end" : "items-start"
                  )}
                >
                  <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
                    <span>{senderName}</span>
                    <span>{formatTimestamp(msg.created_at)}</span>
                  </div>

                  <div
                    className={cn(
                      "rounded-2xl px-3.5 py-2 text-sm shadow-2xs",
                      isMine
                        ? "bg-primary text-primary-foreground rounded-tr-xs"
                        : "bg-muted text-foreground rounded-tl-xs"
                    )}
                  >
                    {isAttachment ? (
                      <AttachmentBubble content={msg.content} />
                    ) : (
                      <p className="whitespace-pre-wrap break-words leading-relaxed">
                        {msg.content}
                      </p>
                    )}

                    {msg.mentions && msg.mentions.length > 0 ? (
                      <div className="flex flex-wrap gap-1 mt-1.5 pt-1 border-t border-current/15 text-[11px] opacity-90">
                        {msg.mentions.map((uid) => (
                          <span key={uid}>@{uid}</span>
                        ))}
                      </div>
                    ) : null}
                  </div>

                  {isMine ? (
                    <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
                      {msg.local_status === "sending" ? (
                        <span className="flex items-center gap-1">
                          <ClockIcon className="size-3" />
                          发送中...
                        </span>
                      ) : msg.local_status === "failed" ? (
                        <span className="flex items-center gap-1.5 text-destructive">
                          <AlertCircleIcon className="size-3" />
                          <span>{msg.error_msg || "发送失败"}</span>
                          <button
                            type="button"
                            onClick={() =>
                              void onSendMessage({
                                messageType: msg.message_type,
                                content: msg.content,
                                mentions: msg.mentions ?? [],
                                retryClientMsgId: msg.client_msg_id,
                              })
                            }
                            className="inline-flex items-center gap-0.5 underline font-medium"
                          >
                            <RefreshCwIcon className="size-3" />
                            重发
                          </button>
                        </span>
                      ) : isGroup ? (
                        <Popover>
                          <PopoverTrigger asChild>
                            <button
                              type="button"
                              className="inline-flex items-center gap-1 hover:text-foreground transition-colors"
                            >
                              {readUsers.length > 0 ? (
                                <CheckCheckIcon className="size-3.5 text-emerald-600" />
                              ) : (
                                <CheckIcon className="size-3.5" />
                              )}
                              <span>{readUsers.length} 人已读</span>
                            </button>
                          </PopoverTrigger>
                          <PopoverContent align="end" className="w-56 p-3 space-y-2">
                            <div className="text-xs font-medium flex items-center gap-1.5">
                              <UsersIcon className="size-3.5" />
                              已读成员 ({readUsers.length})
                            </div>
                            {readUsers.length === 0 ? (
                              <div className="text-xs text-muted-foreground">
                                暂无其他成员已读
                              </div>
                            ) : (
                              <div className="space-y-1 max-h-40 overflow-y-auto">
                                {readUsers.map((u) => (
                                  <div
                                    key={u.user_id}
                                    className="text-xs flex items-center justify-between"
                                  >
                                    <span className="truncate">{u.name}</span>
                                    <span className="text-muted-foreground">
                                      #{u.user_id}
                                    </span>
                                  </div>
                                ))}
                              </div>
                            )}
                          </PopoverContent>
                        </Popover>
                      ) : (
                        <span className="inline-flex items-center gap-1">
                          {readUsers.length > 0 ? (
                            <>
                              <CheckCheckIcon className="size-3.5 text-emerald-600" />
                              <span>已读</span>
                            </>
                          ) : (
                            <>
                              <CheckIcon className="size-3.5" />
                              <span>已送达</span>
                            </>
                          )}
                        </span>
                      )}
                    </div>
                  ) : null}
                </div>
              </div>
            )
          })}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>

      <footer className="border-t p-3 bg-card shrink-0 space-y-2">
        {composerError ? (
          <div className="rounded-md border border-destructive/30 bg-destructive/10 px-2.5 py-1.5 text-xs text-destructive">
            {composerError}
          </div>
        ) : null}

        {mentions.length > 0 ? (
          <div className="flex items-center gap-1.5 flex-wrap">
            <span className="text-xs text-muted-foreground">提及:</span>
            {mentions.map((id) => (
              <Badge
                key={id}
                variant="secondary"
                className="text-[11px] cursor-pointer"
                onClick={() =>
                  setMentions((prev) => prev.filter((item) => item !== id))
                }
              >
                @{id} ×
              </Badge>
            ))}
          </div>
        ) : null}

        <form onSubmit={handleFormSubmit} className="space-y-2">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-1">
              <input
                ref={fileInputRef}
                type="file"
                className="hidden"
                onChange={(e) => void handleFileSelect(e)}
              />
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs gap-1"
                disabled={uploading}
                onClick={() => fileInputRef.current?.click()}
              >
                {uploading ? (
                  <Loader2Icon className="size-3.5 animate-spin" />
                ) : (
                  <PaperclipIcon className="size-3.5" />
                )}
                <span>{uploading ? "上传中..." : "发送附件"}</span>
              </Button>

              {isGroup ? (
                <Popover>
                  <PopoverTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-7 px-2 text-xs gap-1"
                    >
                      <AtSignIcon className="size-3.5" />
                      <span>提及成员</span>
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent align="start" className="w-56 p-2">
                    <div className="text-xs font-medium px-2 py-1">
                      选择要 @ 的群成员
                    </div>
                    <div className="max-h-48 overflow-y-auto space-y-0.5">
                      {groupMembers
                        .filter((m) => m.user_id !== currentUserId)
                        .map((m) => {
                          const label = m.name || `用户 #${m.user_id}`
                          const active = mentions.includes(String(m.user_id))
                          return (
                            <button
                              key={m.user_id}
                              type="button"
                              onClick={() => toggleMention(m.user_id, label)}
                              className={cn(
                                "w-full flex items-center justify-between rounded-sm px-2 py-1.5 text-xs hover:bg-accent",
                                active && "bg-secondary font-medium"
                              )}
                            >
                              <span className="truncate">{label}</span>
                              <span className="text-muted-foreground">
                                #{m.user_id}
                              </span>
                            </button>
                          )
                        })}
                    </div>
                  </PopoverContent>
                </Popover>
              ) : null}
            </div>

            <span className="text-[11px] text-muted-foreground">
              Enter 发送 · Shift+Enter 换行
            </span>
          </div>

          <div className="flex items-end gap-2">
            <Textarea
              value={draft}
              onChange={(e) => handleDraftChange(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="输入消息..."
              className="min-h-16 max-h-36 resize-none text-sm"
            />
            <Button
              type="submit"
              size="icon"
              className="size-10 shrink-0"
              disabled={!draft.trim()}
            >
              <SendIcon className="size-4" />
            </Button>
          </div>
        </form>
      </footer>
    </div>
  )
}
