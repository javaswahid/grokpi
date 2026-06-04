'use client'

import * as React from 'react'
import {
  Activity,
  AlertTriangle,
  Bot,
  CheckCircle2,
  Clipboard,
  Code2,
  DatabaseBackup,
  Film,
  Image as ImageIcon,
  KeyRound,
  Network,
  Play,
  ShieldCheck,
  Terminal,
  Zap,
} from 'lucide-react'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  Progress,
  Skeleton,
  Textarea,
  useToast,
} from '@/components/ui'
import { getApiKey, setApiKey } from '@/lib/function-api'
import { useAPIKeys, useUsageLogs, useUsageStats } from '@/lib/hooks'
import { formatNumber } from '@/lib/utils'
import type { APIKey, DashboardTokenStats, PoolQuota, SystemStatus, UsageStats } from '@/types'

type PlaygroundMode = 'chat' | 'image' | 'video'

interface OpsStudioProps {
  status?: SystemStatus
  tokenStats?: DashboardTokenStats
  apiKeyStats?: { active: number; total: number; inactive: number; rate_limited: number; expired: number }
  quotaPools?: PoolQuota[]
  loading: boolean
}

const commandSnippets = [
  {
    label: 'Health',
    icon: Activity,
    command: 'curl -fsS https://www.grokpi.masjavas.my.id/health',
  },
  {
    label: 'Containers',
    icon: Terminal,
    command: 'cd /opt/grokpi && sudo docker compose ps',
  },
  {
    label: 'Logs',
    icon: Code2,
    command: 'cd /opt/grokpi && sudo docker compose logs --tail=120 grokpi',
  },
  {
    label: 'Backup',
    icon: DatabaseBackup,
    command: 'sudo tar -czf /home/masjavas/deploy-backups/grokpi-live-$(date -u +%Y%m%d-%H%M%S).tar.gz -C /opt/grokpi config.toml data docker-compose.yml',
  },
]

const packageProfiles = [
  { name: 'internal', daily: '5,000/day', rate: '120/min', models: 'All operational models' },
  { name: 'bot', daily: '1,000/day', rate: '60/min', models: 'Chat + mini models' },
  { name: 'client', daily: '2,500/day', rate: '90/min', models: 'Chat, image, video by need' },
  { name: 'reseller', daily: '10,000/day', rate: '180/min', models: 'Whitelist per package' },
]

const playgroundPresets: Record<PlaygroundMode, { label: string; icon: typeof Bot; model: string; prompt: string }> = {
  chat: {
    label: 'Chat',
    icon: Bot,
    model: 'grok-3-mini',
    prompt: 'Buat ringkasan singkat status GrokPi hari ini.',
  },
  image: {
    label: 'Image',
    icon: ImageIcon,
    model: 'grok-imagine-1.0',
    prompt: 'A polished API operations dashboard on a clean workstation.',
  },
  video: {
    label: 'Video',
    icon: Film,
    model: 'grok-imagine-1.0-video',
    prompt: 'A cinematic sweep across a modern AI operations room.',
  },
}

