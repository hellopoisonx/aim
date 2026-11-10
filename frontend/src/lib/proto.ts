import type { SenderInfo } from "@/lib/types"

export const FrameType = {
  UNSPECIFIED: 0,
  SEND_MESSAGE: 1,
  HEARTBEAT: 2,
  TYPING: 3,
  READ_RECEIPT: 4,
  ACK: 5,
  PUSH_MESSAGE: 101,
  PUSH_PRESENCE: 102,
  PUSH_NOTIFICATION: 103,
  PUSH_TYPING: 104,
  RECONNECT: 105,
  SERVER_ACK: 106,
  TOKEN_EXPIRED: 107,
  PUSH_FRIEND_APPLICATION: 108,
  PUSH_READ_RECEIPT: 109,
} as const

export type FrameTypeValue = (typeof FrameType)[keyof typeof FrameType]

export const AckStatus = {
  UNSPECIFIED: 0,
  ACCEPTED: 1,
  REJECTED: 2,
  RETRYABLE: 3,
} as const

export type AckStatusValue = (typeof AckStatus)[keyof typeof AckStatus]

export interface WsFrame {
  type: FrameTypeValue
  seq: number
  payload: Uint8Array
  timestamp: number
}

export interface SendMessagePayload {
  conversation_id: number
  message_type: string
  content: string
  client_msg_id: string
  mentions: string[]
}

export interface HeartbeatPayload {
  last_seq: number
}

export interface TypingPayload {
  conversation_id: number
}

export interface ReadReceiptPayload {
  conversation_id: number
  last_read_message_id: number
}

export interface ClientAckPayload {
  ack_seq: number
}

export interface PushMessagePayload {
  message_id: number
  conversation_id: number
  message_type: string
  content: string
  sender_id: number
  sent_at: number
  conversation_type: string
  client_msg_id: string
  is_system: boolean
  sender_info: SenderInfo
  mentions: string[]
}

export interface PushPresencePayload {
  user_id: number
  status: string
  updated_at: number
}

export interface PushNotificationPayload {
  notification_type: string
  title: string
  body: string
  related_id: number
}

export interface PushFriendApplicationPayload {
  user_id: number
  friend_id: number
  status: string
  created_at: number
  updated_at: number
}

export interface PushTypingPayload {
  user_id: number
  conversation_id: number
}

export interface PushReadReceiptPayload {
  conversation_id: number
  user_id: number
  last_read_message_id: number
  updated_at: number
}

export interface ReconnectPayload {
  reconnect_delay_ms: number
  gateway_node_id: string
}

export interface TokenExpiredPayload {
  expired_at: number
  reason: string
}

export interface ServerAckPayload {
  ack_seq: number
  client_msg_id: string
  code: number
  msg: string
  status: AckStatusValue
  message_id: number
}

const textEncoder = new TextEncoder()
const textDecoder = new TextDecoder()

class ProtoWriter {
  private chunks: number[] = []

  writeVarint(value: number | bigint) {
    let v = BigInt.asUintN(64, BigInt(Math.trunc(Number(value))))
    while (v >= 0x80n) {
      this.chunks.push(Number((v & 0x7fn) | 0x80n))
      v >>= 7n
    }
    this.chunks.push(Number(v))
  }

  writeTag(fieldNumber: number, wireType: number) {
    this.writeVarint((fieldNumber << 3) | wireType)
  }

  writeInt64Field(fieldNumber: number, value: number) {
    if (!value) return
    this.writeTag(fieldNumber, 0)
    this.writeVarint(value)
  }

  writeInt32Field(fieldNumber: number, value: number) {
    if (!value) return
    this.writeTag(fieldNumber, 0)
    this.writeVarint(value)
  }

  writeBoolField(fieldNumber: number, value: boolean) {
    if (!value) return
    this.writeTag(fieldNumber, 0)
    this.writeVarint(1)
  }

  writeBytesField(fieldNumber: number, bytes: Uint8Array) {
    if (bytes.length === 0) return
    this.writeTag(fieldNumber, 2)
    this.writeVarint(bytes.length)
    for (let i = 0; i < bytes.length; i += 1) {
      this.chunks.push(bytes[i])
    }
  }

  writeStringField(fieldNumber: number, value: string) {
    if (!value) return
    const encoded = textEncoder.encode(value)
    this.writeBytesField(fieldNumber, encoded)
  }

