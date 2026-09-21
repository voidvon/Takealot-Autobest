import { useState, useEffect } from 'react'
import { api } from '../../api/client'
import type { AccountStatus, UpdateInfo } from '../../types'
import { Dialog } from '../ui/dialog'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { cn, openExternalURL } from '../../lib/utils'
import {
  User,
  Lock,
  Mail,
  Crown,
  Eye,
  EyeOff,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  RefreshCw,
  LogOut,
  Sparkles,
  Info,
  Globe,
  ExternalLink,
  Copy,
  Check,
  Rocket,
} from 'lucide-react'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  status: AccountStatus | null
  onAuthenticated: (status: AccountStatus) => void
  version?: string
  updateInfo?: UpdateInfo | null
  onOpenUpdate?: () => void
  onCheckUpdate?: () => Promise<void>
  initialTab?: 'account' | 'about'
}

export function AccountModal({
  open,
  onOpenChange,
  status,
  onAuthenticated,
  version = '0.2.0',
  updateInfo,
  onOpenUpdate,
  onCheckUpdate,
  initialTab = 'account',
}: Props) {
  const [activeTab, setActiveTab] = useState<'account' | 'about'>(initialTab)
  const [register, setRegister] = useState(false)
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmation, setShowConfirmation] = useState(false)
  const [busy, setBusy] = useState(false)
  const [checkingUpdate, setCheckingUpdate] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [copied, setCopied] = useState(false)

  // Reset or initialize tab when dialog opens
  useEffect(() => {
    if (open) {
      setActiveTab(initialTab)
      setError('')
      setNotice('')
    }
  }, [open, initialTab])

  async function run(action: () => Promise<void>) {
    setBusy(true)
    setError('')
    setNotice('')
    try {
      await action()
    } catch (e) {
      setError(e instanceof Error ? e.message : '操作失败，请重试')
    } finally {
      setBusy(false)
      setPassword('')
      setConfirmation('')
    }
  }

  const handleToggleMode = (isRegister: boolean) => {
    setRegister(isRegister)
    setError('')
    setNotice('')
  }

  const handleCheckUpdate = async () => {
    if (!onCheckUpdate) return
    setCheckingUpdate(true)
    setError('')
    try {
      await onCheckUpdate()
    } catch (e: any) {
      setError(e?.message || '检查更新失败')
    } finally {
      setCheckingUpdate(false)
    }
  }

  const handleCopyWebsite = () => {
    navigator.clipboard.writeText('https://takealot.0122.vip')
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleOpenWebsite = () => {
    openExternalURL('https://takealot.0122.vip')
  }

  const dialogTitle =
    activeTab === 'account'
      ? status?.authenticated
        ? '账号信息'
        : register
          ? '注册新账号'
          : '登录 Takealot 掌柜'
      : '关于 Takealot 掌柜'

  const dialogDescription =
    activeTab === 'account'
      ? status?.authenticated
        ? '查看当前登录账号与 VIP 会员授权状态'
        : register
          ? '创建账号以开启 Takealot 智能自动化运营'
          : '登录账号以在线验证 VIP 会员并解锁自动化功能'
      : '客户端版本信息、在线自动更新与官方服务支持'

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={dialogTitle}
      description={dialogDescription}
      maxWidth="xl"
    >
      <div className="flex flex-col sm:flex-row gap-4 pt-1 min-h-[360px]">
        {/* Left Navigation Menu */}
        <div className="w-full sm:w-36 shrink-0 flex sm:flex-col gap-1 border-b sm:border-b-0 sm:border-r border-border/70 pb-2 sm:pb-0 sm:pr-3">
          <button
            type="button"
            onClick={() => setActiveTab('account')}
            className={cn(
              "flex items-center gap-2 px-3 py-2 text-xs font-medium rounded-lg transition-colors text-left cursor-pointer",
              activeTab === 'account'
                ? "bg-primary/10 text-primary font-semibold"
                : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
            )}
          >
            <User className="h-4 w-4 shrink-0" />
            <span>账号信息</span>
          </button>

          <button
            type="button"
            onClick={() => setActiveTab('about')}
            className={cn(
              "flex items-center gap-2 px-3 py-2 text-xs font-medium rounded-lg transition-colors text-left cursor-pointer relative",
              activeTab === 'about'
                ? "bg-primary/10 text-primary font-semibold"
                : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
            )}
          >
            <Info className="h-4 w-4 shrink-0" />
            <span className="flex-1">关于</span>
            {updateInfo?.has_update && (
              <span className="flex h-2 w-2 relative">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
            )}
          </button>
        </div>

        {/* Right Content Area */}
        <div className="flex-1 min-w-0 space-y-4">
          {/* Error Notification */}
          {error && (
            <div role="alert" className="flex items-start gap-2.5 p-3 text-xs rounded-lg border border-destructive/30 bg-destructive/10 text-destructive animate-in fade-in-50">
              <AlertCircle className="h-4 w-4 shrink-0 mt-0.5" />
              <span className="leading-snug">{error}</span>
            </div>
          )}

          {/* Success Notice */}
          {notice && (
            <div role="status" className="flex items-start gap-2.5 p-3 text-xs rounded-lg border border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 animate-in fade-in-50">
              <CheckCircle2 className="h-4 w-4 shrink-0 mt-0.5" />
              <span className="leading-snug">{notice}</span>
            </div>
          )}

          {/* TAB 1: 账号信息 (Account Info) */}
          {activeTab === 'account' && (
            <>
              {/* Status Message from server */}
              {status?.message && !status.authenticated && (
                <p className="text-xs text-muted-foreground bg-muted/40 p-2.5 rounded-lg border border-border/50">
                  {status.message}
                </p>
              )}

              {/* Logged-In View: User Profile & VIP Status */}
              {status?.authenticated ? (
                <div className="rounded-xl border border-border/80 bg-muted/20 p-4 space-y-4">
                  {/* User Info Header */}
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="h-10 w-10 rounded-full bg-primary/15 text-primary border border-primary/25 flex items-center justify-center font-bold text-sm shrink-0 shadow-xs">
                        {(status.user?.display_name || status.user?.username || 'U').slice(0, 1).toUpperCase()}
                      </div>
                      <div className="min-w-0">
                        <h4 className="font-semibold text-sm text-foreground truncate">
                          {status.user?.display_name || status.user?.username}
                        </h4>
                        <p className="text-xs text-muted-foreground truncate">
                          账号 ID: #{status.user?.id ?? '--'}
                        </p>
                      </div>
                    </div>

                    {status.eligible ? (
                      <Badge variant="success" className="px-2 py-0.5 text-xs gap-1 font-medium shrink-0">
                        <Crown className="h-3 w-3" />
                        <span>VIP 有效</span>
                      </Badge>
                    ) : (
                      <Badge variant="destructive" className="px-2 py-0.5 text-xs gap-1 font-medium shrink-0 animate-pulse">
                        <Crown className="h-3 w-3" />
                        <span>未激活 VIP</span>
                      </Badge>
                    )}
                  </div>

                  {/* Membership Details */}
                  <div className="rounded-lg border border-border/60 bg-card p-3 space-y-2 text-xs">
                    <div className="flex items-center justify-between text-muted-foreground">
                      <span>会员有效期</span>
                      <span className="font-medium text-foreground">
                        {status.eligible
                          ? (status.expires_at ? new Date(status.expires_at).toLocaleString() : '长期有效')
                          : '暂无有效 VIP 或等待开通'}
                      </span>
                    </div>

                    {status.message && (
                      <div className="flex items-center justify-between text-muted-foreground border-t border-border/40 pt-2">
                        <span>授权详情</span>
                        <span className="text-foreground">{status.message}</span>
                      </div>
                    )}

                    {!status.persistent && (
                      <p className="text-[11px] text-amber-600 dark:text-amber-400 leading-snug border-t border-border/40 pt-2">
                        提示：系统安全存储不可用，本次登录仅在当前程序运行期间保留。
                      </p>
                    )}
                  </div>

                  {/* Action Buttons */}
                  <div className="flex items-center gap-2 pt-1">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      loading={busy}
                      onClick={() => void run(async () => { onAuthenticated(await api.refreshAccount()) })}
                      className="flex-1 gap-1.5 text-xs h-9"
                    >
                      <RefreshCw className="h-3.5 w-3.5" />
                      <span>刷新会员状态</span>
                    </Button>

                    <Button
                      variant="destructive"
                      size="sm"
                      disabled={busy}
                      onClick={() => void run(async () => { onAuthenticated(await api.logoutAccount()) })}
                      className="gap-1.5 text-xs h-9 px-4"
                    >
                      <LogOut className="h-3.5 w-3.5" />
                      <span>退出登录</span>
                    </Button>
                  </div>
                </div>
              ) : (
                /* Logged-Out View: Login & Register Form */
                <div className="space-y-4">
                  {/* Modern Segmented Pill Control */}
                  <div className="flex p-1 bg-muted/70 border border-border/60 rounded-lg">
                    <button
                      type="button"
                      onClick={() => handleToggleMode(false)}
                      className={cn(
                        "flex-1 py-1.5 text-xs font-semibold rounded-md transition-all text-center flex items-center justify-center gap-1.5 cursor-pointer",
                        !register
                          ? "bg-background text-foreground shadow-xs"
                          : "text-muted-foreground hover:text-foreground"
                      )}
                    >
                      <User className="h-3.5 w-3.5" />
                      <span>账号登录</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => handleToggleMode(true)}
                      className={cn(
                        "flex-1 py-1.5 text-xs font-semibold rounded-md transition-all text-center flex items-center justify-center gap-1.5 cursor-pointer",
                        register
                          ? "bg-background text-foreground shadow-xs"
                          : "text-muted-foreground hover:text-foreground"
                      )}
                    >
                      <Sparkles className="h-3.5 w-3.5" />
                      <span>新用户注册</span>
                    </button>
                  </div>

                  <form
                    className="space-y-3.5"
                    onSubmit={(e) => {
                      e.preventDefault()
                      void run(async () => {
                        if (register) {
                          if (password !== confirmation) throw new Error('两次输入的密码不一致')
                          await api.registerAccount(username.trim(), email.trim(), password)
                          setRegister(false)
                          setNotice('注册成功，请登录。VIP 需由管理员开通。')
                        } else {
                          const state = await api.loginAccount(username.trim(), password)
                          onAuthenticated(state)
                          if (state.eligible) onOpenChange(false)
                        }
                      })
                    }}
                  >
                    {/* Username Field */}
                    <div className="space-y-1.5">
                      <label className="text-xs font-medium text-foreground flex items-center gap-1.5">
                        <User className="h-3.5 w-3.5 text-muted-foreground" />
                        <span>{register ? '用户名' : '用户名或邮箱'}</span>
                        {register && <span className="text-[10px] text-muted-foreground">(3–64 字符，不含空格或 @)</span>}
                      </label>
                      <Input
                        autoComplete="username"
                        required
                        maxLength={register ? 64 : 254}
                        value={username}
                        onChange={(e) => setUsername(e.target.value)}
                        placeholder={register ? '设置登录用户名' : '输入用户名或邮箱'}
                        disabled={busy}
                        className="h-9 text-xs"
                      />
                    </div>

                    {/* Email Field (Registration Only) */}
                    {register && (
                      <div className="space-y-1.5 animate-in fade-in-50">
                        <label className="text-xs font-medium text-foreground flex items-center gap-1.5">
                          <Mail className="h-3.5 w-3.5 text-muted-foreground" />
                          <span>电子邮箱</span>
                          <span className="text-[10px] text-muted-foreground">(选填，用于账号通知与找回)</span>
                        </label>
                        <Input
                          type="email"
                          autoComplete="email"
                          maxLength={254}
                          value={email}
                          onChange={(e) => setEmail(e.target.value)}
                          placeholder="name@example.com"
                          disabled={busy}
                          className="h-9 text-xs"
                        />
                      </div>
                    )}

                    {/* Password Field */}
                    <div className="space-y-1.5">
                      <label className="text-xs font-medium text-foreground flex items-center gap-1.5">
                        <Lock className="h-3.5 w-3.5 text-muted-foreground" />
                        <span>密码</span>
                        {register && <span className="text-[10px] text-muted-foreground">(至少 8 字符)</span>}
                      </label>
                      <div className="relative">
                        <Input
                          type={showPassword ? 'text' : 'password'}
                          required
                          autoComplete={register ? 'new-password' : 'current-password'}
                          value={password}
                          onChange={(e) => setPassword(e.target.value)}
                          placeholder={register ? '设置不少于 8 位的安全密码' : '输入登录密码'}
                          disabled={busy}
                          className="h-9 text-xs pr-9"
                        />
                        <button
                          type="button"
                          onClick={() => setShowPassword(!showPassword)}
                          className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                          tabIndex={-1}
                        >
                          {showPassword ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                        </button>
                      </div>
                    </div>

                    {/* Confirm Password Field (Registration Only) */}
                    {register && (
                      <div className="space-y-1.5 animate-in fade-in-50">
                        <label className="text-xs font-medium text-foreground flex items-center gap-1.5">
                          <Lock className="h-3.5 w-3.5 text-muted-foreground" />
                          <span>确认密码</span>
                        </label>
                        <div className="relative">
                          <Input
                            type={showConfirmation ? 'text' : 'password'}
                            required
                            autoComplete="new-password"
                            value={confirmation}
                            onChange={(e) => setConfirmation(e.target.value)}
                            placeholder="再次输入密码"
                            disabled={busy}
                            className="h-9 text-xs pr-9"
                          />
                          <button
                            type="button"
                            onClick={() => setShowConfirmation(!showConfirmation)}
                            className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                            tabIndex={-1}
                          >
                            {showConfirmation ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                          </button>
                        </div>
                      </div>
                    )}

                    {/* Submit Action Button */}
                    <Button
                      type="submit"
                      disabled={busy || status?.state === 'not_configured'}
                      loading={busy}
                      className="w-full h-9 text-xs font-semibold gap-1.5 mt-2"
                    >
                      {!busy && <ArrowRight className="h-3.5 w-3.5" />}
                      <span>{register ? '立即注册新账号' : '登录 Takealot 掌柜'}</span>
                    </Button>

                    {/* Bottom Quick Switch Link */}
                    <div className="pt-1 text-center text-xs text-muted-foreground">
                      {register ? (
                        <span>
                          已有账号？{' '}
                          <button
                            type="button"
                            onClick={() => handleToggleMode(false)}
                            className="text-primary hover:underline font-semibold cursor-pointer"
                          >
                            直接登录
                          </button>
                        </span>
                      ) : (
                        <span>
                          还没有账号？{' '}
                          <button
                            type="button"
                            onClick={() => handleToggleMode(true)}
                            className="text-primary hover:underline font-semibold cursor-pointer"
                          >
                            免费注册
                          </button>
                        </span>
                      )}
                    </div>
                  </form>
                </div>
              )}
            </>
          )}

          {/* TAB 2: 关于 (About) */}
          {activeTab === 'about' && (
            <div className="space-y-3.5 animate-in fade-in-50">
              {/* Brand Banner */}
              <div className="flex items-center gap-3.5 p-3.5 rounded-xl border border-border/80 bg-muted/30">
                <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-primary text-primary-foreground font-black text-lg shadow-xs shrink-0">
                  T
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <h3 className="font-bold text-sm text-foreground">Takealot 掌柜</h3>
                    <Badge variant="secondary" className="font-mono text-[10px] px-1.5 h-4.5">
                      v{(version || '0.2.0').replace(/^v/, '')}
                    </Badge>
                  </div>
                  <p className="text-[11px] text-muted-foreground mt-0.5">
                    现代化跨平台 Takealot 自动化跟价与多店铺运营系统
                  </p>
                </div>
              </div>

              {/* Version & Update Card */}
              <div className="rounded-xl border border-border/80 bg-card p-3.5 space-y-3 text-xs">
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-foreground text-sm">版本更新</span>
                      {updateInfo?.has_update ? (
                        <Badge variant="success" className="text-[10px] px-1.5 h-4.5 gap-1 font-medium">
                          <Sparkles className="h-2.5 w-2.5" />
                          <span>可更新 v{updateInfo.version.replace(/^v/, '')}</span>
                        </Badge>
                      ) : (
                        <Badge variant="outline" className="text-[10px] px-1.5 h-4.5 text-muted-foreground">
                          已是最新版本
                        </Badge>
                      )}
                    </div>

                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <span>当前运行版本: <strong className="font-mono text-foreground font-semibold">v{(version || '0.2.0').replace(/^v/, '')}</strong></span>
                      {updateInfo?.has_update && (
                        <span>• 最新: <strong className="font-mono text-emerald-600 dark:text-emerald-400 font-semibold">v{updateInfo.version.replace(/^v/, '')}</strong></span>
                      )}
                    </div>
                  </div>

                  <div className="shrink-0">
                    {updateInfo?.has_update ? (
                      <Button
                        size="default"
                        variant="default"
                        onClick={() => {
                          if (onOpenUpdate) {
                            onOpenUpdate()
                          }
                        }}
                        className="gap-2 text-xs h-9 px-4 shadow-xs font-semibold"
                      >
                        <Rocket className="h-3.5 w-3.5" />
                        <span>立即更新并重启</span>
                      </Button>
                    ) : (
                      <Button
                        size="default"
                        variant="outline"
                        onClick={handleCheckUpdate}
                        loading={checkingUpdate}
                        disabled={checkingUpdate}
                        className="gap-2 text-xs h-9 px-4 font-medium shadow-2xs"
                      >
                        <RefreshCw className="h-3.5 w-3.5" />
                        <span>检查更新</span>
                      </Button>
                    )}
                  </div>
                </div>
              </div>

              {/* Official Website & Support */}
              <div className="rounded-xl border border-border/80 bg-card p-3.5 space-y-2.5 text-xs">
                <span className="font-semibold text-foreground text-sm">官方网址与服务</span>
                <div className="flex items-center justify-between p-2.5 rounded-lg bg-muted/40 border border-border/50 text-xs">
                  <div className="flex items-center gap-2 min-w-0">
                    <Globe className="h-4 w-4 text-primary shrink-0" />
                    <span className="font-mono text-foreground select-all truncate font-medium text-xs">
                      takealot.0122.vip
                    </span>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={handleCopyWebsite}
                      className="h-8 px-2.5 text-xs gap-1.5 text-muted-foreground hover:text-foreground"
                      title="复制官网地址"
                    >
                      {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
                      <span>{copied ? '已复制' : '复制'}</span>
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={handleOpenWebsite}
                      className="h-8 px-3 text-xs gap-1.5 text-primary hover:text-primary font-medium"
                    >
                      <ExternalLink className="h-3.5 w-3.5" />
                      <span>访问官网</span>
                    </Button>
                  </div>
                </div>
              </div>

              {/* Copyright / Footer */}
              <div className="text-center text-[11px] text-muted-foreground pt-1">
                Copyright © 2026 Takealot AutoBest. All rights reserved.
              </div>
            </div>
          )}
        </div>
      </div>
    </Dialog>
  )
}
