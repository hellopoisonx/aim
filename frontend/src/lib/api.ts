import type {
  AttachmentContentV1,
  AttachmentFileInfo,
  AuthSession,
  ConversationItem,
  DownloadAttachmentResponse,
  FriendTagItem,
  FriendshipItem,
  InitAttachmentUploadRequest,
  InitAttachmentUploadResponse,
  MemberDetailItem,
  MessageItem,
  PresenceItem,
  ReadStateItem,
  SearchResponse,
  UserInfo,
  UserListItem,
} from "@/lib/types"
import { sha256Hex } from "@/lib/utils"

interface ApiEnvelope<T> {
  code: number
  msg: string
  body: T
}

export class ApiError extends Error {
  readonly code: number
  readonly status: number

  constructor(code: number, message: string, status = 200) {
    super(message)
    this.code = code
    this.status = status
  }
}

const SESSION_STORAGE_KEY = "aim.web.session.v1"
const DEVICE_ID_STORAGE_KEY = "aim.web.device_id.v1"

export function getOrCreateDeviceId(): string {
  try {
    const existing = localStorage.getItem(DEVICE_ID_STORAGE_KEY)
    if (existing) return existing
    const created = `web-${crypto.randomUUID()}`
    localStorage.setItem(DEVICE_ID_STORAGE_KEY, created)
    return created
  } catch {
    return `web-${Math.random().toString(36).slice(2, 12)}`
  }
}

export function syncWsAccessTokenCookie(accessToken: string | null) {
  if (typeof document === "undefined") return
  if (!accessToken) {
    document.cookie = "aim_access_token=; Path=/ws; Max-Age=0; SameSite=Strict"
    return
  }
  document.cookie = `aim_access_token=${encodeURIComponent(accessToken)}; Path=/ws; SameSite=Strict`
}

export function loadStoredSession(): AuthSession | null {
  try {
    const raw = localStorage.getItem(SESSION_STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as AuthSession
    if (!parsed.accessToken || !parsed.refreshToken || !parsed.userId) {
      return null
    }
    syncWsAccessTokenCookie(parsed.accessToken)
    return parsed
  } catch {
    return null
  }
}

export function saveStoredSession(session: AuthSession | null) {
  try {
    if (!session) {
      localStorage.removeItem(SESSION_STORAGE_KEY)
      syncWsAccessTokenCookie(null)
      return
    }
    localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session))
    syncWsAccessTokenCookie(session.accessToken)
  } catch {
    syncWsAccessTokenCookie(session?.accessToken ?? null)
  }
}

let activeSession: AuthSession | null = loadStoredSession()
let inflightRefresh: Promise<AuthSession | null> | null = null
const sessionListeners = new Set<(session: AuthSession | null) => void>()

export function getSession(): AuthSession | null {
  return activeSession
}

export function setSession(session: AuthSession | null) {
  activeSession = session
  saveStoredSession(session)
  for (const listener of sessionListeners) {
    listener(session)
  }
}

export function subscribeSession(
  listener: (session: AuthSession | null) => void
): () => void {
  sessionListeners.add(listener)
  return () => {
    sessionListeners.delete(listener)
  }
}

export async function refreshSessionToken(): Promise<AuthSession | null> {
  if (!activeSession?.refreshToken) {
    setSession(null)
    return null
  }
  if (inflightRefresh) {
    return inflightRefresh
  }
  const currentSession = activeSession
  inflightRefresh = (async () => {
    try {
      const response = await fetch("/api/auth/refresh", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: currentSession.refreshToken }),
      })
      const payload = (await response.json()) as ApiEnvelope<{
        access_token: string
        refresh_token: string
        expires_at: number
      }>
      if (!response.ok || payload.code !== 0 || !payload.body?.access_token) {
        setSession(null)
        return null
      }
      const next: AuthSession = {
        ...currentSession,
        accessToken: payload.body.access_token,
        refreshToken: payload.body.refresh_token,
        expiresAt: payload.body.expires_at,
      }
      setSession(next)
      return next
    } catch {
      return null
    } finally {
      inflightRefresh = null
    }
  })()
  return inflightRefresh
}

function isAuthExpiredCode(code: number, httpStatus: number): boolean {
  return httpStatus === 401 || code === 40100 || code === 1004 || code === 1005
}

