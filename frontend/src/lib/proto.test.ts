import { describe, expect, it } from "vitest"
import {
  AckStatus,
  FrameType,
  decodePushMessagePayload,
  decodeSendMessagePayload,
  decodeServerAckPayload,
  decodeWsFrame,
  encodePushMessagePayload,
  encodeSendMessagePayload,
  encodeServerAckPayload,
  encodeWsFrame,
  isReplayableServerFrame,
} from "@/lib/proto"
import { tokenizeMarkedSnippet } from "@/lib/utils"

describe("ws protobuf wire codec", () => {
  it("round-trips WsFrame with SendMessagePayload and UTF-8 + mentions", () => {
    const payloadBytes = encodeSendMessagePayload({
      conversation_id: 9007199254740991,
      message_type: "text",
      content: "你好，AIM Web 端！",
      client_msg_id: "d1-8899-aabb",
      mentions: ["1001", "1002"],
    })

    const frameBytes = encodeWsFrame({
      type: FrameType.SEND_MESSAGE,
      seq: 42,
      payload: payloadBytes,
      timestamp: 1717056000123,
    })

    const decodedFrame = decodeWsFrame(frameBytes)
    expect(decodedFrame.type).toBe(FrameType.SEND_MESSAGE)
    expect(decodedFrame.seq).toBe(42)
    expect(decodedFrame.timestamp).toBe(1717056000123)

    const decodedPayload = decodeSendMessagePayload(decodedFrame.payload)
    expect(decodedPayload).toEqual({
      conversation_id: 9007199254740991,
      message_type: "text",
      content: "你好，AIM Web 端！",
      client_msg_id: "d1-8899-aabb",
      mentions: ["1001", "1002"],
    })
  })

  it("round-trips PushMessagePayload and ServerAckPayload", () => {
    const pushBytes = encodePushMessagePayload({
      message_id: 50001,
      conversation_id: 88,
      message_type: "image",
      content: '{"schema":"aim.attachment.v1","file_id":"att_1"}',
      sender_id: 7,
      sent_at: 1717056099000,
      conversation_type: "group",
      client_msg_id: "d1-client-7",
      is_system: false,
      sender_info: {
        name: "Alice",
        email: "alice@example.com",
      },
      mentions: ["9"],
    })

    const decodedPush = decodePushMessagePayload(pushBytes)
    expect(decodedPush).toEqual({
      message_id: 50001,
      conversation_id: 88,
      message_type: "image",
      content: '{"schema":"aim.attachment.v1","file_id":"att_1"}',
      sender_id: 7,
      sent_at: 1717056099000,
      conversation_type: "group",
      client_msg_id: "d1-client-7",
      is_system: false,
      sender_info: {
        name: "Alice",
        email: "alice@example.com",
      },
      mentions: ["9"],
    })

    const ackBytes = encodeServerAckPayload({
      ack_seq: 12,
      client_msg_id: "d1-client-7",
      code: 0,
      msg: "ok",
      status: AckStatus.ACCEPTED,
      message_id: 50001,
    })
    expect(decodeServerAckPayload(ackBytes)).toEqual({
      ack_seq: 12,
      client_msg_id: "d1-client-7",
      code: 0,
      msg: "ok",
      status: AckStatus.ACCEPTED,
      message_id: 50001,
    })
  })

  it("classifies replayable pending server frames per docs/ws.md", () => {
    expect(isReplayableServerFrame(FrameType.PUSH_MESSAGE)).toBe(true)
    expect(isReplayableServerFrame(FrameType.PUSH_NOTIFICATION)).toBe(true)
    expect(isReplayableServerFrame(FrameType.PUSH_FRIEND_APPLICATION)).toBe(true)
    expect(isReplayableServerFrame(FrameType.PUSH_READ_RECEIPT)).toBe(true)
    expect(isReplayableServerFrame(FrameType.PUSH_TYPING)).toBe(false)
    expect(isReplayableServerFrame(FrameType.PUSH_PRESENCE)).toBe(false)
    expect(isReplayableServerFrame(FrameType.SERVER_ACK)).toBe(false)
  })

  it("tokenizes <mark> search snippets without HTML injection", () => {
    const tokens = tokenizeMarkedSnippet("前缀<mark>高亮词</mark>后缀")
    expect(tokens).toEqual([
      { text: "前缀", highlighted: false },
      { text: "高亮词", highlighted: true },
      { text: "后缀", highlighted: false },
    ])
  })
})
