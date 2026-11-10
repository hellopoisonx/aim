import {
  FileTextIcon,
  MessageSquareIcon,
  SearchIcon,
  UserPlusIcon,
  UsersIcon,
} from "lucide-react"
import { type SyntheticEvent, useState } from "react"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { aimApi } from "@/lib/api"
import type { ConversationItem, SearchResponse } from "@/lib/types"
import {
  formatTimestamp,
  initialsFromName,
  tokenizeMarkedSnippet,
} from "@/lib/utils"

interface SearchDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  scopedConversation?: ConversationItem | null
  onSelectConversation: (conversationId: number) => void
  onStartDirectChat: (targetUserId: number, name?: string) => Promise<void>
  onFriendRequested: () => void
}

function HighlightedSnippet({ snippet }: { snippet: string }) {
  const tokens = tokenizeMarkedSnippet(snippet)
  return (
    <span className="text-xs text-muted-foreground break-all">
      {tokens.map((tok, idx) =>
        tok.highlighted ? (
          <mark
            key={idx}
            className="rounded-xs bg-primary/15 px-0.5 text-foreground font-medium"
          >
            {tok.text}
          </mark>
        ) : (
          <span key={idx}>{tok.text}</span>
        )
      )}
    </span>
  )
}

export function SearchDialog({
  open,
  onOpenChange,
  scopedConversation,
  onSelectConversation,
  onStartDirectChat,
  onFriendRequested,
}: SearchDialogProps) {
  const [query, setQuery] = useState("")
  const [scope, setScope] = useState<
    "all" | "messages" | "conversations" | "friends" | "users"
  >(scopedConversation ? "messages" : "all")
  const [onlyCurrentConv, setOnlyCurrentConv] = useState(
    Boolean(scopedConversation)
  )
  const [results, setResults] = useState<SearchResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const [addingUserId, setAddingUserId] = useState<number | null>(null)

  async function runSearch(cursor?: { createdAt: number; id: number }) {
    const trimmed = query.trim()
    if (!trimmed) return
    setLoading(true)
    setErrorMsg(null)
    try {
      const effectiveScope =
        onlyCurrentConv && scopedConversation
          ? "messages"
          : scope === "all"
            ? undefined
            : scope
      const data = await aimApi.search({
        q: trimmed,
        scope: effectiveScope,
        conversationId:
          onlyCurrentConv && scopedConversation
            ? scopedConversation.conversation_id
            : undefined,
        cursorCreatedAt: cursor?.createdAt,
        cursorId: cursor?.id,
        limit: 20,
      })
      if (cursor && results) {
        setResults({
          ...data,
          messages: [...results.messages, ...(data.messages ?? [])],
        })
      } else {
        setResults(data)
      }
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "搜索失败")
    } finally {
      setLoading(false)
    }
  }

  function handleSubmit(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault()
    void runSearch()
  }

  async function handleAddFriend(userId: number) {
    setAddingUserId(userId)
    try {
      await aimApi.addFriend(userId)
      onFriendRequested()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "发送好友申请失败")
    } finally {
      setAddingUserId(null)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>统一搜索</DialogTitle>
          <DialogDescription>
            搜索消息记录、会话、好友或全站用户
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex items-center gap-2">
          <div className="relative flex-1">
            <SearchIcon className="size-4 text-muted-foreground absolute left-3 top-2.5" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="输入关键词回车搜索..."
              className="pl-9"
              autoFocus
            />
          </div>
          <Button type="submit" disabled={loading || !query.trim()}>
            {loading ? "搜索中..." : "搜索"}
          </Button>
        </form>

        <div className="flex flex-wrap items-center justify-between gap-2">
          <Tabs
            value={onlyCurrentConv ? "messages" : scope}
            onValueChange={(v) => {
              setOnlyCurrentConv(false)
              setScope(
                v as "all" | "messages" | "conversations" | "friends" | "users"
              )
            }}
          >
            <TabsList>
              <TabsTrigger value="all">全部</TabsTrigger>
              <TabsTrigger value="messages">消息</TabsTrigger>
              <TabsTrigger value="conversations">会话</TabsTrigger>
              <TabsTrigger value="friends">好友</TabsTrigger>
              <TabsTrigger value="users">用户</TabsTrigger>
            </TabsList>
          </Tabs>

          {scopedConversation ? (
            <Button
              type="button"
              variant={onlyCurrentConv ? "default" : "outline"}
              size="sm"
              onClick={() => setOnlyCurrentConv((prev) => !prev)}
            >
              仅当前会话: {scopedConversation.name || `#${scopedConversation.conversation_id}`}
            </Button>
          ) : null}
        </div>

        {errorMsg ? (
          <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {errorMsg}
          </div>
        ) : null}

        <ScrollArea className="h-96 rounded-md border p-3">
          {!results ? (
            <div className="h-80 flex items-center justify-center text-sm text-muted-foreground">
              输入关键词搜索消息、群组、好友或用户
            </div>
          ) : (
            <div className="space-y-5">
              {results.conversations?.length ? (
                <div className="space-y-2">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                    <UsersIcon className="size-3.5" />
                    <span>会话 ({results.conversations.length})</span>
                  </div>
                  <div className="space-y-1">
                    {results.conversations.map((item) => (
                      <button
                        key={item.conversation.conversation_id}
                        type="button"
                        onClick={() => {
                          onSelectConversation(item.conversation.conversation_id)
                          onOpenChange(false)
                        }}
                        className="w-full flex items-center justify-between gap-3 rounded-md p-2 text-left hover:bg-accent transition-colors"
                      >
                        <div className="flex items-center gap-2.5 min-w-0">
                          <Avatar className="size-8">
                            <AvatarImage src={item.conversation.avatar} />
                            <AvatarFallback>
                              {initialsFromName(item.conversation.name, "C")}
                            </AvatarFallback>
                          </Avatar>
                          <div className="min-w-0">
                            <div className="text-sm font-medium truncate">
                              {item.conversation.name ||
                                `会话 #${item.conversation.conversation_id}`}
                            </div>
                            <HighlightedSnippet snippet={item.snippet} />
                          </div>
                        </div>
                        <Badge variant="outline">
                          {item.conversation.conversation_type === "group"
                            ? "群聊"
                            : "单聊"}
                        </Badge>
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}

              {results.friends?.length ? (
                <div className="space-y-2">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                    <MessageSquareIcon className="size-3.5" />
                    <span>好友 ({results.friends.length})</span>
                  </div>
                  <div className="space-y-1">
                    {results.friends.map((item) => (
                      <div
                        key={item.user.id}
                        className="flex items-center justify-between gap-3 rounded-md p-2 hover:bg-accent"
                      >
                        <div className="flex items-center gap-2.5 min-w-0">
                          <Avatar className="size-8">
                            <AvatarImage src={item.user.avatar} />
                            <AvatarFallback>
                              {initialsFromName(
                                item.user.nickname || item.friendship.name
                              )}
                            </AvatarFallback>
                          </Avatar>
                          <div className="min-w-0">
                            <div className="text-sm font-medium truncate">
                              {item.user.nickname || item.friendship.name}
                            </div>
                            <HighlightedSnippet snippet={item.snippet} />
                          </div>
                        </div>
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => {
                            void onStartDirectChat(
                              item.user.id,
                              item.user.nickname || item.friendship.name
                            )
                            onOpenChange(false)
                          }}
                        >
                          发消息
                        </Button>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}

              {results.users?.length ? (
                <div className="space-y-2">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                    <UserPlusIcon className="size-3.5" />
                    <span>用户 ({results.users.length})</span>
                  </div>
                  <div className="space-y-1">
                    {results.users.map((item) => (
                      <div
                        key={item.user.id}
                        className="flex items-center justify-between gap-3 rounded-md p-2 hover:bg-accent"
                      >
                        <div className="flex items-center gap-2.5 min-w-0">
                          <Avatar className="size-8">
                            <AvatarImage src={item.user.avatar} />
                            <AvatarFallback>
                              {initialsFromName(item.user.nickname)}
                            </AvatarFallback>
                          </Avatar>
                          <div className="min-w-0">
                            <div className="text-sm font-medium truncate">
                              {item.user.nickname} (ID: {item.user.id})
                            </div>
                            <HighlightedSnippet snippet={item.snippet} />
                          </div>
                        </div>
                        <div className="flex items-center gap-1.5">
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={addingUserId === item.user.id}
                            onClick={() => void handleAddFriend(item.user.id)}
                          >
                            加好友
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}

              {results.messages?.length ? (
                <div className="space-y-2">
                  <div className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
                    <FileTextIcon className="size-3.5" />
                    <span>消息 ({results.messages.length})</span>
                  </div>
                  <div className="space-y-1">
                    {results.messages.map((item) => (
                      <button
                        key={item.message.id}
                        type="button"
                        onClick={() => {
                          onSelectConversation(item.message.conversation_id)
                          onOpenChange(false)
                        }}
                        className="w-full flex flex-col gap-1 rounded-md border p-2.5 text-left hover:bg-accent transition-colors"
                      >
                        <div className="flex items-center justify-between text-xs text-muted-foreground">
                          <span className="font-medium text-foreground">
                            {item.message.sender_info?.name ||
                              `用户 #${item.message.sender_id}`}{" "}
                            · 会话 #{item.message.conversation_id}
                          </span>
                          <span>{formatTimestamp(item.message.created_at)}</span>
                        </div>
                        <HighlightedSnippet
                          snippet={item.snippet || item.message.content}
                        />
                      </button>
                    ))}
                  </div>
                  {results.has_more ? (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="w-full"
                      disabled={loading}
                      onClick={() =>
                        void runSearch({
                          createdAt: results.next_cursor_created_at,
                          id: results.next_cursor_id,
                        })
                      }
                    >
                      加载更多消息结果
                    </Button>
                  ) : null}
                </div>
              ) : null}

              {!results.conversations?.length &&
                !results.friends?.length &&
                !results.users?.length &&
                !results.messages?.length ? (
                <div className="h-64 flex items-center justify-center text-sm text-muted-foreground">
                  未找到匹配结果
                </div>
              ) : null}
            </div>
          )}
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}
