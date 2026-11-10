import {
  CrownIcon,
  LogOutIcon,
  ShieldAlertIcon,
  ShieldCheckIcon,
  Trash2Icon,
  UserMinusIcon,
  UserPlusIcon,
} from "lucide-react"
import { type SyntheticEvent, useEffect, useState } from "react"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { aimApi } from "@/lib/api"
import type {
  ConversationItem,
  FriendshipItem,
  MemberDetailItem,
} from "@/lib/types"
import { initialsFromName } from "@/lib/utils"

interface CreateConversationDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentUserId: number
  friends: FriendshipItem[]
  onCreated: (conversation: ConversationItem) => void
}

export function CreateConversationDialog({
  open,
  onOpenChange,
  currentUserId,
  friends,
  onCreated,
}: CreateConversationDialogProps) {
  const [mode, setMode] = useState<"group" | "direct">("group")
  const [groupName, setGroupName] = useState("")
  const [groupAvatar, setGroupAvatar] = useState("")
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [manualUserId, setManualUserId] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  function toggleMember(userId: number, checked: boolean) {
    if (mode === "direct") {
      setSelectedIds(checked ? [userId] : [])
      return
    }
    setSelectedIds((prev) =>
      checked ? Array.from(new Set([...prev, userId])) : prev.filter((id) => id !== userId)
    )
  }

  function handleAddManualId() {
    const parsed = Number.parseInt(manualUserId.trim(), 10)
    if (!Number.isFinite(parsed) || parsed <= 0 || parsed === currentUserId) {
      return
    }
    toggleMember(parsed, true)
    setManualUserId("")
  }

  async function handleSubmit(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault()
    setErrorMsg(null)
    if (selectedIds.length === 0) {
      setErrorMsg("请至少选择或输入 1 位成员")
      return
    }
    if (mode === "group" && !groupName.trim()) {
      setErrorMsg("请输入群聊名称")
      return
    }
    setSubmitting(true)
    try {
      const created =
        mode === "group"
          ? await aimApi.createGroup({
            member_ids: selectedIds,
            name: groupName.trim(),
            avatar: groupAvatar.trim() || undefined,
          })
          : await aimApi.createConversation({
            conversation_type: "direct",
            member_ids: [selectedIds[0]],
          })
      onCreated(created)
      onOpenChange(false)
      setGroupName("")
      setGroupAvatar("")
      setSelectedIds([])
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "创建会话失败")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>发起会话</DialogTitle>
          <DialogDescription>创建新的群聊或发起一对一单聊</DialogDescription>
        </DialogHeader>

        <Tabs
          value={mode}
          onValueChange={(v) => {
            setMode(v as "group" | "direct")
            setSelectedIds([])
            setErrorMsg(null)
          }}
        >
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="group">创建群聊</TabsTrigger>
            <TabsTrigger value="direct">发起单聊</TabsTrigger>
          </TabsList>

          <form onSubmit={handleSubmit} className="space-y-4 pt-3">
            {errorMsg ? (
              <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                {errorMsg}
              </div>
            ) : null}

            <TabsContent value="group" className="space-y-3 mt-0">
              <div className="space-y-1.5">
                <Label htmlFor="group-name">群聊名称</Label>
                <Input
                  id="group-name"
                  placeholder="例如：架构设计讨论组"
                  value={groupName}
                  onChange={(e) => setGroupName(e.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="group-avatar">群头像 URL（可选）</Label>
                <Input
                  id="group-avatar"
                  placeholder="https://..."
                  value={groupAvatar}
                  onChange={(e) => setGroupAvatar(e.target.value)}
                />
              </div>
            </TabsContent>

            <div className="space-y-2">
              <Label>选择成员 (已选 {selectedIds.length})</Label>
              <div className="flex gap-2">
                <Input
                  placeholder="输入用户 ID 添加..."
                  value={manualUserId}
                  onChange={(e) => setManualUserId(e.target.value)}
                  className="h-8 text-xs"
                />
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  className="h-8"
                  onClick={handleAddManualId}
                >
                  添加 ID
                </Button>
              </div>

              {selectedIds.length > 0 ? (
                <div className="flex flex-wrap gap-1 pt-1">
                  {selectedIds.map((id) => (
                    <Badge
                      key={id}
                      variant="secondary"
                      className="cursor-pointer"
                      onClick={() => toggleMember(id, false)}
                    >
                      #{id} ×
                    </Badge>
                  ))}
                </div>
              ) : null}

              <ScrollArea className="h-44 rounded-md border p-2">
                {friends.length === 0 ? (
                  <div className="h-36 flex items-center justify-center text-xs text-muted-foreground">
                    暂无好友，可通过上方输入框按用户 ID 发起
                  </div>
                ) : (
                  <div className="space-y-1.5">
                    {friends.map((f) => {
                      const peerId =
                        f.user_id === currentUserId ? f.friend_id : f.user_id
                      const name = f.name || `用户 #${peerId}`
                      const checked = selectedIds.includes(peerId)
                      return (
                        <label
                          key={peerId}
                          className="flex items-center justify-between rounded-md px-2 py-1.5 hover:bg-accent cursor-pointer"
                        >
                          <div className="flex items-center gap-2.5 min-w-0">
                            <Checkbox
                              checked={checked}
                              onCheckedChange={(v) =>
                                toggleMember(peerId, Boolean(v))
                              }
                            />
                            <Avatar className="size-7">
                              <AvatarImage src={f.avatar} />
                              <AvatarFallback>
                                {initialsFromName(name)}
                              </AvatarFallback>
                            </Avatar>
                            <span className="text-xs font-medium truncate">
                              {name} (#{peerId})
                            </span>
                          </div>
                        </label>
                      )
                    })}
                  </div>
                )}
              </ScrollArea>
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                取消
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting
                  ? "创建中..."
                  : mode === "group"
                    ? "创建群聊"
                    : "发起单聊"}
              </Button>
            </DialogFooter>
          </form>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}

interface GroupSettingsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  conversation: ConversationItem
  currentUserId: number
  friends: FriendshipItem[]
  onConversationUpdated: (updated: ConversationItem) => void
  onConversationRemoved: (conversationId: number) => void
}

export function GroupSettingsDialog({
  open,
  onOpenChange,
  conversation,
  currentUserId,
  friends,
  onConversationUpdated,
  onConversationRemoved,
}: GroupSettingsDialogProps) {
  const [members, setMembers] = useState<MemberDetailItem[]>([])
  const [loadingMembers, setLoadingMembers] = useState(false)
  const [editName, setEditName] = useState(conversation.name)
  const [editAvatar, setEditAvatar] = useState(conversation.avatar)
  const [inviteUserId, setInviteUserId] = useState("")
  const [busyAction, setBusyAction] = useState<string | null>(null)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  const myRole =
    members.find((m) => m.user_id === currentUserId)?.role ??
    (conversation.creator_id === currentUserId ? "owner" : "member")
  const isOwner = myRole === "owner"
  const canManage = isOwner || myRole === "admin"

  async function loadMembers() {
    setLoadingMembers(true)
    setErrorMsg(null)
    try {
      const list = await aimApi.getConversationMembers(
        conversation.conversation_id
      )
      setMembers(list)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "加载群成员失败")
    } finally {
      setLoadingMembers(false)
    }
  }

  useEffect(() => {
    if (open) {
      setEditName(conversation.name)
      setEditAvatar(conversation.avatar)
      void loadMembers()
    }
  }, [open, conversation.conversation_id])

  async function handleSaveGroupInfo(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!canManage) return
    setBusyAction("save-info")
    setErrorMsg(null)
    try {
      const updated = await aimApi.updateGroupInfo(
        conversation.conversation_id,
        {
          name: editName.trim() || undefined,
          avatar: editAvatar.trim() || undefined,
        }
      )
      onConversationUpdated(updated)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "更新群资料失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleAddMember(userId: number) {
    setBusyAction(`add-${userId}`)
    setErrorMsg(null)
    try {
      const updated = await aimApi.addGroupMembers(
        conversation.conversation_id,
        [userId]
      )
      onConversationUpdated(updated)
      setInviteUserId("")
      await loadMembers()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "添加成员失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleRemoveMember(userId: number) {
    setBusyAction(`remove-${userId}`)
    setErrorMsg(null)
    try {
      await aimApi.removeGroupMember(conversation.conversation_id, userId)
      await loadMembers()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "移除成员失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleToggleAdmin(member: MemberDetailItem) {
    setBusyAction(`admin-${member.user_id}`)
    setErrorMsg(null)
    try {
      if (member.role === "admin") {
        await aimApi.revokeGroupAdmin(
          conversation.conversation_id,
          member.user_id
        )
      } else {
        await aimApi.grantGroupAdmin(
          conversation.conversation_id,
          member.user_id
        )
      }
      await loadMembers()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "修改管理员权限失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleTransferOwner(userId: number) {
    setBusyAction(`owner-${userId}`)
    setErrorMsg(null)
    try {
      const updated = await aimApi.transferGroupOwner(
        conversation.conversation_id,
        userId
      )
      onConversationUpdated(updated)
      await loadMembers()
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "转让群主失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleLeaveGroup() {
    setBusyAction("leave")
    setErrorMsg(null)
    try {
      await aimApi.leaveGroup(conversation.conversation_id)
      onConversationRemoved(conversation.conversation_id)
      onOpenChange(false)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "退出群聊失败")
    } finally {
      setBusyAction(null)
    }
  }

  async function handleDismissGroup() {
    setBusyAction("dismiss")
    setErrorMsg(null)
    try {
      await aimApi.dismissGroup(conversation.conversation_id)
      onConversationRemoved(conversation.conversation_id)
      onOpenChange(false)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "解散群聊失败")
    } finally {
      setBusyAction(null)
    }
  }

  const invitableFriends = friends.filter((f) => {
    const peerId = f.user_id === currentUserId ? f.friend_id : f.user_id
    return !members.some((m) => m.user_id === peerId)
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            群聊设置 · {conversation.name || `#${conversation.conversation_id}`}
          </DialogTitle>
          <DialogDescription>
            群号 #{conversation.conversation_id} · 共 {members.length} 名成员 ·
            你的角色:{" "}
            {myRole === "owner"
              ? "群主"
              : myRole === "admin"
                ? "管理员"
                : "普通成员"}
          </DialogDescription>
        </DialogHeader>

        {errorMsg ? (
          <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {errorMsg}
          </div>
        ) : null}

        {canManage ? (
          <form
            onSubmit={handleSaveGroupInfo}
            className="grid grid-cols-1 sm:grid-cols-2 gap-3 rounded-lg border p-3"
          >
            <div className="space-y-1">
              <Label className="text-xs">群名称</Label>
              <Input
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                className="h-8 text-xs"
              />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">群头像 URL</Label>
              <div className="flex gap-1.5">
                <Input
                  value={editAvatar}
                  onChange={(e) => setEditAvatar(e.target.value)}
                  className="h-8 text-xs"
                  placeholder="https://..."
                />
                <Button
                  type="submit"
                  size="sm"
                  className="h-8"
                  disabled={busyAction === "save-info"}
                >
                  保存
                </Button>
              </div>
            </div>
          </form>
        ) : null}

        {canManage ? (
          <div className="space-y-2">
            <Label className="text-xs">邀请新成员</Label>
            <div className="flex gap-2">
              <Input
                value={inviteUserId}
                onChange={(e) => setInviteUserId(e.target.value)}
                placeholder="输入用户 ID 邀请入群..."
                className="h-8 text-xs"
              />
              <Button
                type="button"
                size="sm"
                variant="secondary"
                className="h-8"
                onClick={() => {
                  const parsed = Number.parseInt(inviteUserId.trim(), 10)
                  if (Number.isFinite(parsed) && parsed > 0) {
                    void handleAddMember(parsed)
                  }
                }}
              >
                <UserPlusIcon className="size-3.5" />
                添加
              </Button>
            </div>
            {invitableFriends.length > 0 ? (
              <div className="flex flex-wrap gap-1.5 pt-1">
                {invitableFriends.slice(0, 8).map((f) => {
                  const peerId =
                    f.user_id === currentUserId ? f.friend_id : f.user_id
                  return (
                    <Button
                      key={peerId}
                      type="button"
                      variant="outline"
                      size="sm"
                      className="h-6 px-2 text-[11px]"
                      onClick={() => void handleAddMember(peerId)}
                    >
                      + {f.name || `#${peerId}`}
                    </Button>
                  )
                })}
              </div>
            ) : null}
          </div>
        ) : null}

        <Separator />

        <div className="space-y-2">
          <div className="text-xs font-semibold text-muted-foreground">
            群成员 ({members.length})
          </div>
          <ScrollArea className="h-60 rounded-md border p-2">
            {loadingMembers ? (
              <div className="h-48 flex items-center justify-center text-xs text-muted-foreground">
                正在加载成员...
              </div>
            ) : (
              <div className="space-y-1.5">
                {members.map((member) => {
                  const displayName = member.name || `用户 #${member.user_id}`
                  const isSelf = member.user_id === currentUserId
                  const canKick =
                    !isSelf &&
                    ((isOwner && member.role !== "owner") ||
                      (myRole === "admin" && member.role === "member"))
                  return (
                    <div
                      key={member.user_id}
                      className="flex items-center justify-between gap-2 rounded-md p-2 hover:bg-accent/50"
                    >
                      <div className="flex items-center gap-2.5 min-w-0">
                        <Avatar className="size-8">
                          <AvatarImage src={member.avatar} />
                          <AvatarFallback>
                            {initialsFromName(displayName)}
                          </AvatarFallback>
                        </Avatar>
                        <div className="min-w-0">
                          <div className="flex items-center gap-1.5">
                            <span className="text-xs font-medium truncate">
                              {displayName}
                            </span>
                            <span className="text-[11px] text-muted-foreground">
                              #{member.user_id}
                            </span>
                            {member.role === "owner" ? (
                              <Badge variant="default" className="h-4 px-1 text-[10px]">
                                群主
                              </Badge>
                            ) : member.role === "admin" ? (
                              <Badge variant="secondary" className="h-4 px-1 text-[10px]">
                                管理员
                              </Badge>
                            ) : null}
                          </div>
                          <div className="text-[11px] text-muted-foreground truncate">
                            {member.email}
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-1 shrink-0">
                        {isOwner && !isSelf ? (
                          <>
                            <Button
                              type="button"
                              size="sm"
                              variant="ghost"
                              className="h-7 px-2 text-xs"
                              onClick={() => void handleToggleAdmin(member)}
                              title={
                                member.role === "admin"
                                  ? "撤销管理员"
                                  : "设为管理员"
                              }
                            >
                              {member.role === "admin" ? (
                                <ShieldAlertIcon className="size-3.5" />
                              ) : (
                                <ShieldCheckIcon className="size-3.5" />
                              )}
                            </Button>
                            <Button
                              type="button"
                              size="sm"
                              variant="ghost"
                              className="h-7 px-2 text-xs"
                              onClick={() =>
                                void handleTransferOwner(member.user_id)
                              }
                              title="转让群主"
                            >
                              <CrownIcon className="size-3.5" />
                            </Button>
                          </>
                        ) : null}

                        {canKick ? (
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="h-7 px-2 text-xs text-destructive"
                            onClick={() =>
                              void handleRemoveMember(member.user_id)
                            }
                            title="移出群聊"
                          >
                            <UserMinusIcon className="size-3.5" />
                          </Button>
                        ) : null}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </ScrollArea>
        </div>

        <DialogFooter className="flex items-center justify-between sm:justify-between">
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busyAction === "leave"}
              onClick={() => void handleLeaveGroup()}
            >
              <LogOutIcon className="size-3.5" />
              退出群聊
            </Button>
            {isOwner ? (
              <Button
                type="button"
                variant="destructive"
                size="sm"
                disabled={busyAction === "dismiss"}
                onClick={() => void handleDismissGroup()}
              >
                <Trash2Icon className="size-3.5" />
                解散群聊
              </Button>
            ) : null}
          </div>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => onOpenChange(false)}
          >
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