  writeRepeatedStringField(fieldNumber: number, values: string[]) {
    for (const item of values) {
      const encoded = textEncoder.encode(item)
      this.writeTag(fieldNumber, 2)
      this.writeVarint(encoded.length)
      for (let i = 0; i < encoded.length; i += 1) {
        this.chunks.push(encoded[i])
      }
    }
  }

  finish(): Uint8Array<ArrayBuffer> {
    return new Uint8Array(this.chunks)
  }
}

class ProtoReader {
  private pos = 0
  private readonly view: Uint8Array

  constructor(buf: Uint8Array) {
    this.view = buf
  }

  hasMore(): boolean {
    return this.pos < this.view.length
  }

  readVarintBigInt(): bigint {
    let result = 0n
    let shift = 0n
    while (this.pos < this.view.length) {
      const byte = this.view[this.pos]
      this.pos += 1
      result |= BigInt(byte & 0x7f) << shift
      if ((byte & 0x80) === 0) {
        return BigInt.asIntN(64, result)
      }
      shift += 7n
      if (shift > 63n) {
        throw new Error("Malformed varint")
      }
    }
    throw new Error("Unexpected EOF reading varint")
  }

  readInt64(): number {
    return Number(this.readVarintBigInt())
  }

  readInt32(): number {
    return Number(BigInt.asIntN(32, this.readVarintBigInt()))
  }

  readBool(): boolean {
    return this.readVarintBigInt() !== 0n
  }

  readTag(): { fieldNumber: number; wireType: number } {
    const raw = Number(this.readVarintBigInt())
    return {
      fieldNumber: raw >>> 3,
      wireType: raw & 0x07,
    }
  }

  readBytes(): Uint8Array {
    const length = this.readInt32()
    if (length < 0 || this.pos + length > this.view.length) {
      throw new Error("Invalid length-delimited field")
    }
    const slice = this.view.subarray(this.pos, this.pos + length)
    this.pos += length
    return slice
  }

  readString(): string {
    return textDecoder.decode(this.readBytes())
  }

  skip(wireType: number) {
    switch (wireType) {
      case 0:
        this.readVarintBigInt()
        break
      case 1:
        this.pos += 8
        break
      case 2: {
        const len = this.readInt32()
        this.pos += len
        break
      }
      case 5:
        this.pos += 4
        break
      default:
        throw new Error(`Unsupported wire type ${wireType}`)
    }
  }
}

export function encodeWsFrame(frame: WsFrame): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeInt32Field(1, frame.type)
  w.writeInt64Field(2, frame.seq)
  w.writeBytesField(3, frame.payload)
  w.writeInt64Field(4, frame.timestamp)
  return w.finish()
}

export function decodeWsFrame(buf: Uint8Array): WsFrame {
  const r = new ProtoReader(buf)
  const frame: WsFrame = {
    type: FrameType.UNSPECIFIED,
    seq: 0,
    payload: new Uint8Array(0),
    timestamp: 0,
  }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) {
      frame.type = r.readInt32() as FrameTypeValue
    } else if (fieldNumber === 2 && wireType === 0) {
      frame.seq = r.readInt64()
    } else if (fieldNumber === 3 && wireType === 2) {
      frame.payload = r.readBytes()
    } else if (fieldNumber === 4 && wireType === 0) {
      frame.timestamp = r.readInt64()
    } else {
      r.skip(wireType)
    }
  }
  return frame
}

export function encodeSendMessagePayload(msg: SendMessagePayload): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeInt64Field(1, msg.conversation_id)
  w.writeStringField(2, msg.message_type)
  w.writeStringField(3, msg.content)
  w.writeStringField(4, msg.client_msg_id)
  if (msg.mentions?.length) {
    w.writeRepeatedStringField(5, msg.mentions)
  }
  return w.finish()
}

export function decodeSendMessagePayload(buf: Uint8Array): SendMessagePayload {
  const r = new ProtoReader(buf)
  const out: SendMessagePayload = {
    conversation_id: 0,
    message_type: "",
    content: "",
    client_msg_id: "",
    mentions: [],
  }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.conversation_id = r.readInt64()
    else if (fieldNumber === 2 && wireType === 2) out.message_type = r.readString()
    else if (fieldNumber === 3 && wireType === 2) out.content = r.readString()
    else if (fieldNumber === 4 && wireType === 2) out.client_msg_id = r.readString()
    else if (fieldNumber === 5 && wireType === 2) out.mentions.push(r.readString())
    else r.skip(wireType)
  }
  return out
}

