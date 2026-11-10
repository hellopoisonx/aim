import { getSession, refreshSessionToken, syncWsAccessTokenCookie } from "@/lib/api"
import type { EntityId } from "@/lib/types"
import { hasId } from "@/lib/utils"
import {
  AckStatus,
  FrameType,
  type FrameTypeValue,
  type PushFriendApplicationPayload,
  type PushMessagePayload,
  type PushNotificationPayload,
  type PushPresencePayload,
  type PushReadReceiptPayload,
  type PushTypingPayload,
  type SendMessagePayload,
  type ServerAckPayload,
  decodePushFriendApplicationPayload,
  decodePushMessagePayload,
  decodePushNotificationPayload,
  decodePushPresencePayload,
  decodePushReadReceiptPayload,
  decodePushTypingPayload,
  decodeReconnectPayload,
  decodeServerAckPayload,
  decodeTokenExpiredPayload,
  decodeWsFrame,
  encodeClientAckPayload,
  encodeHeartbeatPayload,
  encodeReadReceiptPayload,
  encodeSendMessagePayload,
  encodeTypingPayload,
  encodeWsFrame,
  isReplayableServerFrame,
} from "@/lib/proto"

export type WsConnectionStatus =
  | "disconnected"
  | "connecting"
  | "connected"
  | "reconnecting"

export interface WsEventHandlers {
  onStatusChange?: (status: WsConnectionStatus) => void
  onReconnected?: () => void
  onPushMessage?: (payload: PushMessagePayload) => void
  onPushPresence?: (payload: PushPresencePayload) => void
  onPushTyping?: (payload: PushTypingPayload) => void
  onPushReadReceipt?: (payload: PushReadReceiptPayload) => void
  onPushFriendApplication?: (payload: PushFriendApplicationPayload) => void
  onPushNotification?: (payload: PushNotificationPayload) => void
  onServerAck?: (payload: ServerAckPayload) => void
}

type TimerHandle = ReturnType<typeof globalThis.setTimeout>

interface PendingOutbound {
  seq: number
  payload: SendMessagePayload
  attempts: number
  resolve: (ack: ServerAckPayload) => void
  reject: (err: Error) => void
  timer?: TimerHandle
}

export class AimWsClient {
  private ws: WebSocket | null = null
  private clientSeq = 0
  private lastContiguousReplaySeq = 0
  private receivedReplaySeqs = new Set<number>()
  private status: WsConnectionStatus = "disconnected"
  private reconnectAttempts = 0
  private reconnectTimer: TimerHandle | undefined = undefined
  private heartbeatTimer: TimerHandle | undefined = undefined
  private lastServerActivityAt = 0
  private manuallyStopped = false
  private hasConnectedOnce = false
  private pendingMessages = new Map<string, PendingOutbound>()
  private handlers: WsEventHandlers = {}

  setHandlers(handlers: WsEventHandlers) {
    this.handlers = handlers
  }

  getStatus(): WsConnectionStatus {
    return this.status
  }

  connect() {
    this.manuallyStopped = false
    const session = getSession()
    if (!session?.accessToken) {
      this.updateStatus("disconnected")
      return
    }
    if (
      this.ws &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return
    }

    clearTimeout(this.reconnectTimer)
    this.reconnectTimer = undefined
    this.updateStatus(this.hasConnectedOnce ? "reconnecting" : "connecting")
    syncWsAccessTokenCookie(session.accessToken)

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
    const wsUrl = `${protocol}//${window.location.host}/ws`
    const socket = new WebSocket(wsUrl)
    socket.binaryType = "arraybuffer"
    this.ws = socket

    socket.onopen = () => {
      if (this.ws !== socket) return
      const isReconnect = this.hasConnectedOnce
      this.hasConnectedOnce = true
      this.reconnectAttempts = 0
      this.lastContiguousReplaySeq = 0
      this.receivedReplaySeqs.clear()
      this.lastServerActivityAt = Date.now()
      this.updateStatus("connected")
      this.startHeartbeat()
      for (const entry of this.pendingMessages.values()) {
        this.transmitPendingMessage(entry)
      }
      if (isReconnect) {
        this.handlers.onReconnected?.()
      }
    }

    socket.onmessage = (event) => {
      if (this.ws !== socket) return
      if (!(event.data instanceof ArrayBuffer)) return
      this.lastServerActivityAt = Date.now()
      this.handleRawFrame(new Uint8Array(event.data))
    }

    socket.onerror = () => {
      if (this.ws !== socket) return
    }

    socket.onclose = (event) => {
      if (this.ws === socket) {
        this.ws = null
      }
      this.stopHeartbeat()
      if (this.manuallyStopped || event.code === 1000) {
        this.updateStatus("disconnected")
        return
      }
      if (event.code === 1008) {
        void this.refreshAndScheduleReconnect(500)
        return
      }
      this.scheduleReconnect()
    }
  }

