interface SystemMessagePayload {
  event?: string
  operator_name?: string
  operator_id?: number
  target_ids?: number[]
}

/** Parse group system message JSON content into user-facing text. */
export function formatSystemMessage(content: string): string {
  if (!content.trim()) {
    return '系统消息'
  }

  try {
    const data = JSON.parse(content) as SystemMessagePayload
    const name = data.operator_name?.trim() || '有人'

    switch (data.event) {
      case 'member_joined':
        return `${name} 邀请新成员加入群聊`
      case 'member_removed':
        return `${name} 移除了群成员`
      case 'member_left':
        return `${name} 退出了群聊`
      case 'group_renamed':
        return `${name} 修改了群名称`
      case 'group_avatar_changed':
        return `${name} 修改了群头像`
      case 'group_dismissed':
        return '群聊已解散'
      default:
        return '系统消息'
    }
  } catch {
    return content
  }
}

export function isSystemMessageType(messageType: string | undefined, isSystem?: boolean): boolean {
  return isSystem === true || messageType === 'system'
}