export function encodeHeartbeatPayload(msg: HeartbeatPayload): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeInt64Field(1, msg.last_seq)
  return w.finish()
}

export function encodeTypingPayload(msg: TypingPayload): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeInt64Field(1, msg.conversation_id)
  return w.finish()
}

export function encodeReadReceiptPayload(msg: ReadReceiptPayload): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeInt64Field(1, msg.conversation_id)
  w.writeInt64Field(2, msg.last_read_message_id)
  return w.finish()
}

export function encodeClientAckPayload(msg: ClientAckPayload): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeInt64Field(1, msg.ack_seq)
  return w.finish()
}

export function encodeSenderInfo(info: SenderInfo): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeStringField(1, info.name)
  w.writeStringField(2, info.email)
  return w.finish()
}

export function decodeSenderInfo(buf: Uint8Array): SenderInfo {
  const r = new ProtoReader(buf)
  const out: SenderInfo = { name: "", email: "" }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 2) out.name = r.readString()
    else if (fieldNumber === 2 && wireType === 2) out.email = r.readString()
    else r.skip(wireType)
  }
  return out
}

export function encodePushMessagePayload(msg: PushMessagePayload): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeInt64Field(1, msg.message_id)
  w.writeInt64Field(2, msg.conversation_id)
  w.writeStringField(3, msg.message_type)
  w.writeStringField(4, msg.content)
  w.writeInt64Field(5, msg.sender_id)
  w.writeInt64Field(6, msg.sent_at)
  w.writeStringField(7, msg.conversation_type)
  w.writeStringField(8, msg.client_msg_id)
  w.writeBoolField(9, msg.is_system)
  if (msg.sender_info && (msg.sender_info.name || msg.sender_info.email)) {
    w.writeBytesField(10, encodeSenderInfo(msg.sender_info))
  }
  if (msg.mentions?.length) {
    w.writeRepeatedStringField(11, msg.mentions)
  }
  return w.finish()
}

export function decodePushMessagePayload(buf: Uint8Array): PushMessagePayload {
  const r = new ProtoReader(buf)
  const out: PushMessagePayload = {
    message_id: 0,
    conversation_id: 0,
    message_type: "",
    content: "",
    sender_id: 0,
    sent_at: 0,
    conversation_type: "",
    client_msg_id: "",
    is_system: false,
    sender_info: { name: "", email: "" },
    mentions: [],
  }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.message_id = r.readInt64()
    else if (fieldNumber === 2 && wireType === 0) out.conversation_id = r.readInt64()
    else if (fieldNumber === 3 && wireType === 2) out.message_type = r.readString()
    else if (fieldNumber === 4 && wireType === 2) out.content = r.readString()
    else if (fieldNumber === 5 && wireType === 0) out.sender_id = r.readInt64()
    else if (fieldNumber === 6 && wireType === 0) out.sent_at = r.readInt64()
    else if (fieldNumber === 7 && wireType === 2) out.conversation_type = r.readString()
    else if (fieldNumber === 8 && wireType === 2) out.client_msg_id = r.readString()
    else if (fieldNumber === 9 && wireType === 0) out.is_system = r.readBool()
    else if (fieldNumber === 10 && wireType === 2) out.sender_info = decodeSenderInfo(r.readBytes())
    else if (fieldNumber === 11 && wireType === 2) out.mentions.push(r.readString())
    else r.skip(wireType)
  }
  return out
}

export function decodePushPresencePayload(buf: Uint8Array): PushPresencePayload {
  const r = new ProtoReader(buf)
  const out: PushPresencePayload = { user_id: 0, status: "", updated_at: 0 }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.user_id = r.readInt64()
    else if (fieldNumber === 2 && wireType === 2) out.status = r.readString()
    else if (fieldNumber === 3 && wireType === 0) out.updated_at = r.readInt64()
    else r.skip(wireType)
  }
  return out
}

export function decodePushNotificationPayload(buf: Uint8Array): PushNotificationPayload {
  const r = new ProtoReader(buf)
  const out: PushNotificationPayload = {
    notification_type: "",
    title: "",
    body: "",
    related_id: 0,
  }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 2) out.notification_type = r.readString()
    else if (fieldNumber === 2 && wireType === 2) out.title = r.readString()
    else if (fieldNumber === 3 && wireType === 2) out.body = r.readString()
    else if (fieldNumber === 4 && wireType === 0) out.related_id = r.readInt64()
    else r.skip(wireType)
  }
  return out
}