export function DashboardOpsStudio({ status, tokenStats, apiKeyStats, quotaPools, loading }: OpsStudioProps) {
  const { toast } = useToast()
  const [isHttps, setIsHttps] = React.useState(false)
  const { data: dayUsage, isLoading: usageLoading } = useUsageStats('day')
  const { data: apiKeys, isLoading: keysLoading } = useAPIKeys({ page: 1, page_size: 100 })
  const { data: recentFailures, isLoading: failuresLoading } = useUsageLogs({
    page: 1,
    pageSize: 5,
    sortBy: 'time',
    sortDir: 'desc',
    period: 'day',
    status: '4',
  })

  React.useEffect(() => {
    setIsHttps(window.location.protocol === 'https:')
  }, [])

  const errorRate = computeErrorRate(dayUsage?.requests ?? 0, dayUsage?.errors ?? 0)
  const lowestQuota = lowestQuotaPercent(quotaPools)
  const recommendations = buildRecommendations({
    activeTokens: tokenStats?.active ?? 0,
    totalTokens: tokenStats?.total ?? 0,
    activeKeys: apiKeyStats?.active ?? 0,
    errors: dayUsage?.errors ?? 0,
    lowestQuota,
  })

  return (
    <div className="space-y-4">
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(360px,0.75fr)]">
        <Card>
          <CardHeader className="pb-4">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle className="text-base">Command Center</CardTitle>
                <p className="mt-1 text-[13px] text-muted">MASJAVAS GrokPi operations</p>
              </div>
              <Badge variant={status?.status === 'healthy' ? 'success' : 'warning'}>
                {status?.status ?? 'checking'}
              </Badge>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <OpsMetric label="Public API" value={status?.status === 'healthy' ? 'Online' : 'Check'} tone={status?.status === 'healthy' ? 'success' : 'warning'} icon={Network} loading={loading} />
              <OpsMetric label="HTTPS" value={isHttps ? 'TLS active' : 'Local'} tone={isHttps ? 'success' : 'warning'} icon={ShieldCheck} />
              <OpsMetric label="Token Pool" value={`${tokenStats?.active ?? 0}/${tokenStats?.total ?? 0}`} tone={(tokenStats?.active ?? 0) > 0 ? 'success' : 'warning'} icon={KeyRound} loading={loading} />
              <OpsMetric label="API Keys" value={`${apiKeyStats?.active ?? 0}/${apiKeyStats?.total ?? 0}`} tone={(apiKeyStats?.active ?? 0) > 0 ? 'success' : 'warning'} icon={KeyRound} loading={loading} />
              <OpsMetric label="Error Rate" value={`${errorRate.toFixed(1)}%`} tone={errorRate <= 2 ? 'success' : errorRate <= 10 ? 'warning' : 'danger'} icon={AlertTriangle} loading={usageLoading} />
              <OpsMetric label="Requests Today" value={formatNumber(dayUsage?.requests ?? 0)} tone="neutral" icon={Activity} loading={usageLoading} />
            </div>

            <div className="rounded-md border border-border bg-[rgba(255,255,255,0.5)] p-4 dark:bg-white/[0.03]">
              <div className="mb-3 flex items-center justify-between gap-3">
                <div className="text-sm font-semibold">Operational Priorities</div>
                <Badge variant={recommendations.length === 1 && recommendations[0].tone === 'success' ? 'success' : 'warning'}>
                  {recommendations.length}
                </Badge>
              </div>
              <div className="grid gap-2 md:grid-cols-2">
                {recommendations.map((item) => (
                  <div key={item.label} className="flex items-start gap-2 rounded-[4px] bg-black/[0.025] px-3 py-2 text-[13px] dark:bg-white/[0.04]">
                    {item.tone === 'success' ? <CheckCircle2 className="mt-0.5 h-4 w-4 text-success" /> : <AlertTriangle className="mt-0.5 h-4 w-4 text-warning" />}
                    <span>{item.label}</span>
                  </div>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>

        <ApiPlayground />
      </section>

      <section className="grid gap-4 xl:grid-cols-3">
        <PoolHealthCard pools={quotaPools} loading={loading} />
        <UsageIntelligenceCard usage={dayUsage} loading={usageLoading} />
        <TroubleshootingCard failures={recentFailures?.data ?? []} loading={failuresLoading} />
      </section>

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <KeyPackagesCard keys={apiKeys?.data ?? []} loading={keysLoading} />
        <RunbookCard onCopy={(command) => {
          navigator.clipboard?.writeText(command)
          toast({ title: 'Command copied' })
        }} />
      </section>
    </div>
  )
}

function OpsMetric({ label, value, tone, icon: Icon, loading }: { label: string; value: string; tone: 'success' | 'warning' | 'danger' | 'neutral'; icon: typeof Activity; loading?: boolean }) {
  const toneClass = {
    success: 'text-success bg-success/10',
    warning: 'text-warning bg-warning/10',
    danger: 'text-destructive bg-destructive/10',
    neutral: 'text-primary bg-primary/8',
  }[tone]

  return (
    <div className="flex min-h-[76px] items-center gap-3 rounded-md border border-border bg-background/70 p-3">
      <div className={`flex h-9 w-9 items-center justify-center rounded-[4px] ${toneClass}`}>
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <div className="text-[12px] font-medium text-muted">{label}</div>
        {loading ? <Skeleton className="mt-2 h-5 w-20" /> : <div className="mt-1 truncate text-lg font-semibold">{value}</div>}
      </div>
    </div>
  )
}

function PoolHealthCard({ pools, loading }: { pools?: PoolQuota[]; loading: boolean }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Pool Health</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {loading ? (
          <Skeleton className="h-32" />
        ) : !pools?.length ? (
          <EmptyState label="No pool quota yet" />
        ) : (
          pools.map((pool) => {
            const percent = quotaPercent(pool)
            return (
              <div key={pool.pool} className="space-y-2">
                <div className="flex items-center justify-between gap-3 text-sm">
                  <span className="font-medium">{pool.pool}</span>
                  <span className="text-muted">{percent.toFixed(0)}%</span>
                </div>
                <Progress value={percent} className={percent > 50 ? '[&>div]:bg-success' : percent > 20 ? '[&>div]:bg-warning' : '[&>div]:bg-destructive'} />
                <div className="grid grid-cols-3 gap-2 text-[12px] text-muted">
                  <span>Chat {pool.remaining_chat_quota}/{pool.total_chat_quota}</span>
                  <span>Image {pool.remaining_image_quota}/{pool.total_image_quota}</span>
                  <span>Video {pool.remaining_video_quota}/{pool.total_video_quota}</span>
                </div>
              </div>
            )
          })
        )}
      </CardContent>
    </Card>
  )
}

function UsageIntelligenceCard({ usage, loading }: { usage?: UsageStats; loading: boolean }) {
  const topModels = topModelRows(usage?.by_model)
  const topKeys = [...(usage?.by_api_key ?? [])].sort((a, b) => b.requests - a.requests).slice(0, 3)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Usage Intelligence</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {loading ? (
          <Skeleton className="h-32" />
        ) : (
          <>
            <InsightList title="Top Models" rows={topModels.map(([name, stats]) => ({ name, value: `${stats.requests} req` }))} />
            <InsightList title="Top API Keys" rows={topKeys.map((item) => ({ name: item.api_key_name, value: `${item.requests} req` }))} />
          </>
        )}
      </CardContent>
    </Card>
  )
}

