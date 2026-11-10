import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatTimestamp(ms: number): string {
  if (!ms) return ""
  const date = new Date(ms)
  const now = new Date()
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()
  if (sameDay) {
    return date.toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    })
  }
  return date.toLocaleDateString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB"]
  let value = bytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`
}

export function initialsFromName(name?: string, fallback = "U"): string {
  const trimmed = (name ?? "").trim()
  if (!trimmed) return fallback
  const parts = trimmed.split(/\s+/)
  if (parts.length >= 2) {
    return `${parts[0][0] ?? ""}${parts[1][0] ?? ""}`.toUpperCase()
  }
  return trimmed.slice(0, 2).toUpperCase()
}

export async function sha256Hex(buffer: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", buffer)
  const bytes = new Uint8Array(digest)
  let out = ""
  for (let i = 0; i < bytes.length; i += 1) {
    out += bytes[i].toString(16).padStart(2, "0")
  }
  return out
}

export interface SnippetToken {
  text: string
  highlighted: boolean
}

export function tokenizeMarkedSnippet(snippet: string): SnippetToken[] {
  if (!snippet) return []
  const tokens: SnippetToken[] = []
  const regex = /<mark>(.*?)<\/mark>/gi
  let lastIndex = 0
  let match = regex.exec(snippet)
  while (match !== null) {
    if (match.index > lastIndex) {
      tokens.push({
        text: snippet.slice(lastIndex, match.index),
        highlighted: false,
      })
    }
    if (match[1]) {
      tokens.push({
        text: match[1],
        highlighted: true,
      })
    }
    lastIndex = regex.lastIndex
    match = regex.exec(snippet)
  }
  if (lastIndex < snippet.length) {
    tokens.push({
      text: snippet.slice(lastIndex),
      highlighted: false,
    })
  }
  return tokens
}
