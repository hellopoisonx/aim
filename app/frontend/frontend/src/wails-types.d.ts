/**
 * Type declarations for AIM Wails v2 Go bindings.
 * These mirror the Go exported methods on the App struct.
 * When Wails regenerates bindings, it will produce App.d.ts — these local
 * declarations fill the gap for TypeScript until then.
 */

export interface GatewayConfig {
  httpUrl: string
  wsUrl: string
}

export interface DeviceIDResult {
  deviceId: string
}

export interface RegisterRequest {
  username: string
  password: string
}

export interface LoginRequest {
  username: string
  password
}

export interface SessionStateResult {
  userId: string
  username: string
  token: string
  expiresAt: string
  isGuest: boolean
}

export interface ProtocolCatalogResult {
  restEndpoints: RestEndpoint[]
  wsFrames: WsFrame[]
}

export interface RestEndpoint {
  method: string
  path: string
  description: string
}

export interface WsFrame {
  frameType: string
  fields: string
  description: string
}

export interface SendMessageRequest {
  conversationId: string
  content: string
  clientMsgId: string
}

export interface SendHeartbeatRequest {
  conversationId: string
}

export interface SendTypingRequest {
  conversationId: string
}

export interface SendReadReceiptRequest {
  conversationId: string
  messageId: string
}

export interface SendAckRequest {
  conversationId: string
  messageId: string
}

export interface ConnectionState {
  connected: boolean
  nodeId: string
  sessionId: string
  serverTime: string
}

export interface FrameReceived {
  frameType: string
  payload: string
  timestamp: string
}

export interface AIMError {
  code: string
  message: string
}

// Wails v2 runtime events
export type WailsEventCallback<T = unknown> = (data: T) => void