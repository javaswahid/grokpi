'use client'

import { Progress } from '@/components/ui'
import { cn } from '@/lib/utils'
import { useTranslation } from '@/lib/i18n/context'
import type { Token } from '@/types'
import { buildTokenQuotaMetrics, quotaProgressColor, quotaSurfaceColor, quotaTextColor } from './token-quota-utils'

function formatTokenDate(value: string | null): string {
  if (!value) return '-'
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

export function TokenDetails({ token }: { token: Token }) {
  const { t } = useTranslation()
  const quotaMetrics = buildTokenQuotaMetrics(token, {
    chat: t.tokens.chatQuota,
    image: t.tokens.imageQuota,
    video: t.tokens.videoQuota,
  })

  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        {quotaMetrics.map((metric) => (
          <div key={metric.key} className={cn('rounded-lg border p-3', quotaSurfaceColor(metric.percent))}>
            <div className="flex items-start justify-between gap-3">
              <span className="text-sm text-muted">{metric.label}</span>
              <span className={cn('text-xs font-semibold', quotaTextColor(metric.percent))}>{metric.percent.toFixed(0)}%</span>
            </div>
            <div className="mt-1 text-base font-semibold">{metric.remaining} / {metric.total}</div>
            <Progress value={metric.percent} className={cn('mt-3 h-2', quotaProgressColor(metric.percent))} />
          </div>
        ))}
      </div>

      <div className="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
        <div>
          <span className="text-muted">{t.tokens.failCount}</span>
          <span className="ml-2 font-medium">{token.fail_count || 0}</span>
        </div>
        <div>
          <span className="text-muted">{t.tokens.lastUsed}</span>
          <span className="ml-2 font-medium">{formatTokenDate(token.last_used)}</span>
        </div>
        <div>
          <span className="text-muted">{t.tokens.coolUntil}</span>
          <span className="ml-2 font-medium">{formatTokenDate(token.cool_until)}</span>
        </div>
        <div>
          <span className="text-muted">Expires:</span>
          <span className="ml-2 font-medium">{formatTokenDate(token.expires_at ?? null)}</span>
        </div>
        <div>
          <span className="text-muted">Validated:</span>
          <span className="ml-2 font-medium">{formatTokenDate(token.last_validated_at ?? null)}</span>
        </div>
        <div>
          <span className="text-muted">Refresh:</span>
          <span className="ml-2 font-medium">{token.refresh_status || '-'}</span>
        </div>
        <div>
          <span className="text-muted">Version:</span>
          <span className="ml-2 font-medium">{token.token_version || 1}</span>
        </div>
        <div>
          <span className="text-muted">Quota Mode:</span>
          <span className="ml-2 font-medium">{token.quota_mode || 'limited'}</span>
        </div>
        <div>
          <span className="text-muted">Video Capable:</span>
          <span className={cn('ml-2 font-medium', token.upstream_video_capable === false && 'text-red-600', token.upstream_video_capable === true && 'text-green-700')}>
            {token.upstream_video_capable === true ? 'yes' : token.upstream_video_capable === false ? 'no' : 'unknown'}
          </span>
        </div>
        <div>
          <span className="text-muted">Video Check:</span>
          <span className="ml-2 font-medium">{token.last_video_check_status || '-'}</span>
        </div>
        <div>
          <span className="text-muted">Video Checked At:</span>
          <span className="ml-2 font-medium">{formatTokenDate(token.last_video_check_at ?? null)}</span>
        </div>
        <div>
          <span className="text-muted">Video Used:</span>
          <span className="ml-2 font-medium">{token.local_video_used ?? Math.max((token.total_video_quota || 0) - (token.video_quota || 0), 0)} / {token.configured_video_quota ?? token.total_video_quota}</span>
        </div>
        <div>
          <span className="text-muted">{t.tokens.priority}:</span>
          <span className="ml-2 font-medium">{token.priority ?? 0}</span>
        </div>
        <div>
          <span className="text-muted">{t.tokens.nsfw}:</span>
          <span className="ml-2 font-medium">
            {token.nsfw_enabled ? t.common.enabled : t.common.disabled}
          </span>
        </div>
        <div className="md:col-span-2">
          <span className="text-muted">{t.tokens.remark}:</span>
          <span className="ml-2 font-medium">{token.remark || '-'}</span>
        </div>
        {token.refresh_error && (
          <div className="md:col-span-4">
            <span className="text-muted">Refresh Error:</span>
            <span className="ml-2 font-medium text-red-600">{token.refresh_error}</span>
          </div>
        )}
        {token.warning_message && (
          <div className="md:col-span-4">
            <span className="text-muted">Warning:</span>
            <span className="ml-2 font-medium text-red-600">{token.warning_message}</span>
          </div>
        )}
        {token.last_video_error && (
          <div className="md:col-span-4">
            <span className="text-muted">Last Video Error:</span>
            <span className="ml-2 font-medium text-red-600">{token.last_video_error}</span>
          </div>
        )}
        <div className="md:col-span-2">
          <span className="text-muted">{t.tokens.createdAt}</span>
          <span className="ml-2 font-medium">{formatTokenDate(token.created_at)}</span>
        </div>
      </div>
    </div>
  )
}