export function decodePushFriendApplicationPayload(
  buf: Uint8Array
): PushFriendApplicationPayload {
  const r = new ProtoReader(buf)
  const out: PushFriendApplicationPayload = {
    user_id: 0,
    friend_id: 0,
    status: "",
    created_at: 0,
    updated_at: 0,
  }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.user_id = r.readInt64()
    else if (fieldNumber === 2 && wireType === 0) out.friend_id = r.readInt64()
    else if (fieldNumber === 3 && wireType === 2) out.status = r.readString()
    else if (fieldNumber === 4 && wireType === 0) out.created_at = r.readInt64()
    else if (fieldNumber === 5 && wireType === 0) out.updated_at = r.readInt64()
    else r.skip(wireType)
  }
  return out
}

export function decodePushTypingPayload(buf: Uint8Array): PushTypingPayload {
  const r = new ProtoReader(buf)
  const out: PushTypingPayload = { user_id: 0, conversation_id: 0 }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.user_id = r.readInt64()
    else if (fieldNumber === 2 && wireType === 0) out.conversation_id = r.readInt64()
    else r.skip(wireType)
  }
  return out
}

export function decodePushReadReceiptPayload(buf: Uint8Array): PushReadReceiptPayload {
  const r = new ProtoReader(buf)
  const out: PushReadReceiptPayload = {
    conversation_id: 0,
    user_id: 0,
    last_read_message_id: 0,
    updated_at: 0,
  }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.conversation_id = r.readInt64()
    else if (fieldNumber === 2 && wireType === 0) out.user_id = r.readInt64()
    else if (fieldNumber === 3 && wireType === 0) out.last_read_message_id = r.readInt64()
    else if (fieldNumber === 4 && wireType === 0) out.updated_at = r.readInt64()
    else r.skip(wireType)
  }
  return out
}

export function decodeReconnectPayload(buf: Uint8Array): ReconnectPayload {
  const r = new ProtoReader(buf)
  const out: ReconnectPayload = { reconnect_delay_ms: 0, gateway_node_id: "" }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.reconnect_delay_ms = r.readInt64()
    else if (fieldNumber === 2 && wireType === 2) out.gateway_node_id = r.readString()
    else r.skip(wireType)
  }
  return out
}

export function decodeTokenExpiredPayload(buf: Uint8Array): TokenExpiredPayload {
  const r = new ProtoReader(buf)
  const out: TokenExpiredPayload = { expired_at: 0, reason: "" }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.expired_at = r.readInt64()
    else if (fieldNumber === 2 && wireType === 2) out.reason = r.readString()
    else r.skip(wireType)
  }
  return out
}

export function encodeServerAckPayload(msg: ServerAckPayload): Uint8Array<ArrayBuffer> {
  const w = new ProtoWriter()
  w.writeInt64Field(1, msg.ack_seq)
  w.writeStringField(2, msg.client_msg_id)
  w.writeInt32Field(3, msg.code)
  w.writeStringField(4, msg.msg)
  w.writeInt32Field(5, msg.status)
  w.writeInt64Field(6, msg.message_id)
  return w.finish()
}

export function decodeServerAckPayload(buf: Uint8Array): ServerAckPayload {
  const r = new ProtoReader(buf)
  const out: ServerAckPayload = {
    ack_seq: 0,
    client_msg_id: "",
    code: 0,
    msg: "",
    status: AckStatus.UNSPECIFIED,
    message_id: 0,
  }
  while (r.hasMore()) {
    const { fieldNumber, wireType } = r.readTag()
    if (fieldNumber === 1 && wireType === 0) out.ack_seq = r.readInt64()
    else if (fieldNumber === 2 && wireType === 2) out.client_msg_id = r.readString()
    else if (fieldNumber === 3 && wireType === 0) out.code = r.readInt32()
    else if (fieldNumber === 4 && wireType === 2) out.msg = r.readString()
    else if (fieldNumber === 5 && wireType === 0) out.status = r.readInt32() as AckStatusValue
    else if (fieldNumber === 6 && wireType === 0) out.message_id = r.readInt64()
    else r.skip(wireType)
  }
  return out
}

export function isReplayableServerFrame(type: FrameTypeValue): boolean {
  return (
    type === FrameType.PUSH_MESSAGE ||
    type === FrameType.PUSH_NOTIFICATION ||
    type === FrameType.PUSH_FRIEND_APPLICATION ||
    type === FrameType.PUSH_READ_RECEIPT
  )
}
