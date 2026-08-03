/**
 * Session storage: the bearer token issued at join time is kept per room code so
 * a refresh (or the Jira OAuth round trip) does not kick the player out.
 */

export interface StoredSession {
  token: string
  participantId: string
  name: string
}

const PREFIX = 'estimeet.session.'

function key(code: string): string {
  return `${PREFIX}${code.toUpperCase()}`
}

export function loadSession(code: string): StoredSession | null {
  try {
    const raw = window.localStorage.getItem(key(code))
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<StoredSession>
    if (!parsed.token || !parsed.participantId) return null
    return { token: parsed.token, participantId: parsed.participantId, name: parsed.name ?? '' }
  } catch {
    return null
  }
}

export function saveSession(code: string, session: StoredSession): void {
  try {
    window.localStorage.setItem(key(code), JSON.stringify(session))
  } catch {
    // Private-mode browsers may refuse to persist; the session still works in memory.
  }
}

export function clearSession(code: string): void {
  try {
    window.localStorage.removeItem(key(code))
  } catch {
    // Ignore.
  }
}

const LAST_NAME_KEY = 'estimeet.lastName'

export function rememberName(name: string): void {
  try {
    window.localStorage.setItem(LAST_NAME_KEY, name)
  } catch {
    // Ignore.
  }
}

export function recallName(): string {
  try {
    return window.localStorage.getItem(LAST_NAME_KEY) ?? ''
  } catch {
    return ''
  }
}
