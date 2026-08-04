/**
 * Where the API lives.
 *
 * Empty means "the same origin as the page", which is what the dev proxy and
 * the single-container deployment both use. A build for GitHub Pages has no
 * backend next to it, so it sets VITE_API_BASE_URL to the API's public origin
 * instead.
 */
const origin = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/+$/, '')

export const API_BASE = `${origin}/api`

/** socketUrl turns an API path into a ws:// or wss:// URL on the same host. */
export function socketUrl(path: string): string {
  const base = origin || window.location.origin
  return `${base.replace(/^http/, 'ws')}/api${path}`
}
