// Token types mirroring Go models

export interface Token {
  id: number
  token: string
  pool: string
  status: TokenStatus
  status_reason?: string
  quota_mode: QuotaMode
  chat_quota: number
  total_chat_quota: number
  image_quota: number
  total_image_quota: number
  video_quota: number
  total_video_quota: number
  fail_count: number
  cool_until: string | null
  last_used: string | null
  expires_at?: string | null
  last_validated_at?: string | null
  last_refresh_attempt_at?: string | null
  refresh_status?: string
  refresh_error?: string
  refresh_count?: number
  token_version?: number
  replaced_at?: string | null
  warning_message?: string
  configured_video_quota?: number
  local_video_used?: number
  local_video_remaining?: number
  upstream_video_capable?: boolean | null
  last_video_check_at?: string | null
  last_video_check_status?: string
  last_video_error?: string
  priority: number
  remark?: string
  nsfw_enabled?: boolean
  created_at: string
  updated_at: string
}

export type TokenStatus = 'active' | 'disabled' | 'expired' | 'cooling' | 'quota_exhausted' | 'invalid' | 'refresh_required' | 'refresh_failed'
export type QuotaMode = 'limited' | 'daily_reset' | 'unlimited' | 'manual'

export interface TokenUpdateRequest {
  status?: TokenStatus
  pool?: string
  chat_quota?: number
  image_quota?: number
  video_quota?: number
  quota_mode?: QuotaMode
  priority?: number
  remark?: string
  nsfw_enabled?: boolean
}

export interface TokenReplaceRequest {
  token: string
  status?: TokenStatus
  quota_mode?: QuotaMode
  chat_quota?: number
  image_quota?: number
  video_quota?: number
}
