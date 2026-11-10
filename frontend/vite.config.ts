import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import type { IncomingMessage } from "node:http"
import { defineConfig } from "vite"

function extractAccessTokenFromCookie(req: IncomingMessage): string | null {
  const raw = req.headers.cookie
  if (!raw) return null
  for (const part of raw.split(";")) {
    const [k, ...rest] = part.trim().split("=")
    if (k === "aim_access_token" && rest.length > 0) {
      return decodeURIComponent(rest.join("="))
    }
  }
  return null
}

const gatewayHttpTarget = process.env.AIM_GATEWAY_URL ?? "http://127.0.0.1:8888"
const gatewayWsTarget = process.env.AIM_GATEWAY_WS_URL ?? "ws://127.0.0.1:8888"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: gatewayHttpTarget,
        changeOrigin: true,
      },
      "/ws": {
        target: gatewayWsTarget,
        ws: true,
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on("proxyReq", (proxyReq, req) => {
            if (!req.headers.authorization) {
              const token = extractAccessTokenFromCookie(req)
              if (token) {
                proxyReq.setHeader("Authorization", `Bearer ${token}`)
              }
            }
          })
          proxy.on("proxyReqWs", (proxyReq, req) => {
            if (!req.headers.authorization) {
              const token = extractAccessTokenFromCookie(req)
              if (token) {
                proxyReq.setHeader("Authorization", `Bearer ${token}`)
              }
            }
          })
        },
      },
    },
  },
})
