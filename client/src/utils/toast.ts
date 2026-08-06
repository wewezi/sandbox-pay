import { toast } from '@heroui/react'

/**
 * Thin convenience wrapper around HeroUI's global toast.
 * Import directly — no React context needed.
 */
export const showToast = {
  success: (msg: string) => toast.success(msg),
  error: (msg: string) => toast.danger(msg),
  info: (msg: string) => toast.info(msg),
}
