import type {
  ImportResult,
  JiraIssue,
  JiraProject,
  Mode,
  RoomState,
  RoomSummary,
  SessionResponse,
} from './types'

const API_BASE = '/api'

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
  if (token) headers.Authorization = `Bearer ${token}`

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
  createRoom: (input: { name: string; mode: Mode; hostName: string; autoReveal: boolean }) =>
    request<SessionResponse>('/rooms', { method: 'POST', body: input }),

  roomSummary: (code: string, signal?: AbortSignal) =>
    request<RoomSummary>(`/rooms/${encodeURIComponent(code)}`, { signal }),

  joinRoom: (code: string, input: { name: string; asObserver: boolean }) =>
    request<SessionResponse>(`/rooms/${encodeURIComponent(code)}/join`, { method: 'POST', body: input }),

  state: (code: string, token: string, signal?: AbortSignal) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/state`, { token, signal }),

  updateRoom: (code: string, token: string, input: { name: string; autoReveal: boolean }) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}`, { method: 'PATCH', token, body: input }),

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

  jiraConnect: (code: string, token: string) =>
    request<{ authorizeUrl: string }>(`/rooms/${encodeURIComponent(code)}/jira/connect`, {
      method: 'POST',
      token,
    }),

  jiraDisconnect: (code: string, token: string) =>
    request<RoomState>(`/rooms/${encodeURIComponent(code)}/jira`, { method: 'DELETE', token }),

  jiraProjects: (code: string, token: string, query: string) =>
    request<{ projects: JiraProject[] }>(
      `/rooms/${encodeURIComponent(code)}/jira/projects?query=${encodeURIComponent(query)}`,
      { token },
    ),

  jiraEpics: (code: string, token: string, project: string, query: string) =>
    request<{ epics: JiraIssue[] }>(
      `/rooms/${encodeURIComponent(code)}/jira/epics?project=${encodeURIComponent(project)}&query=${encodeURIComponent(query)}`,
      { token },
    ),

  jiraEpicIssues: (code: string, token: string, epicKey: string) =>
    request<{ issues: JiraIssue[] }>(
      `/rooms/${encodeURIComponent(code)}/jira/epics/${encodeURIComponent(epicKey)}/issues`,
      { token },
    ),

  jiraImport: (code: string, token: string, keys: string[]) =>
    request<{ result: ImportResult; state: RoomState }>(`/rooms/${encodeURIComponent(code)}/jira/import`, {
      method: 'POST',
      token,
      body: { keys },
    }),
}