async function request<T>(
  path: string,
  init: RequestInit = {},
  options: { auth?: boolean; retryOnAuth?: boolean } = {}
): Promise<T> {
  const requireAuth = options.auth ?? true
  const allowRetry = options.retryOnAuth ?? true

  if (requireAuth && activeSession) {
    const nowSec = Math.floor(Date.now() / 1000)
    if (activeSession.expiresAt > 0 && activeSession.expiresAt - nowSec < 45) {
      await refreshSessionToken()
    }
  }

  const headers = new Headers(init.headers)
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json")
  }
  if (requireAuth && activeSession?.accessToken) {
    headers.set("Authorization", `Bearer ${activeSession.accessToken}`)
  }

  const response = await fetch(path, {
    ...init,
    headers,
  })

  let envelope: ApiEnvelope<T> | null = null
  try {
    envelope = (await response.json()) as ApiEnvelope<T>
  } catch {
    if (!response.ok) {
      throw new ApiError(response.status, response.statusText || "请求失败", response.status)
    }
    return undefined as T
  }

  const code = envelope?.code ?? (response.ok ? 0 : response.status)
  if (requireAuth && allowRetry && isAuthExpiredCode(code, response.status)) {
    const refreshed = await refreshSessionToken()
    if (refreshed) {
      return request<T>(path, init, { auth: requireAuth, retryOnAuth: false })
    }
    throw new ApiError(code, envelope?.msg || "登录状态已过期", response.status)
  }

  if (!response.ok || code !== 0) {
    throw new ApiError(code || response.status, envelope?.msg || "请求失败", response.status)
  }

  return envelope.body
}

