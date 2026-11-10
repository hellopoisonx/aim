import {
  CheckIcon,
  MessageSquarePlusIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  TagIcon,
  Trash2Icon,
  UserCheckIcon,
  UserPlusIcon,
  XIcon,
} from "lucide-react"
import { type SyntheticEvent, useState } from "react"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { aimApi } from "@/lib/api"
import type {
  EntityId,
  FriendTagItem,
  FriendshipItem,
  UserListItem,
} from "@/lib/types"
import { cn, formatTimestamp, initialsFromName } from "@/lib/utils"

interface ContactsPaneProps {
  currentUserId: EntityId
  friends: FriendshipItem[]
  applications: FriendshipItem[]
  tags: FriendTagItem[]
  presenceMap: Record<EntityId, string>
  onRefreshContacts: () => Promise<void>
  onStartDirectChat: (friendUserId: EntityId, friendName?: string) => Promise<void>
}

export function ContactsPane({
  currentUserId,
  friends,
  applications,
  tags,
  presenceMap,
  onRefreshContacts,
  onStartDirectChat,
}: ContactsPaneProps) {
  const [selectedTagId, setSelectedTagId] = useState<EntityId | null>(null)
  const [newTagName, setNewTagName] = useState("")
  const [editingTagId, setEditingTagId] = useState<EntityId | null>(null)
  const [editingTagName, setEditingTagName] = useState("")
  const [searchName, setSearchName] = useState("")
  const [foundUsers, setFoundUsers] = useState<UserListItem[]>([])
  const [searchingUsers, setSearchingUsers] = useState(false)
  const [busyAction, setBusyAction] = useState<string | null>(null)
  const [feedback, setFeedback] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  const pendingIncomingCount = applications.filter(
    (app) =>
      app.status === "pending" &&
      (app.friend_id === currentUserId || app.user_id !== currentUserId)
  ).length

  const filteredFriends =
    selectedTagId === null
      ? friends
      : friends.filter((f) => f.tags?.some((t) => t.id === selectedTagId))

  function resolvePeerUserId(item: FriendshipItem): EntityId {
    return item.user_id === currentUserId ? item.friend_id : item.user_id
  }

  async function handleCreateTag(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault()
    const trimmed = newTagName.trim()
    if (!trimmed) return
    setBusyAction("create-tag")
    setErrorMsg(null)
    try {
      await aimApi.createFriendTag(trimmed)
      setNewTagName("")
      await onRefreshContacts()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "创建标签失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleRenameTag(tagId: EntityId) {
    const trimmed = editingTagName.trim()
    if (!trimmed) return
    setBusyAction(`rename-tag-${tagId}`)
    setErrorMsg(null)
    try {
      await aimApi.renameFriendTag(tagId, trimmed)
      setEditingTagId(null)
      setEditingTagName("")
      await onRefreshContacts()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "重命名标签失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleDeleteTag(tagId: EntityId) {
    setBusyAction(`delete-tag-${tagId}`)
    setErrorMsg(null)
    try {
      await aimApi.deleteFriendTag(tagId)
      if (selectedTagId === tagId) {
        setSelectedTagId(null)
      }
      await onRefreshContacts()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "删除标签失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleToggleFriendTag(
    friendUserId: EntityId,
    currentTags: FriendTagItem[],
    tagId: EntityId,
    checked: boolean
  ) {
    setBusyAction(`friend-tag-${friendUserId}-${tagId}`)
    setErrorMsg(null)
    try {
      if (checked) {
        const nextIds = Array.from(
          new Set([...currentTags.map((t) => t.id), tagId])
        )
        await aimApi.setFriendTags(friendUserId, nextIds)
      } else {
        await aimApi.removeFriendTag(friendUserId, tagId)
      }
      await onRefreshContacts()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "更新好友标签失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleAcceptApplication(applicantId: EntityId) {
    setBusyAction(`accept-${applicantId}`)
    setErrorMsg(null)
    try {
      await aimApi.acceptFriend(applicantId)
      await onRefreshContacts()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "接受申请失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleRejectApplication(applicantId: EntityId) {
    setBusyAction(`reject-${applicantId}`)
    setErrorMsg(null)
    try {
      await aimApi.rejectFriend(applicantId)
      await onRefreshContacts()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "拒绝申请失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleSearchUsers(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault()
    const trimmed = searchName.trim()
    if (!trimmed) return
    setSearchingUsers(true)
    setErrorMsg(null)
    setFeedback(null)
    try {
      const users = await aimApi.searchUsersByName(trimmed)
      setFoundUsers(users)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "搜索用户失败")
    } finally {
      setSearchingUsers(false)
    }
  }

  async function handleSendFriendRequest(targetUserId: EntityId) {
    setBusyAction(`add-friend-${targetUserId}`)
    setErrorMsg(null)
    setFeedback(null)
    try {
      await aimApi.addFriend(targetUserId)
      setFeedback(`已向用户 #${targetUserId} 发送好友申请`)
      await onRefreshContacts()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "发送好友申请失败")
    } finally {
      setBusyAction(null)
    }
  }

  return (
    <div className="flex-1 flex flex-col h-full bg-background overflow-hidden">
      <div className="h-14 border-b px-6 flex items-center justify-between shrink-0">
        <div>
          <h2 className="text-base font-semibold">通讯录与好友管理</h2>
          <p className="text-xs text-muted-foreground">
            共 {friends.length} 位好友 · {tags.length} 个分组标签
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void onRefreshContacts()}>
          刷新列表
        </Button>
      </div>

      {errorMsg ? (
        <div className="mx-6 mt-4 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {errorMsg}
        </div>
      ) : null}

      {feedback ? (
        <div className="mx-6 mt-4 rounded-md border bg-muted px-3 py-2 text-xs text-foreground">
          {feedback}
        </div>
      ) : null}

      <Tabs defaultValue="friends" className="flex-1 flex flex-col overflow-hidden p-6">
        <TabsList className="w-fit">
          <TabsTrigger value="friends">
            我的好友 ({friends.length})
          </TabsTrigger>
          <TabsTrigger value="applications" className="gap-1.5">
            <span>好友申请</span>
            {pendingIncomingCount > 0 ? (
              <Badge variant="destructive" className="h-4 px-1.5 text-[10px]">
                {pendingIncomingCount}
              </Badge>
            ) : null}
          </TabsTrigger>
          <TabsTrigger value="discover">添加好友</TabsTrigger>
        </TabsList>

        <TabsContent value="friends" className="flex-1 flex gap-6 overflow-hidden pt-4">
          <div className="w-64 shrink-0 flex flex-col rounded-lg border bg-card p-3">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-semibold text-muted-foreground flex items-center gap-1.5">
                <TagIcon className="size-3.5" />
                好友标签
              </span>
            </div>

            <form onSubmit={handleCreateTag} className="flex gap-1.5 mb-3">
              <Input
                value={newTagName}
                onChange={(e) => setNewTagName(e.target.value)}
                placeholder="新建标签名称..."
                className="h-8 text-xs"
                maxLength={64}
              />
              <Button
                type="submit"
                size="sm"
                className="h-8 px-2.5"
                disabled={!newTagName.trim() || busyAction === "create-tag"}
              >
                <PlusIcon className="size-3.5" />
              </Button>
            </form>

            <Separator className="mb-2" />

            <ScrollArea className="flex-1">
              <div className="space-y-1">
                <button
                  type="button"
                  onClick={() => setSelectedTagId(null)}
                  className={cn(
                    "w-full flex items-center justify-between rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors",
                    selectedTagId === null
                      ? "bg-primary text-primary-foreground"
                      : "hover:bg-accent text-foreground"
                  )}
                >
                  <span>全部好友</span>
                  <span>{friends.length}</span>
                </button>

                {tags.map((tag) => {
                  const count = friends.filter((f) =>
                    f.tags?.some((t) => t.id === tag.id)
                  ).length
                  const isEditing = editingTagId === tag.id
                  return (
                    <div
                      key={tag.id}
                      className={cn(
                        "group flex items-center justify-between gap-1 rounded-md px-2 py-1 text-xs transition-colors",
                        selectedTagId === tag.id
                          ? "bg-secondary font-medium"
                          : "hover:bg-accent"
                      )}
                    >
                      {isEditing ? (
                        <div className="flex items-center gap-1 w-full">
                          <Input
                            value={editingTagName}
                            onChange={(e) => setEditingTagName(e.target.value)}
                            className="h-6 text-xs px-1.5"
                            autoFocus
                          />
                          <Button
                            type="button"
                            size="icon"
                            variant="ghost"
                            className="size-6"
                            onClick={() => void handleRenameTag(tag.id)}
                          >
                            <CheckIcon className="size-3" />
                          </Button>
                          <Button
                            type="button"
                            size="icon"
                            variant="ghost"
                            className="size-6"
                            onClick={() => setEditingTagId(null)}
                          >
                            <XIcon className="size-3" />
                          </Button>
                        </div>
                      ) : (
                        <>
                          <button
                            type="button"
                            onClick={() => setSelectedTagId(tag.id)}
                            className="flex-1 text-left truncate py-0.5"
                          >
                            {tag.name} ({count})
                          </button>
                          <div className="flex items-center opacity-0 group-hover:opacity-100 transition-opacity">
                            <Button
                              type="button"
                              size="icon"
                              variant="ghost"
                              className="size-6"
                              onClick={() => {
                                setEditingTagId(tag.id)
                                setEditingTagName(tag.name)
                              }}
                            >
                              <PencilIcon className="size-3" />
                            </Button>
                            <Button
                              type="button"
                              size="icon"
                              variant="ghost"
                              className="size-6 text-destructive"
                              onClick={() => void handleDeleteTag(tag.id)}
                            >
                              <Trash2Icon className="size-3" />
                            </Button>
                          </div>
                        </>
                      )}
                    </div>
                  )
                })}
              </div>
            </ScrollArea>
          </div>

          <ScrollArea className="flex-1 rounded-lg border bg-card p-4">
            {filteredFriends.length === 0 ? (
              <div className="h-64 flex flex-col items-center justify-center text-sm text-muted-foreground">
                暂无符合条件的好友
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {filteredFriends.map((friend) => {
                  const peerId = resolvePeerUserId(friend)
                  const online = presenceMap[peerId] === "online"
                  const displayName = friend.name || `用户 #${peerId}`
                  const friendTags = friend.tags ?? []
                  return (
                    <div
                      key={`${friend.user_id}-${friend.friend_id}`}
                      className="flex items-center justify-between gap-3 rounded-lg border p-3 hover:bg-accent/40 transition-colors"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="relative">
                          <Avatar className="size-10">
                            <AvatarImage src={friend.avatar} />
                            <AvatarFallback>
                              {initialsFromName(displayName)}
                            </AvatarFallback>
                          </Avatar>
                          <span
                            className={cn(
                              "absolute bottom-0 right-0 size-2.5 rounded-full ring-2 ring-background",
                              online ? "bg-emerald-500" : "bg-zinc-400"
                            )}
                          />
                        </div>
                        <div className="min-w-0 space-y-1">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium truncate">
                              {displayName}
                            </span>
                            <span className="text-[11px] text-muted-foreground">
                              #{peerId}
                            </span>
                          </div>
                          {friend.email ? (
                            <div className="text-xs text-muted-foreground truncate">
                              {friend.email}
                            </div>
                          ) : null}
                          <div className="flex flex-wrap gap-1">
                            {friendTags.map((t) => (
                              <Badge
                                key={t.id}
                                variant="secondary"
                                className="text-[10px] px-1.5 py-0"
                              >
                                {t.name}
                              </Badge>
                            ))}
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-1.5 shrink-0">
                        <Popover>
                          <PopoverTrigger asChild>
                            <Button variant="outline" size="icon" className="size-8">
                              <TagIcon className="size-3.5" />
                            </Button>
                          </PopoverTrigger>
                          <PopoverContent align="end" className="w-52 p-3 space-y-2">
                            <div className="text-xs font-medium">
                              设置标签 ({displayName})
                            </div>
                            {tags.length === 0 ? (
                              <div className="text-xs text-muted-foreground">
                                请先在左侧创建标签
                              </div>
                            ) : (
                              <div className="space-y-1.5">
                                {tags.map((tag) => {
                                  const checked = friendTags.some(
                                    (t) => t.id === tag.id
                                  )
                                  return (
                                    <label
                                      key={tag.id}
                                      className="flex items-center gap-2 text-xs cursor-pointer"
                                    >
                                      <Checkbox
                                        checked={checked}
                                        onCheckedChange={(val) =>
                                          void handleToggleFriendTag(
                                            peerId,
                                            friendTags,
                                            tag.id,
                                            Boolean(val)
                                          )
                                        }
                                      />
                                      <span className="truncate">{tag.name}</span>
                                    </label>
                                  )
                                })}
                              </div>
                            )}
                          </PopoverContent>
                        </Popover>

                        <Button
                          size="sm"
                          className="h-8 gap-1"
                          onClick={() => void onStartDirectChat(peerId, displayName)}
                        >
                          <MessageSquarePlusIcon className="size-3.5" />
                          <span>发消息</span>
                        </Button>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </ScrollArea>
        </TabsContent>

        <TabsContent value="applications" className="flex-1 overflow-hidden pt-4">
          <ScrollArea className="h-full rounded-lg border bg-card p-4">
            {applications.length === 0 ? (
              <div className="h-64 flex items-center justify-center text-sm text-muted-foreground">
                暂无好友申请记录
              </div>
            ) : (
              <div className="space-y-2.5">
                {applications.map((app) => {
                  const isIncoming = app.friend_id === currentUserId
                  const peerId = isIncoming ? app.user_id : app.friend_id
                  const displayName = app.name || `用户 #${peerId}`
                  return (
                    <div
                      key={`${app.user_id}-${app.friend_id}`}
                      className="flex items-center justify-between gap-4 rounded-lg border p-3"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <Avatar className="size-9">
                          <AvatarImage src={app.avatar} />
                          <AvatarFallback>
                            {initialsFromName(displayName)}
                          </AvatarFallback>
                        </Avatar>
                        <div className="min-w-0">
                          <div className="text-sm font-medium">
                            {displayName}{" "}
                            <span className="text-xs text-muted-foreground">
                              (#{peerId})
                            </span>
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {isIncoming ? "向你发来好友申请" : "你发起的好友申请"} ·{" "}
                            {formatTimestamp(app.updated_at || app.created_at)}
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-2">
                        {app.status === "pending" && isIncoming ? (
                          <>
                            <Button
                              size="sm"
                              disabled={busyAction === `accept-${peerId}`}
                              onClick={() => void handleAcceptApplication(peerId)}
                            >
                              <UserCheckIcon className="size-3.5" />
                              接受
                            </Button>
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={busyAction === `reject-${peerId}`}
                              onClick={() => void handleRejectApplication(peerId)}
                            >
                              拒绝
                            </Button>
                          </>
                        ) : (
                          <Badge
                            variant={
                              app.status === "accepted"
                                ? "default"
                                : app.status === "rejected"
                                  ? "destructive"
                                  : "secondary"
                            }
                          >
                            {app.status === "accepted"
                              ? "已通过"
                              : app.status === "rejected"
                                ? "已拒绝"
                                : "待对方确认"}
                          </Badge>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </ScrollArea>
        </TabsContent>

        <TabsContent value="discover" className="flex-1 flex flex-col gap-4 overflow-hidden pt-4">
          <form onSubmit={handleSearchUsers} className="flex gap-2 max-w-lg">
            <div className="relative flex-1">
              <SearchIcon className="size-4 text-muted-foreground absolute left-3 top-2.5" />
              <Input
                value={searchName}
                onChange={(e) => setSearchName(e.target.value)}
                placeholder="按用户昵称搜索..."
                className="pl-9"
              />
            </div>
            <Button type="submit" disabled={searchingUsers || !searchName.trim()}>
              {searchingUsers ? "查找中..." : "搜索用户"}
            </Button>
          </form>

          <ScrollArea className="flex-1 rounded-lg border bg-card p-4">
            {foundUsers.length === 0 ? (
              <div className="h-56 flex items-center justify-center text-sm text-muted-foreground">
                输入用户昵称搜索并发送好友申请
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {foundUsers.map((user) => {
                  const isSelf = user.id === currentUserId
                  const alreadyFriend = friends.some(
                    (f) => resolvePeerUserId(f) === user.id
                  )
                  return (
                    <div
                      key={user.id}
                      className="flex items-center justify-between gap-3 rounded-lg border p-3"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <Avatar className="size-9">
                          <AvatarImage src={user.avatar} />
                          <AvatarFallback>
                            {initialsFromName(user.nickname)}
                          </AvatarFallback>
                        </Avatar>
                        <div className="min-w-0">
                          <div className="text-sm font-medium truncate">
                            {user.nickname} (#{user.id})
                          </div>
                          <div className="text-xs text-muted-foreground truncate">
                            {user.email}
                          </div>
                        </div>
                      </div>

                      {isSelf ? (
                        <Badge variant="outline">本人</Badge>
                      ) : alreadyFriend ? (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => void onStartDirectChat(user.id, user.nickname)}
                        >
                          发消息
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          disabled={busyAction === `add-friend-${user.id}`}
                          onClick={() => void handleSendFriendRequest(user.id)}
                        >
                          <UserPlusIcon className="size-3.5" />
                          加好友
                        </Button>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </ScrollArea>
        </TabsContent>
      </Tabs>
    </div>
  )
}