function TroubleshootingCard({ failures, loading }: { failures: Array<{ id: number; api_key_name: string; model: string; status: number; created_at: string }>; loading: boolean }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Troubleshooting</CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-32" />
        ) : failures.length === 0 ? (
          <EmptyState label="No client-side 4xx failures today" />
        ) : (
          <div className="space-y-2">
            {failures.map((item) => (
              <div key={item.id} className="flex items-center justify-between gap-3 rounded-[4px] border border-border px-3 py-2 text-[12px]">
                <div className="min-w-0">
                  <div className="truncate font-medium">{item.api_key_name || 'unknown key'}</div>
                  <div className="truncate text-muted">{item.model || 'unknown model'}</div>
                </div>
                <Badge variant="warning">{item.status}</Badge>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function KeyPackagesCard({ keys, loading }: { keys: APIKey[]; loading: boolean }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Key Management Packages</CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-36" />
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {packageProfiles.map((profile) => {
              const existing = keys.find((key) => key.name.toLowerCase().includes(profile.name))
              return (
                <div key={profile.name} className="rounded-md border border-border p-3">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-sm font-semibold capitalize">{profile.name}</span>
                    <Badge variant={existing?.status === 'active' ? 'success' : existing ? 'warning' : 'outline'}>
                      {existing?.status ?? 'template'}
                    </Badge>
                  </div>
                  <div className="mt-3 grid gap-1 text-[12px] text-muted">
                    <span>{profile.rate}</span>
                    <span>{profile.daily}</span>
                    <span>{profile.models}</span>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function RunbookCard({ onCopy }: { onCopy: (command: string) => void }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Ops Runbook</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3 md:grid-cols-2">
        {commandSnippets.map(({ label, command, icon: Icon }) => (
          <button
            key={label}
            type="button"
            onClick={() => onCopy(command)}
            className="flex min-h-[72px] items-center gap-3 rounded-md border border-border bg-background/70 p-3 text-left transition-colors hover:bg-black/[0.03] dark:hover:bg-white/[0.04]"
          >
            <div className="flex h-9 w-9 items-center justify-center rounded-[4px] bg-primary/8 text-primary">
              <Icon className="h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-sm font-semibold">{label}</div>
              <div className="mt-1 truncate font-mono text-[11px] text-muted">{command}</div>
            </div>
            <Clipboard className="h-4 w-4 text-muted" />
          </button>
        ))}
      </CardContent>
    </Card>
  )
}

function ApiPlayground() {
  const [apiKeyValue, setApiKeyValue] = React.useState('')
  const [mode, setMode] = React.useState<PlaygroundMode>('chat')
  const [prompt, setPrompt] = React.useState(playgroundPresets.chat.prompt)
  const [loading, setLoading] = React.useState(false)
  const [result, setResult] = React.useState<{ status: number; summary: string } | null>(null)
  const { toast } = useToast()

  React.useEffect(() => {
    const key = getApiKey()
    if (key) setApiKeyValue(key)
  }, [])

  const run = async () => {
    const key = apiKeyValue.trim()
    if (!key) {
      toast({ title: 'API key required', variant: 'destructive' })
      return
    }
    setApiKey(key)
    setLoading(true)
    setResult(null)
    try {
      const preset = playgroundPresets[mode]
      const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${key}`,
        },
        body: JSON.stringify(buildPlaygroundBody(mode, preset.model, prompt)),
      })
      const payload = await response.json().catch(() => null)
      setResult({ status: response.status, summary: summarizePlayground(payload) })
    } catch (error) {
      setResult({ status: 0, summary: error instanceof Error ? error.message : 'Request failed' })
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card>
      <CardHeader className="pb-4">
        <div className="flex items-center justify-between gap-3">
          <CardTitle className="text-base">API Playground</CardTitle>
          <Badge variant="secondary">{playgroundPresets[mode].model}</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid grid-cols-3 gap-2">
          {(Object.keys(playgroundPresets) as PlaygroundMode[]).map((item) => {
            const preset = playgroundPresets[item]
            const Icon = preset.icon
            return (
              <Button
                key={item}
                type="button"
                variant={mode === item ? 'default' : 'outline'}
                size="sm"
                onClick={() => {
                  setMode(item)
                  setPrompt(playgroundPresets[item].prompt)
                }}
              >
                <Icon className="h-4 w-4" />
                {preset.label}
              </Button>
            )
          })}
        </div>
        <Input
          value={apiKeyValue}
          onChange={(event) => setApiKeyValue(event.target.value)}
          placeholder="API key"
          type="password"
        />
        <Textarea
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          className="min-h-[96px]"
        />
        <Button type="button" onClick={run} disabled={loading || !prompt.trim()} className="w-full">
          {loading ? <Zap className="h-4 w-4 animate-pulse" /> : <Play className="h-4 w-4" />}
          {loading ? 'Running' : 'Run Test'}
        </Button>
        {result && (
          <div className="rounded-md border border-border bg-black/[0.025] p-3 dark:bg-white/[0.04]">
            <div className="mb-1 flex items-center justify-between text-[12px]">
              <span className="font-semibold">HTTP {result.status}</span>
              <Badge variant={result.status >= 200 && result.status < 300 ? 'success' : 'warning'}>
                {result.status >= 200 && result.status < 300 ? 'ok' : 'check'}
              </Badge>
            </div>
            <p className="line-clamp-4 text-[12px] text-muted">{result.summary}</p>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function InsightList({ title, rows }: { title: string; rows: Array<{ name: string; value: string }> }) {
  return (
    <div>
      <div className="mb-2 text-[12px] font-semibold uppercase text-muted">{title}</div>
      {rows.length === 0 ? (
        <EmptyState label="No usage yet" compact />
      ) : (
        <div className="space-y-2">
          {rows.map((row) => (
            <div key={`${title}-${row.name}`} className="flex items-center justify-between gap-3 text-[13px]">
              <span className="truncate">{row.name}</span>
              <span className="font-medium text-muted">{row.value}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function EmptyState({ label, compact = false }: { label: string; compact?: boolean }) {
  return (
    <div className={`flex items-center justify-center rounded-md border border-dashed border-border text-[13px] text-muted ${compact ? 'h-10' : 'h-24'}`}>
      {label}
    </div>
  )
}

function computeErrorRate(success: number, errors: number): number {
  const total = success + errors
  return total <= 0 ? 0 : (errors / total) * 100
}

function quotaPercent(pool: PoolQuota): number {
  const total = pool.total_chat_quota + pool.total_image_quota + pool.total_video_quota
  const remaining = pool.remaining_chat_quota + pool.remaining_image_quota + pool.remaining_video_quota
  return total <= 0 ? 0 : (remaining / total) * 100
}

function lowestQuotaPercent(pools?: PoolQuota[]): number | null {
  if (!pools?.length) return null
  return Math.min(...pools.map(quotaPercent))
}

function topModelRows(byModel?: UsageStats['by_model']) {
  if (!byModel) return []
  return Object.entries(byModel).sort((a, b) => b[1].requests - a[1].requests).slice(0, 3)
}

function buildRecommendations(input: { activeTokens: number; totalTokens: number; activeKeys: number; errors: number; lowestQuota: number | null }) {
  const items: Array<{ label: string; tone: 'success' | 'warning' }> = []
  if (input.activeTokens === 0) items.push({ label: 'Import upstream Grok SSO tokens', tone: 'warning' })
  if (input.totalTokens > 0 && input.activeTokens < input.totalTokens) items.push({ label: 'Review cooling, disabled, or expired tokens', tone: 'warning' })
  if (input.activeKeys === 0) items.push({ label: 'Create at least one production API key', tone: 'warning' })
  if (input.lowestQuota != null && input.lowestQuota < 20) items.push({ label: 'Add capacity to the lowest quota pool', tone: 'warning' })
  if (input.errors > 0) items.push({ label: 'Inspect failed requests from Usage logs', tone: 'warning' })
  if (items.length === 0) items.push({ label: 'Gateway is ready for production traffic', tone: 'success' })
  return items
}

function buildPlaygroundBody(mode: PlaygroundMode, model: string, prompt: string) {
  if (mode === 'image') {
    return {
      model,
      stream: false,
      messages: [{ role: 'user', content: prompt }],
      image_config: { n: 1, size: '1024x1024', response_format: 'b64_json' },
    }
  }
  if (mode === 'video') {
    return {
      model,
      stream: false,
      messages: [{ role: 'user', content: prompt }],
      video_config: { aspect_ratio: '16:9', video_length: 6, resolution_name: '480p', preset: 'normal' },
    }
  }
  return {
    model,
    stream: false,
    messages: [{ role: 'user', content: prompt }],
  }
}

function summarizePlayground(payload: unknown): string {
  if (!payload || typeof payload !== 'object') return 'No JSON payload returned'
  const data = payload as { error?: { message?: string; code?: string }; choices?: Array<{ message?: { content?: string } }> }
  if (data.error) return data.error.message || data.error.code || 'Request failed'
  const content = data.choices?.[0]?.message?.content
  if (!content) return 'Request completed'
  if (content.includes('data:image') || content.length > 1200) return 'Media payload returned successfully'
  return content.slice(0, 500)
}