export const aimApi = {
  async register(input: {
    email: string
    password: string
    username: string
    avatar?: string
  }): Promise<{ user_id: number }> {
    return request<{ user_id: number }>(
      "/api/auth/register",
      {
        method: "POST",
        body: JSON.stringify({
          ...input,
          device_id: getOrCreateDeviceId(),
        }),
      },
      { auth: false }
    )
  },

  async login(input: { email: string; password: string }): Promise<AuthSession> {
    const deviceId = getOrCreateDeviceId()
    const body = await request<{
      user_id: number
      access_token: string
      refresh_token: string
      expires_at: number
    }>(
      "/api/auth/login",
      {
        method: "POST",
        body: JSON.stringify({
          email: input.email,
          password: input.password,
          device_id: deviceId,
        }),
      },
      { auth: false }
    )
    const session: AuthSession = {
      userId: body.user_id,
      accessToken: body.access_token,
      refreshToken: body.refresh_token,
      expiresAt: body.expires_at,
      deviceId,
      email: input.email,
    }
    setSession(session)
    return session
  },

  async logout(): Promise<void> {
    try {
      await request<{ success: boolean }>(
        "/api/auth/logout",
        { method: "POST" },
        { retryOnAuth: false }
      )
    } finally {
      setSession(null)
    }
  },

  async getUserById(id: number): Promise<UserInfo> {
    const res = await request<{ user: UserInfo }>(`/api/users/by-id/${id}`)
    return res.user
  },

  async searchUsersByName(name: string): Promise<UserListItem[]> {
    const res = await request<{ users: UserListItem[] }>(
      `/api/users/by-name/${encodeURIComponent(name)}`
    )
    return res.users ?? []
  },

  async addFriend(userId: number): Promise<FriendshipItem> {
    const res = await request<{ friendship: FriendshipItem }>(
      `/api/users/friends/${userId}`,
      { method: "POST" }
    )
    return res.friendship
  },

  async listFriends(): Promise<FriendshipItem[]> {
    const res = await request<{ friends: FriendshipItem[] }>("/api/friends/me")
    return res.friends ?? []
  },

  async listFriendApplications(): Promise<FriendshipItem[]> {
    const res = await request<{ applications: FriendshipItem[] }>(
      "/api/friends/applications"
    )
    return res.applications ?? []
  },

  async acceptFriend(userId: number): Promise<FriendshipItem> {
    const res = await request<{ friendship: FriendshipItem }>(
      `/api/friends/accept/${userId}`,
      { method: "POST" }
    )
    return res.friendship
  },

  async rejectFriend(userId: number): Promise<FriendshipItem> {
    const res = await request<{ friendship: FriendshipItem }>(
      `/api/friends/reject/${userId}`,
      { method: "POST" }
    )
    return res.friendship
  },

  async listFriendTags(): Promise<FriendTagItem[]> {
    const res = await request<{ tags: FriendTagItem[] }>("/api/friends/tags")
    return res.tags ?? []
  },

  async createFriendTag(name: string): Promise<FriendTagItem> {
    const res = await request<{ tag: FriendTagItem }>("/api/friends/tags", {
      method: "POST",
      body: JSON.stringify({ name }),
    })
    return res.tag
  },

  async renameFriendTag(id: number, name: string): Promise<FriendTagItem> {
    const res = await request<{ tag: FriendTagItem }>(`/api/friends/tags/${id}`, {
      method: "PUT",
      body: JSON.stringify({ name }),
    })
    return res.tag
  },

  async deleteFriendTag(id: number): Promise<boolean> {
    const res = await request<{ deleted: boolean }>(`/api/friends/tags/${id}`, {
      method: "DELETE",
    })
    return res.deleted
  },

  async setFriendTags(friendId: number, tagIds: number[]): Promise<FriendshipItem> {
    const res = await request<{ friendship: FriendshipItem }>(
      `/api/friends/${friendId}/tags`,
      {
        method: "PUT",
        body: JSON.stringify({ tag_ids: tagIds }),
      }
    )
    return res.friendship
  },

  async removeFriendTag(friendId: number, tagId: number): Promise<FriendshipItem> {
    const res = await request<{ friendship: FriendshipItem }>(
      `/api/friends/${friendId}/tags/${tagId}`,
      { method: "DELETE" }
    )
    return res.friendship
  },

  async getFriendsPresence(): Promise<PresenceItem[]> {
    const res = await request<{ presences: PresenceItem[] }>("/api/presence/friends")
    return res.presences ?? []
  },

  async listConversations(): Promise<ConversationItem[]> {
    const res = await request<{ conversations: ConversationItem[] }>(
      "/api/conversations"
    )
    return res.conversations ?? []
  },

  async createConversation(input: {
    conversation_type: "direct" | "group"
    member_ids: number[]
    name?: string
    avatar?: string
  }): Promise<ConversationItem> {
    return request<ConversationItem>("/api/conversations", {
      method: "POST",
      body: JSON.stringify(input),
    })
  },

  async createGroup(input: {
    member_ids: number[]
    name: string
    avatar?: string
  }): Promise<ConversationItem> {
    return request<ConversationItem>("/api/conversations/group", {
      method: "POST",
      body: JSON.stringify(input),
    })
  },

  async getConversationHistory(params: {
    conversationId: number
    cursorCreatedAt?: number
    cursorId?: number
    limit?: number
  }): Promise<{
    messages: MessageItem[]
    next_cursor_created_at: number
    next_cursor_id: number
    has_more: boolean
    read_states: ReadStateItem[]
  }> {
    const query = new URLSearchParams()
    if (params.cursorCreatedAt) {
      query.set("cursor_created_at", String(params.cursorCreatedAt))
    }
    if (params.cursorId) {
      query.set("cursor_id", String(params.cursorId))
    }
    query.set("limit", String(params.limit ?? 50))
    const qs = query.toString()
    return request(`/api/conversations/history/${params.conversationId}?${qs}`)
  },

  async getConversationMembers(conversationId: number): Promise<MemberDetailItem[]> {
    const res = await request<{ members: MemberDetailItem[] }>(
      `/api/conversations/${conversationId}/members`
    )
    return res.members ?? []
  },

  async addGroupMembers(
    conversationId: number,
    memberIds: number[]
  ): Promise<ConversationItem> {
    return request<ConversationItem>(
      `/api/conversations/${conversationId}/members`,
      {
        method: "POST",
        body: JSON.stringify({ member_ids: memberIds }),
      }
    )
  },

  async removeGroupMember(
    conversationId: number,
    userId: number
  ): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(
      `/api/conversations/${conversationId}/members/${userId}`,
      { method: "DELETE" }
    )
  },

  async grantGroupAdmin(
    conversationId: number,
    userId: number
  ): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(
      `/api/conversations/${conversationId}/members/${userId}/admin`,
      { method: "POST" }
    )
  },

  async revokeGroupAdmin(
    conversationId: number,
    userId: number
  ): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(
      `/api/conversations/${conversationId}/members/${userId}/admin`,
      { method: "DELETE" }
    )
  },

  async transferGroupOwner(
    conversationId: number,
    userId: number
  ): Promise<ConversationItem> {
    return request<ConversationItem>(
      `/api/conversations/${conversationId}/owner`,
      {
        method: "POST",
        body: JSON.stringify({ user_id: userId }),
      }
    )
  },

  async leaveGroup(conversationId: number): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(
      `/api/conversations/${conversationId}/leave`,
      { method: "POST" }
    )
  },

  async dismissGroup(conversationId: number): Promise<{ success: boolean }> {
    return request<{ success: boolean }>(`/api/conversations/${conversationId}`, {
      method: "DELETE",
    })
  },

  async updateGroupInfo(
    conversationId: number,
    input: { name?: string; avatar?: string }
  ): Promise<ConversationItem> {
    return request<ConversationItem>(`/api/conversations/${conversationId}`, {
      method: "PUT",
      body: JSON.stringify(input),
    })
  },

  async initAttachmentUpload(
    input: InitAttachmentUploadRequest
  ): Promise<InitAttachmentUploadResponse> {
    return request<InitAttachmentUploadResponse>("/api/attachments/init", {
      method: "POST",
      body: JSON.stringify(input),
    })
  },

  async completeAttachmentUpload(
    fileId: string,
    sha256?: string
  ): Promise<AttachmentFileInfo> {
    return request<AttachmentFileInfo>(`/api/attachments/${fileId}/complete`, {
      method: "POST",
      body: JSON.stringify({ sha256 }),
    })
  },

  async getAttachment(fileId: string): Promise<AttachmentFileInfo> {
    return request<AttachmentFileInfo>(`/api/attachments/${fileId}`)
  },

  async downloadAttachment(fileId: string): Promise<DownloadAttachmentResponse> {
    return request<DownloadAttachmentResponse>(
      `/api/attachments/${fileId}/download`
    )
  },

  async uploadFileForConversation(
    conversationId: number,
    file: File
  ): Promise<AttachmentContentV1> {
    const buffer = await file.arrayBuffer()
    const sha256 = await sha256Hex(buffer)
    const mime = file.type || "application/octet-stream"
    let kind: "image" | "video" | "audio" | "file" = "file"
    if (mime.startsWith("image/")) kind = "image"
    else if (mime.startsWith("video/")) kind = "video"
    else if (mime.startsWith("audio/")) kind = "audio"

    const initRes = await this.initAttachmentUpload({
      conversation_id: conversationId,
      kind,
      original_name: file.name,
      mime,
      size: file.size,
      sha256,
    })

    const uploadHeaders = new Headers(initRes.headers ?? {})
    if (!uploadHeaders.has("Content-Type")) {
      uploadHeaders.set("Content-Type", mime)
    }
    const uploadResponse = await fetch(initRes.upload_url, {
      method: initRes.upload_method || "PUT",
      headers: uploadHeaders,
      body: buffer,
    })
    if (!uploadResponse.ok) {
      throw new ApiError(
        uploadResponse.status,
        `上传文件失败 (${uploadResponse.status})`,
        uploadResponse.status
      )
    }

    const completed = await this.completeAttachmentUpload(initRes.file_id, sha256)
    return {
      schema: "aim.attachment.v1",
      file_id: completed.file_id,
      kind: completed.kind,
      original: {
        name: completed.original_name || file.name,
        mime: completed.mime || mime,
        size: completed.size || file.size,
        sha256: completed.sha256 || sha256,
      },
      thumbnail_file_id: completed.thumbnail_object_key,
      parse_status: completed.parse_status,
      duration_ms: completed.duration_ms,
      width: completed.width,
      height: completed.height,
      metadata: completed.metadata,
    }
  },

  async search(params: {
    q: string
    scope?: string
    conversationId?: number
    cursorCreatedAt?: number
    cursorId?: number
    limit?: number
  }): Promise<SearchResponse> {
    const qs = new URLSearchParams()
    qs.set("q", params.q)
    if (params.scope) qs.set("scope", params.scope)
    if (params.conversationId) qs.set("conversation_id", String(params.conversationId))
    if (params.cursorCreatedAt) {
      qs.set("cursor_created_at", String(params.cursorCreatedAt))
    }
    if (params.cursorId) qs.set("cursor_id", String(params.cursorId))
    qs.set("limit", String(params.limit ?? 20))
    return request<SearchResponse>(`/api/search?${qs.toString()}`)
  },
}