  disconnect() {
    this.manuallyStopped = true
    this.hasConnectedOnce = false
    clearTimeout(this.reconnectTimer)
    this.reconnectTimer = undefined
    this.stopHeartbeat()
    if (this.ws) {
      const sock = this.ws
      this.ws = null
      try {
        sock.close(1000, "client logout")
      } catch { }
    }
    for (const item of this.pendingMessages.values()) {
      clearTimeout(item.timer)
      item.reject(new Error("连接已断开"))
    }
    this.pendingMessages.clear()
    this.updateStatus("disconnected")
  }

  sendChatMessage(payload: SendMessagePayload): Promise<ServerAckPayload> {
    const { promise, resolve, reject } = Promise.withResolvers<ServerAckPayload>()
    const existing = this.pendingMessages.get(payload.client_msg_id)
    clearTimeout(existing?.timer)
    this.clientSeq += 1
    const entry: PendingOutbound = {
      seq: this.clientSeq,
      payload,
      attempts: 1,
      resolve,
      reject,
    }
    this.pendingMessages.set(payload.client_msg_id, entry)
    this.transmitPendingMessage(entry)
    return promise
  }

  sendTyping(conversationId: EntityId) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    this.sendFrame(
      FrameType.TYPING,
      encodeTypingPayload({ conversation_id: conversationId })
    )
  }

  sendReadReceipt(conversationId: EntityId, lastReadMessageId: EntityId) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN || !hasId(lastReadMessageId)) {
      return
    }
    this.sendFrame(
      FrameType.READ_RECEIPT,
      encodeReadReceiptPayload({
        conversation_id: conversationId,
        last_read_message_id: lastReadMessageId,
      })
    )
  }

  private transmitPendingMessage(entry: PendingOutbound) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return
    }
    this.clientSeq += 1
    entry.seq = this.clientSeq
    this.ws.send(
      encodeWsFrame({
        type: FrameType.SEND_MESSAGE,
        seq: entry.seq,
        payload: encodeSendMessagePayload(entry.payload),
        timestamp: Date.now(),
      })
    )
    clearTimeout(entry.timer)
    entry.timer = setTimeout(() => {
      if (!this.pendingMessages.has(entry.payload.client_msg_id)) return
      if (entry.attempts < 4) {
        entry.attempts += 1
        this.transmitPendingMessage(entry)
      } else {
        this.pendingMessages.delete(entry.payload.client_msg_id)
        entry.reject(new Error("发送超时，请点击重发"))
      }
    }, 6000)
  }

  private handleRawFrame(bytes: Uint8Array) {
    let frame
    try {
      frame = decodeWsFrame(bytes)
    } catch {
      return
    }

    if (isReplayableServerFrame(frame.type) && frame.seq > 0) {
      this.recordReplaySeq(frame.seq)
      this.sendFrame(
        FrameType.ACK,
        encodeClientAckPayload({ ack_seq: this.lastContiguousReplaySeq || frame.seq })
      )
    }

    switch (frame.type) {
      case FrameType.PUSH_MESSAGE: {
        const payload = decodePushMessagePayload(frame.payload)
        const pending = payload.client_msg_id
          ? this.pendingMessages.get(payload.client_msg_id)
          : undefined
        if (pending && hasId(payload.message_id)) {
          clearTimeout(pending.timer)
          this.pendingMessages.delete(payload.client_msg_id)
          pending.resolve({
            ack_seq: pending.seq,
            client_msg_id: payload.client_msg_id,
            code: 0,
            msg: "ok",
            status: AckStatus.ACCEPTED,
            message_id: payload.message_id,
          })
        }
        this.handlers.onPushMessage?.(payload)
        break
      }
      case FrameType.PUSH_PRESENCE: {
        const payload = decodePushPresencePayload(frame.payload)
        this.handlers.onPushPresence?.(payload)
        break
      }
      case FrameType.PUSH_TYPING: {
        const payload = decodePushTypingPayload(frame.payload)
        this.handlers.onPushTyping?.(payload)
        break
      }
      case FrameType.PUSH_READ_RECEIPT: {
        const payload = decodePushReadReceiptPayload(frame.payload)
        this.handlers.onPushReadReceipt?.(payload)
        break
      }
      case FrameType.PUSH_FRIEND_APPLICATION: {
        const payload = decodePushFriendApplicationPayload(frame.payload)
        this.handlers.onPushFriendApplication?.(payload)
        break
      }
      case FrameType.PUSH_NOTIFICATION: {
        const payload = decodePushNotificationPayload(frame.payload)
        this.handlers.onPushNotification?.(payload)
        break
      }
      case FrameType.SERVER_ACK: {
        const ack = decodeServerAckPayload(frame.payload)
        this.handleServerAck(ack)
        this.handlers.onServerAck?.(ack)
        break
      }
      case FrameType.TOKEN_EXPIRED: {
        decodeTokenExpiredPayload(frame.payload)
        void this.refreshAndScheduleReconnect(100)
        break
      }
      case FrameType.RECONNECT: {
        const reconnect = decodeReconnectPayload(frame.payload)
        const delay = Math.max(100, Math.min(reconnect.reconnect_delay_ms || 500, 10000))
        this.forceCloseSocket()
        this.scheduleReconnect(delay)
        break
      }
      default:
        break
    }
  }

  private handleServerAck(ack: ServerAckPayload) {
    let matched: PendingOutbound | undefined
    if (ack.client_msg_id) {
      matched = this.pendingMessages.get(ack.client_msg_id)
    }
    if (!matched && ack.ack_seq > 0) {
      for (const item of this.pendingMessages.values()) {
        if (item.seq === ack.ack_seq) {
          matched = item
          break
        }
      }
    }
    if (!matched) return

    clearTimeout(matched.timer)

    if (ack.code === 0 || ack.status === AckStatus.ACCEPTED) {
      this.pendingMessages.delete(matched.payload.client_msg_id)
      matched.resolve(ack)
      return
    }

    if (ack.code === 40100) {
      void this.refreshAndScheduleReconnect(100)
    }

    if (
      (ack.status === AckStatus.RETRYABLE || ack.code === 42900 || ack.code === 50000) &&
      matched.attempts < 5
    ) {
      const delay = Math.min(1000 * 2 ** (matched.attempts - 1), 8000)
      matched.attempts += 1
      matched.timer = setTimeout(() => {
        this.transmitPendingMessage(matched)
      }, delay)
      return
    }

    this.pendingMessages.delete(matched.payload.client_msg_id)
    matched.reject(new Error(ack.msg || `发送失败 (${ack.code})`))
  }

  private recordReplaySeq(seq: number) {
    this.receivedReplaySeqs.add(seq)
    if (this.lastContiguousReplaySeq === 0) {
      this.lastContiguousReplaySeq = seq
    } else if (seq === this.lastContiguousReplaySeq + 1) {
      this.lastContiguousReplaySeq = seq
    }
    while (this.receivedReplaySeqs.has(this.lastContiguousReplaySeq + 1)) {
      this.lastContiguousReplaySeq += 1
    }
  }

  private startHeartbeat() {
    this.stopHeartbeat()
    this.heartbeatTimer = setInterval(() => {
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
      if (Date.now() - this.lastServerActivityAt > 75000) {
        this.forceCloseSocket()
        this.scheduleReconnect(0)
        return
      }
      this.sendFrame(
        FrameType.HEARTBEAT,
        encodeHeartbeatPayload({ last_seq: this.lastContiguousReplaySeq })
      )
    }, 25000)
  }

  private stopHeartbeat() {
    if (this.heartbeatTimer !== undefined) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = undefined
    }
  }

  private async refreshAndScheduleReconnect(delayMs: number) {
    this.forceCloseSocket()
    const refreshed = await refreshSessionToken()
    if (!refreshed) {
      this.updateStatus("disconnected")
      return
    }
    this.scheduleReconnect(delayMs)
  }

  private scheduleReconnect(explicitDelayMs?: number) {
    if (this.manuallyStopped) return
    clearTimeout(this.reconnectTimer)
    this.updateStatus("reconnecting")
    let delay = explicitDelayMs
    if (delay === undefined) {
      const schedule = [0, 1000, 2000, 4000, 10000]
      delay = schedule[Math.min(this.reconnectAttempts, schedule.length - 1)]
      this.reconnectAttempts += 1
    }
    this.reconnectTimer = setTimeout(() => {
      this.connect()
    }, delay)
  }

  private forceCloseSocket() {
    if (!this.ws) return
    const sock = this.ws
    this.ws = null
    try {
      sock.close()
    } catch { }
  }

  private sendFrame(type: FrameTypeValue, payload: Uint8Array): number {
    this.clientSeq += 1
    const seq = this.clientSeq
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(
        encodeWsFrame({
          type,
          seq,
          payload,
          timestamp: Date.now(),
        })
      )
    }
    return seq
  }

  private updateStatus(next: WsConnectionStatus) {
    if (this.status === next) return
    this.status = next
    this.handlers.onStatusChange?.(next)
  }
}

export const aimWsClient = new AimWsClient()
