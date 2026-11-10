import { MessageSquareIcon, ShieldCheckIcon } from "lucide-react"
import { type SyntheticEvent, useState } from "react"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { aimApi, getOrCreateDeviceId } from "@/lib/api"
import type { AuthSession } from "@/lib/types"

interface AuthViewProps {
  onAuthed: (session: AuthSession) => void
}

export function AuthView({ onAuthed }: AuthViewProps) {
  const [tab, setTab] = useState<"login" | "register">("login")
  const [loginEmail, setLoginEmail] = useState("")
  const [loginPassword, setLoginPassword] = useState("")
  const [regEmail, setRegEmail] = useState("")
  const [regUsername, setRegUsername] = useState("")
  const [regPassword, setRegPassword] = useState("")
  const [regAvatar, setRegAvatar] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const [infoMsg, setInfoMsg] = useState<string | null>(null)

  const deviceId = getOrCreateDeviceId()

  async function handleLogin(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault()
    setErrorMsg(null)
    setInfoMsg(null)
    if (!loginEmail.trim() || loginPassword.length < 8) {
      setErrorMsg("请输入有效邮箱且密码不少于 8 位")
      return
    }
    setSubmitting(true)
    try {
      const session = await aimApi.login({
        email: loginEmail.trim(),
        password: loginPassword,
      })
      onAuthed(session)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "登录失败")
    } finally {
      setSubmitting(false)
    }
  }

  async function handleRegister(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault()
    setErrorMsg(null)
    setInfoMsg(null)
    if (!regEmail.trim() || !regUsername.trim() || regPassword.length < 8) {
      setErrorMsg("请填写邮箱、昵称，且密码不少于 8 位")
      return
    }
    setSubmitting(true)
    try {
      await aimApi.register({
        email: regEmail.trim(),
        username: regUsername.trim(),
        password: regPassword,
        avatar: regAvatar.trim() || undefined,
      })
      const session = await aimApi.login({
        email: regEmail.trim(),
        password: regPassword,
      })
      onAuthed(session)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : "注册失败")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-muted/30 p-4">
      <div className="w-full max-w-md space-y-6">
        <div className="flex flex-col items-center text-center space-y-2">
          <div className="size-11 rounded-xl bg-primary text-primary-foreground flex items-center justify-center shadow-sm">
            <MessageSquareIcon className="size-5" />
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">AIM 网页端</h1>
          <p className="text-sm text-muted-foreground">
            基于二进制 Protobuf WebSocket 与云端消息漫游的即时通讯客户端
          </p>
        </div>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">账号认证</CardTitle>
            <CardDescription className="flex items-center gap-1.5 text-xs">
              <ShieldCheckIcon className="size-3.5 text-muted-foreground" />
              <span className="truncate" title={deviceId}>
                设备标识: {deviceId}
              </span>
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Tabs
              value={tab}
              onValueChange={(v) => {
                setTab(v as "login" | "register")
                setErrorMsg(null)
                setInfoMsg(null)
              }}
            >
              <TabsList className="grid w-full grid-cols-2 mb-4">
                <TabsTrigger value="login">登录</TabsTrigger>
                <TabsTrigger value="register">注册账号</TabsTrigger>
              </TabsList>

              {errorMsg ? (
                <div className="mb-4 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                  {errorMsg}
                </div>
              ) : null}

              {infoMsg ? (
                <div className="mb-4 rounded-md border bg-muted px-3 py-2 text-xs text-foreground">
                  {infoMsg}
                </div>
              ) : null}

              <TabsContent value="login">
                <form onSubmit={handleLogin} className="space-y-4">
                  <div className="space-y-1.5">
                    <Label htmlFor="login-email">邮箱</Label>
                    <Input
                      id="login-email"
                      type="email"
                      placeholder="user@example.com"
                      autoComplete="email"
                      value={loginEmail}
                      onChange={(e) => setLoginEmail(e.target.value)}
                      required
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="login-password">密码</Label>
                    <Input
                      id="login-password"
                      type="password"
                      placeholder="不少于 8 位字符"
                      autoComplete="current-password"
                      value={loginPassword}
                      onChange={(e) => setLoginPassword(e.target.value)}
                      minLength={8}
                      required
                    />
                  </div>
                  <Button type="submit" className="w-full" disabled={submitting}>
                    {submitting ? "正在登录..." : "登录"}
                  </Button>
                </form>
              </TabsContent>

              <TabsContent value="register">
                <form onSubmit={handleRegister} className="space-y-4">
                  <div className="space-y-1.5">
                    <Label htmlFor="reg-email">邮箱</Label>
                    <Input
                      id="reg-email"
                      type="email"
                      placeholder="alice@example.com"
                      autoComplete="email"
                      value={regEmail}
                      onChange={(e) => setRegEmail(e.target.value)}
                      required
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="reg-username">昵称</Label>
                    <Input
                      id="reg-username"
                      type="text"
                      placeholder="Alice"
                      value={regUsername}
                      onChange={(e) => setRegUsername(e.target.value)}
                      required
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="reg-password">密码</Label>
                    <Input
                      id="reg-password"
                      type="password"
                      placeholder="不少于 8 位字符"
                      autoComplete="new-password"
                      value={regPassword}
                      onChange={(e) => setRegPassword(e.target.value)}
                      minLength={8}
                      required
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="reg-avatar">头像 URL（可选）</Label>
                    <Input
                      id="reg-avatar"
                      type="url"
                      placeholder="https://example.com/avatar.png"
                      value={regAvatar}
                      onChange={(e) => setRegAvatar(e.target.value)}
                    />
                  </div>
                  <Button type="submit" className="w-full" disabled={submitting}>
                    {submitting ? "正在注册..." : "注册并登录"}
                  </Button>
                </form>
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
