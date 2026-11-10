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
import { parseApiJson, stringifyApiBody } from "@/lib/api"
import { tokenizeMarkedSnippet } from "@/lib/utils"

describe("ws protobuf wire codec", () => {
  it("round-trips WsFrame with SendMessagePayload and 64-bit Snowflake IDs", () => {
    const payloadBytes = encodeSendMessagePayload({
      conversation_id: "245762623319052288",
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
      conversation_id: "245762623319052288",
      message_type: "text",
      content: "你好，AIM Web 端！",
      client_msg_id: "d1-8899-aabb",
      mentions: ["1001", "1002"],
    })
  })

  it("round-trips PushMessagePayload and ServerAckPayload", () => {
    const pushBytes = encodePushMessagePayload({
      message_id: "245763041197559808",
      conversation_id: "245762623319052288",
      message_type: "image",
      content: '{"schema":"aim.attachment.v1","file_id":"att_1"}',
      sender_id: "245762623319052289",
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
      message_id: "245763041197559808",
      conversation_id: "245762623319052288",
      message_type: "image",
      content: '{"schema":"aim.attachment.v1","file_id":"att_1"}',
      sender_id: "245762623319052289",
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
      message_id: "245763041197559808",
    })
    expect(decodeServerAckPayload(ackBytes)).toEqual({
      ack_seq: 12,
      client_msg_id: "d1-client-7",
      code: 0,
      msg: "ok",
      status: AckStatus.ACCEPTED,
      message_id: "245763041197559808",
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

  it("preserves 64-bit Snowflake IDs across JSON parse and stringify", () => {
    const raw =
      '{"code":0,"msg":"ok","body":{"users":[{"id":245762623319052288,"nickname":"test-user1","email":"test-user@email.com","avatar":""}]}}'
    const parsed = parseApiJson<{
      code: number
      body: { users: Array<{ id: string; nickname: string }> }
    }>(raw)
    expect(parsed.body.users[0].id).toBe("245762623319052288")

    const bodyJson = stringifyApiBody({
      conversation_type: "direct",
      member_ids: ["245762623319052288", "245763041197559808"],
      user_id: "245762623319052288",
    })
    expect(bodyJson).toBe(
      '{"conversation_type":"direct","member_ids":[245762623319052288,245763041197559808],"user_id":245762623319052288}'
    )
  })
})
