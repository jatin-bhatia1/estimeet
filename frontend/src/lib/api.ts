import { API_BASE } from './origin'
import type {
  AppConfig,
  ImportResult,
  Mode,
  RoomState,
  RoomSummary,
  SessionResponse,
  SourceContainer,
  SourceItem,
} from './types'

/** ApiError carries the HTTP status so callers can react to 403/409 specifically. */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

interface RequestOptions {
  method?: string
  body?: unknown
  token?: string
  signal?: AbortSignal
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, token, signal } = options

  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  // Authorization goes too, for a backend deployed without the header below.
  if (token) {
    headers['X-Estimeet-Token'] = token
    headers.Authorization = `Bearer ${token}`
  }

  const response = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    signal,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (!response.ok) {
    let message = `Request failed (${response.status})`
    try {
      const parsed = (await response.json()) as { error?: string }
      if (parsed?.error) message = parsed.error
    } catch {
      // Keep the generic message when the body is not JSON.
    }
    throw new ApiError(response.status, message)
  }

  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

export const api = {
  createRoom: (input: {
    name: string
    mode: Mode
    hostName: string
    autoReveal: boolean
    expectedSize?: number
    expectedNames?: string[]
    deck?: string[]
  }) =>
    request<SessionResponse>('/rooms', { method: 'POST', body: input }),

  roomSummary: (code: string, signal?: AbortSignal) =>
    request<RoomSummary>(`/rooms/${encodeURIComponent(code)}`, { signal }),

  joinRoom: (code: string, input: { name: string; asObserver: boolean }) =>
    request<SessionResponse>(`/rooms/${encodeURIComponent(code)}/join`, { method: 'POST', body: input }),

  state: (code: string, token: string, signal?: AbortSignal) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/state`, { token, signal }),

  updateRoom: (code: string, token: string, input: { name: string; autoReveal: boolean }) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}`, { method: 'PATCH', token, body: input }),

  setRoster: (code: string, token: string, input: { size: number; names: string[] }) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/roster`, { method: 'PUT', token, body: input }),

  setDeck: (code: string, token: string, cards: string[]) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/deck`, { method: 'PUT', token, body: { cards } }),

  updateProfile: (code: string, token: string, input: { name: string; isObserver: boolean }) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/me`, { method: 'PATCH', token, body: input }),

  kick: (code: string, token: string, participantId: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/participants/${participantId}`, {
      method: 'DELETE',
      token,
    }),

  addTopics: (code: string, token: string, topics: { title: string; description: string }[]) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics`, { method: 'POST', token, body: { topics } }),

  updateTopic: (code: string, token: string, topicId: string, input: { title: string; description: string }) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics/${topicId}`, {
      method: 'PATCH',
      token,
      body: input,
    }),

  deleteTopic: (code: string, token: string, topicId: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics/${topicId}`, { method: 'DELETE', token }),

  reorderTopics: (code: string, token: string, topicIds: string[]) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics/reorder`, {
      method: 'POST',
      token,
      body: { topicIds },
    }),

  vote: (code: string, token: string, topicId: string, value: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics/${topicId}/vote`, {
      method: 'POST',
      token,
      body: { value },
    }),

  clearVote: (code: string, token: string, topicId: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics/${topicId}/vote`, { method: 'DELETE', token }),

  reveal: (code: string, token: string, topicId: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics/${topicId}/reveal`, { method: 'POST', token }),

  resetTopic: (code: string, token: string, topicId: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics/${topicId}/reset`, { method: 'POST', token }),

  estimate: (code: string, token: string, topicId: string, value: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/topics/${topicId}/estimate`, {
      method: 'POST',
      token,
      body: { value },
    }),

  setCurrent: (code: string, token: string, body: { topicId?: string; direction?: 'next' | 'prev' }) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/current`, { method: 'POST', token, body }),

  config: (signal?: AbortSignal) => request<AppConfig>('/config', { signal }),

  /** Starts Atlassian's OAuth flow; every other tracker uses sourceConnect. */
  jiraConnect: (code: string, token: string) =>
    request<{ authorizeUrl: string }>(`/rooms/${encodeURIComponent(code)}/jira/connect`, {
      method: 'POST',
      token,
    }),

  sourceConnect: (
    code: string,
    token: string,
    input: { provider: string; baseUrl: string; account: string; token: string },
  ) => request<RoomState>(`/rooms/${encodeURIComponent(code)}/source`, { method: 'POST', token, body: input }),

  sourceDisconnect: (code: string, token: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/source`, { method: 'DELETE', token }),

  sourceContainers: (code: string, token: string, query: string, signal?: AbortSignal) =>
    request<{ containers: SourceContainer[] }>(
      `/rooms/${encodeURIComponent(code)}/source/containers?query=${encodeURIComponent(query)}`,
      { token, signal },
    ),

  sourceGroups: (code: string, token: string, container: string, query: string, signal?: AbortSignal) =>
    request<{ groups: SourceItem[] }>(
      `/rooms/${encodeURIComponent(code)}/source/groups?container=${encodeURIComponent(container)}&query=${encodeURIComponent(query)}`,
      { token, signal },
    ),

  sourceItems: (code: string, token: string, container: string, group: string) =>
    request<{ items: SourceItem[] }>(
      `/rooms/${encodeURIComponent(code)}/source/items?container=${encodeURIComponent(container)}&group=${encodeURIComponent(group)}`,
      { token },
    ),

  sourceImport: (code: string, token: string, container: string, group: string, keys: string[]) =>
    request<{ result: ImportResult; state: RoomState }>(`/rooms/${encodeURIComponent(code)}/source/import`, {
      method: 'POST',
      token,
      body: { container, group, keys },
    }),
}
