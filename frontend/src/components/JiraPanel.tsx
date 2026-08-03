import { useCallback, useEffect, useMemo, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'

import { ApiError, api } from '../lib/api'
import type { JiraIssue, JiraProject, RoomState } from '../lib/types'

interface JiraPanelProps {
  state: RoomState
  token: string
  onImported: (state: RoomState, imported: number, skipped: string[]) => void
  onError: (message: string) => void
}

/**
 * JiraPanel is the optional backlog source: connect the room to a Jira Cloud
 * site, search for an epic and pull its stories in as topics. Only the host
 * sees it.
 */
export function JiraPanel({ state, token, onImported, onError }: JiraPanelProps) {
  const { room, me } = state
  const [importing, setImporting] = useState(false)
  const [connectOpen, setConnectOpen] = useState(false)

  if (!me.isHost) return null

  if (!room.jiraAvailable) {
    return (
      <div className="panel p-4">
        <h2 className="text-sm font-semibold text-slate-200">Jira</h2>
        <p className="mt-1.5 text-xs leading-relaxed text-slate-500">
          The Jira integration is unavailable on this server.
        </p>
      </div>
    )
  }

  const disconnect = async () => {
    try {
      onImported(await api.jiraDisconnect(room.code, token), 0, [])
    } catch (err) {
      onError(err instanceof ApiError ? err.message : 'Could not disconnect Jira.')
    }
  }

  return (
    <div className="panel p-4">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-slate-200">Jira</h2>
        {room.jiraConnected && (
          <button type="button" onClick={() => void disconnect()} className="text-xs text-slate-600 hover:text-rose-300">
            disconnect
          </button>
        )}
      </div>

      {room.jiraConnected ? (
        <>
          <p className="mb-1 truncate text-xs text-slate-500" title={room.jiraSiteUrl}>
            Connected to <span className="text-slate-300">{room.jiraSiteName || room.jiraSiteUrl}</span>
          </p>
          <p className="mb-3 truncate text-[11px] text-slate-600">
            {room.jiraAuthType === 'token' ? `API token · ${room.jiraAccountEmail ?? ''}` : 'OAuth'}
          </p>
          <button type="button" className="btn-ghost w-full" onClick={() => setImporting(true)}>
            Search epics and import
          </button>
        </>
      ) : (
        <>
          <p className="mb-3 text-xs leading-relaxed text-slate-500">
            Connect the room to your Jira site to pull the stories of an epic straight into the backlog.
          </p>
          <button type="button" className="btn-ghost w-full" onClick={() => setConnectOpen(true)}>
            Connect to Jira
          </button>
        </>
      )}

      {connectOpen && (
        <ConnectDialog
          roomCode={room.code}
          token={token}
          oauthAvailable={room.jiraOauthAvailable}
          onClose={() => setConnectOpen(false)}
          onConnected={(next) => {
            setConnectOpen(false)
            onImported(next, 0, [])
          }}
          onError={onError}
        />
      )}

      {importing && (
        <ImportDialog
          roomCode={room.code}
          token={token}
          onClose={() => setImporting(false)}
          onImported={onImported}
          onError={onError}
        />
      )}
    </div>
  )
}

// ------------------------------------------------------------------ connect

interface ConnectDialogProps {
  roomCode: string
  token: string
  oauthAvailable: boolean
  onClose: () => void
  onConnected: (state: RoomState) => void
  onError: (message: string) => void
}

const API_TOKEN_HELP = 'https://id.atlassian.com/manage-profile/security/api-tokens'

/**
 * ConnectDialog offers the two ways to link a room to Jira Cloud: an Atlassian
 * API token, which needs no server configuration, and the OAuth flow when this
 * server has an Atlassian app registered.
 */
function ConnectDialog({ roomCode, token, oauthAvailable, onClose, onConnected, onError }: ConnectDialogProps) {
  const [siteUrl, setSiteUrl] = useState('')
  const [email, setEmail] = useState('')
  const [apiToken, setApiToken] = useState('')
  const [busy, setBusy] = useState<'oauth' | 'token' | null>(null)
  const [error, setError] = useState<string | null>(null)

  const startOAuth = async () => {
    setBusy('oauth')
    setError(null)
    try {
      const { authorizeUrl } = await api.jiraConnect(roomCode, token)
      window.location.href = authorizeUrl
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not start the Jira connection.')
      setBusy(null)
    }
  }

  const submitToken = async (event: FormEvent) => {
    event.preventDefault()
    setBusy('token')
    setError(null)
    try {
      onConnected(await api.jiraConnectToken(roomCode, token, { siteUrl, email, apiToken }))
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Could not connect to Jira.'
      setError(message)
      onError(message)
      setBusy(null)
    }
  }

  return (
    <Modal label="Connect to Jira" onClose={onClose}>
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-50">Connect to Jira</h2>
        <button type="button" onClick={onClose} className="text-slate-500 hover:text-slate-200">
          ✕
        </button>
      </div>

      <div className="min-h-0 overflow-y-auto">
        {oauthAvailable && (
          <div className="mb-5">
            <button
              type="button"
              className="btn-primary w-full"
              onClick={() => void startOAuth()}
              disabled={busy !== null}
            >
              {busy === 'oauth' ? 'Redirecting…' : 'Connect with Atlassian'}
            </button>
            <div className="my-4 flex items-center gap-3 text-[11px] uppercase tracking-wider text-slate-600">
              <span className="h-px flex-1 bg-white/10" />
              or use an API token
              <span className="h-px flex-1 bg-white/10" />
            </div>
          </div>
        )}

        <form onSubmit={submitToken} className="space-y-4">
          <div>
            <label className="label" htmlFor="jira-site">
              Jira site
            </label>
            <input
              id="jira-site"
              className="field"
              value={siteUrl}
              onChange={(e) => setSiteUrl(e.target.value)}
              placeholder="https://your-team.atlassian.net"
              autoComplete="url"
              required
            />
          </div>

          <div>
            <label className="label" htmlFor="jira-email">
              Atlassian account email
            </label>
            <input
              id="jira-email"
              type="email"
              className="field"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="ada@example.com"
              autoComplete="username"
              required
            />
          </div>

          <div>
            <label className="label" htmlFor="jira-token">
              API token
            </label>
            <input
              id="jira-token"
              type="password"
              className="field"
              value={apiToken}
              onChange={(e) => setApiToken(e.target.value)}
              placeholder="••••••••••••"
              autoComplete="off"
              required
            />
            <p className="mt-1.5 text-xs leading-relaxed text-slate-500">
              Create one at{' '}
              <a
                href={API_TOKEN_HELP}
                target="_blank"
                rel="noreferrer noopener"
                className="text-accent-400 hover:underline"
              >
                id.atlassian.com
              </a>
              . It is encrypted before it is stored, and only this room can use it.
            </p>
          </div>

          {error && <p className="text-sm text-rose-300">{error}</p>}

          <div className="flex justify-end gap-2">
            <button type="button" className="btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn-primary" disabled={busy !== null}>
              {busy === 'token' ? 'Connecting…' : 'Connect'}
            </button>
          </div>
        </form>
      </div>
    </Modal>
  )
}

// ------------------------------------------------------------------- import

interface ImportDialogProps {
  roomCode: string
  token: string
  onClose: () => void
  onImported: (state: RoomState, imported: number, skipped: string[]) => void
  onError: (message: string) => void
}

function ImportDialog({ roomCode, token, onClose, onImported, onError }: ImportDialogProps) {
  const [projects, setProjects] = useState<JiraProject[]>([])
  const [project, setProject] = useState('')
  const [epicQuery, setEpicQuery] = useState('')
  const [epics, setEpics] = useState<JiraIssue[]>([])
  const [epic, setEpic] = useState<JiraIssue | null>(null)
  const [issues, setIssues] = useState<JiraIssue[]>([])
  const [issueFilter, setIssueFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState<'epics' | 'issues' | 'import' | null>(null)
  const [projectsLoading, setProjectsLoading] = useState(true)

  const fail = useCallback(
    (err: unknown, fallback: string) => onError(err instanceof ApiError ? err.message : fallback),
    [onError],
  )

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const { projects: list } = await api.jiraProjects(roomCode, token, '')
        if (!cancelled) setProjects(list)
      } catch (err) {
        if (!cancelled) fail(err, 'Could not load Jira projects.')
      } finally {
        if (!cancelled) setProjectsLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [roomCode, token, fail])

  // Epics are searched server-side with JQL, so the input is debounced to keep
  // the request count down while the host types.
  useEffect(() => {
    let cancelled = false
    setLoading('epics')
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const { epics: list } = await api.jiraEpics(roomCode, token, project, epicQuery.trim())
          if (!cancelled) setEpics(list)
        } catch (err) {
          if (!cancelled) {
            setEpics([])
            fail(err, 'Could not search epics.')
          }
        } finally {
          if (!cancelled) setLoading(null)
        }
      })()
    }, 300)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [roomCode, token, project, epicQuery, fail])

  const openEpic = async (next: JiraIssue) => {
    setEpic(next)
    setIssueFilter('')
    setSelected(new Set())
    setLoading('issues')
    try {
      const { issues: list } = await api.jiraEpicIssues(roomCode, token, next.key)
      setIssues(list)
      setSelected(new Set(list.map((issue) => issue.key)))
    } catch (err) {
      setIssues([])
      fail(err, 'Could not load the issues of that epic.')
    } finally {
      setLoading(null)
    }
  }

  const visibleIssues = useMemo(() => {
    const needle = issueFilter.trim().toLowerCase()
    if (!needle) return issues
    return issues.filter(
      (issue) =>
        issue.key.toLowerCase().includes(needle) ||
        issue.summary.toLowerCase().includes(needle) ||
        issue.status.toLowerCase().includes(needle),
    )
  }, [issues, issueFilter])

  const toggle = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const setAllVisible = (checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev)
      for (const issue of visibleIssues) {
        if (checked) next.add(issue.key)
        else next.delete(issue.key)
      }
      return next
    })
  }

  const runImport = async () => {
    setLoading('import')
    try {
      const { result, state } = await api.jiraImport(roomCode, token, [...selected])
      onImported(state, result.imported, result.skipped)
      onClose()
    } catch (err) {
      fail(err, 'The import failed.')
      setLoading(null)
    }
  }

  return (
    <Modal label="Import from Jira" onClose={onClose}>
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-50">{epic ? `Stories in ${epic.key}` : 'Find an epic'}</h2>
        <button type="button" onClick={onClose} className="text-slate-500 hover:text-slate-200">
          ✕
        </button>
      </div>

      {epic ? (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <button type="button" className="btn-ghost !py-1.5 text-xs" onClick={() => setEpic(null)}>
              ← Back to epics
            </button>
            <input
              className="field !py-1.5 min-w-40 flex-1 text-sm"
              value={issueFilter}
              onChange={(e) => setIssueFilter(e.target.value)}
              placeholder="Filter stories…"
              aria-label="Filter stories"
            />
            <button type="button" className="btn-ghost !py-1.5 text-xs" onClick={() => setAllVisible(true)}>
              Select all
            </button>
            <button type="button" className="btn-ghost !py-1.5 text-xs" onClick={() => setAllVisible(false)}>
              Clear
            </button>
          </div>

          <div className="mt-3 min-h-0 flex-1 overflow-y-auto rounded-xl border border-white/10 bg-black/20 p-2">
            {loading === 'issues' ? (
              <p className="p-6 text-center text-sm text-slate-500">Loading stories…</p>
            ) : visibleIssues.length === 0 ? (
              <p className="p-6 text-center text-sm text-slate-500">
                {issues.length === 0 ? 'This epic has no child issues.' : 'No story matches that filter.'}
              </p>
            ) : (
              <ul className="space-y-1">
                {visibleIssues.map((issue) => (
                  <li key={issue.key}>
                    <label className="flex cursor-pointer items-start gap-3 rounded-lg px-2.5 py-2 hover:bg-white/5">
                      <input
                        type="checkbox"
                        checked={selected.has(issue.key)}
                        onChange={() => toggle(issue.key)}
                        className="mt-0.5 h-4 w-4 rounded border-white/20 bg-black/40 accent-sky-500"
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm text-slate-200">
                          <span className="mr-2 font-semibold text-accent-400">{issue.key}</span>
                          {issue.summary}
                        </span>
                        <span className="mt-0.5 block text-xs text-slate-500">
                          {issue.type} · {issue.status}
                        </span>
                      </span>
                    </label>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="mt-4 flex items-center justify-between gap-3">
            <span className="text-xs text-slate-500">{selected.size} selected</span>
            <div className="flex gap-2">
              <button type="button" className="btn-ghost" onClick={onClose}>
                Cancel
              </button>
              <button
                type="button"
                className="btn-primary"
                onClick={() => void runImport()}
                disabled={selected.size === 0 || loading === 'import'}
              >
                {loading === 'import' ? 'Importing…' : `Import ${selected.size || ''}`}
              </button>
            </div>
          </div>
        </>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-[12rem_minmax(0,1fr)]">
            <div>
              <label className="label" htmlFor="jira-project">
                Project
              </label>
              <select
                id="jira-project"
                className="field"
                value={project}
                onChange={(e) => setProject(e.target.value)}
                disabled={projectsLoading}
              >
                <option value="">{projectsLoading ? 'Loading…' : 'All projects'}</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.key}>
                    {p.key} — {p.name}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="label" htmlFor="jira-epic-search">
                Search epics
              </label>
              <input
                id="jira-epic-search"
                className="field"
                value={epicQuery}
                onChange={(e) => setEpicQuery(e.target.value)}
                placeholder="Epic name or key, e.g. PAY-12"
                autoFocus
              />
            </div>
          </div>

          <div className="mt-4 min-h-0 flex-1 overflow-y-auto rounded-xl border border-white/10 bg-black/20 p-2">
            {loading === 'epics' ? (
              <p className="p-6 text-center text-sm text-slate-500">Searching…</p>
            ) : epics.length === 0 ? (
              <p className="p-6 text-center text-sm text-slate-500">
                No epic matched. Try another name, or pick a different project.
              </p>
            ) : (
              <ul className="space-y-1">
                {epics.map((item) => (
                  <li key={item.key}>
                    <button
                      type="button"
                      onClick={() => void openEpic(item)}
                      className="flex w-full items-start gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-white/5"
                    >
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm text-slate-200">
                          <span className="mr-2 font-semibold text-accent-400">{item.key}</span>
                          {item.summary}
                        </span>
                        <span className="mt-0.5 block text-xs text-slate-500">{item.status}</span>
                      </span>
                      <span className="mt-0.5 shrink-0 text-xs text-slate-600">→</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="mt-4 flex justify-end">
            <button type="button" className="btn-ghost" onClick={onClose}>
              Cancel
            </button>
          </div>
        </>
      )}
    </Modal>
  )
}

// -------------------------------------------------------------------- modal

interface ModalProps {
  label: string
  onClose: () => void
  children: ReactNode
}

function Modal({ label, onClose, children }: ModalProps) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-label={label}
    >
      <div className="panel flex max-h-[85vh] w-full max-w-2xl flex-col bg-surface-900 p-6">{children}</div>
    </div>
  )
}
