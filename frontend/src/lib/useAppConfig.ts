import { useEffect, useState } from 'react'

import { api } from './api'
import type { AppConfig } from './types'

// The config never changes while the tab is open, so the first component to ask
// for it starts the request and every later one reuses the same promise.
let pending: Promise<AppConfig> | null = null
let cached: AppConfig | null = null

/** useAppConfig returns the server-wide settings, or null until they arrive. */
export function useAppConfig(): AppConfig | null {
  const [config, setConfig] = useState<AppConfig | null>(cached)

  useEffect(() => {
    if (cached) return
    let active = true
    pending ??= api.config()
    void pending
      .then((next) => {
        cached = next
        if (active) setConfig(next)
      })
      .catch(() => {
        // A missing config only costs the footer and the source picker; the
        // rest of the app works without it, so there is nothing to report.
        pending = null
      })
    return () => {
      active = false
    }
  }, [])

  return config
}
