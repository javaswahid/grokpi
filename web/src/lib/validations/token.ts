import { z } from 'zod'

export const tokenStatusSchema = z.enum(['active', 'disabled', 'expired', 'cooling', 'quota_exhausted', 'invalid', 'refresh_required', 'refresh_failed'])
export const quotaModeSchema = z.enum(['limited', 'daily_reset', 'unlimited', 'manual'])

export const tokenUpdateSchema = z.object({
  status: tokenStatusSchema.optional(),
  pool: z.string().optional(),
  chat_quota: z.number().int().min(0).optional(),
  image_quota: z.number().int().min(0).optional(),
  video_quota: z.number().int().min(0).optional(),
  quota_mode: quotaModeSchema.optional(),
  priority: z.number().int().min(0).optional(),
  remark: z.string().max(500, 'Remark must be 500 characters or less').optional(),
  nsfw_enabled: z.boolean().optional(),
})

export type TokenUpdateInput = z.infer<typeof tokenUpdateSchema>
