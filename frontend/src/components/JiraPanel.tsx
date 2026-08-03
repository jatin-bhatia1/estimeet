import { useCallback, useEffect, useState } from 'react'

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
 * site, pick an epic and pull its stories in as topics.
 */
export function JiraPanel({ state, token, onImported, onError }: JiraPanelProps) {
  const { room, me } = state
  const [open, setOpen] = useState(false)
  const [connecting, setConnecting] = useState(false)

  if (!me.isHost) return null

  if (!room.jiraAvailable) {
    return (
      <div className="panel p-4">
        <h2 className="text-sm font-semibold text-slate-200">Jira</h2>
        <p className="mt-1.5 text-xs leading-relaxed text-slate-500">
          Not configured on this server. Set <code className="text-slate-400">JIRA_CLIENT_ID</code>,{' '}
          <code className="text-slate-400">JIRA_CLIENT_SECRET</code> and{' '}
          <code className="text-slate-400">JIRA_REDIRECT_URI</code> to enable epic imports.
        </p>
      </div>
    )
  }

  const connect = async () => {
    setConnecting(true)
    try {
      const { authorizeUrl } = await api.jiraConnect(room.code, token)
      window.location.href = authorizeUrl
    } catch (err) {
      onError(err instanceof ApiError ? err.message : 'Could not start the Jira connection.')
      setConnecting(false)
    }
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
          <p className="mb-3 truncate text-xs text-slate-500" title={room.jiraSiteUrl}>
            Connected to <span className="text-slate-300">{room.jiraSiteName || room.jiraSiteUrl}</span>
          </p>
          <button type="button" className="btn-ghost w-full" onClick={() => setOpen(true)}>
            Import from an epic
          </button>
        </>
      ) : (
        <>
          <p className="mb-3 text-xs leading-relaxed text-slate-500">
            Connect the room to your Jira site to pull the stories of an epic straight into the backlog.
          </p>
          <button type="button" className="btn-ghost w-full" onClick={() => void connect()} disabled={connecting}>
            {connecting ? 'Redirecting…' : 'Connect to Jira'}
          </button>
        </>
      )}

      {open && (
        <ImportDialog
          roomCode={room.code}
          token={token}
          onClose={() => setOpen(false)}
          onImported={onImported}
          onError={onError}
        />
      )}
    </div>
  )
}

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
  const [epics, setEpics] = useState<JiraIssue[]>([])
  const [epic, setEpic] = useState('')
  const [issues, setIssues] = useState<JiraIssue[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState<'projects' | 'epics' | 'issues' | 'import' | null>('projects')

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
        if (!cancelled) setLoading(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [roomCode, token, fail])

  const loadEpics = async (projectKey: string) => {
    setProject(projectKey)
    setEpic('')
    setIssues([])
    setSelected(new Set())
    if (!projectKey) {
      setEpics([])
      return
    }
    setLoading('epics')
    try {
      const { epics: list } = await api.jiraEpics(roomCode, token, projectKey, '')
      setEpics(list)
    } catch (err) {
      fail(err, 'Could not load epics.')
    } finally {
      setLoading(null)
    }
  }

  const loadIssues = async (epicKey: string) => {
    setEpic(epicKey)
    setSelected(new Set())
    if (!epicKey) {
      setIssues([])
      return
    }
    setLoading('issues')
    try {
      const { issues: list } = await api.jiraEpicIssues(roomCode, token, epicKey)
      setIssues(list)
      setSelected(new Set(list.map((issue) => issue.key)))
    } catch (err) {
      fail(err, 'Could not load the issues of that epic.')
    } finally {
      setLoading(null)
    }
  }

  const toggle = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
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
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-label="Import from Jira"
    >
      <div className="panel flex max-h-[85vh] w-full max-w-2xl flex-col bg-surface-900 p-6">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-slate-50">Import from a Jira epic</h2>
          <button type="button" onClick={onClose} className="text-slate-500 hover:text-slate-200">
            ✕
          </button>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label className="label" htmlFor="jira-project">
              Project
            </label>
            <select
              id="jira-project"
              className="field"
              value={project}
              onChange={(e) => void loadEpics(e.target.value)}
              disabled={loading === 'projects'}
            >
              <option value="">{loading === 'projects' ? 'Loading…' : 'Select a project'}</option>
              {projects.map((p) => (
                <option key={p.id} value={p.key}>
                  {p.key} — {p.name}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="label" htmlFor="jira-epic">
              Epic
            </label>
            <select
              id="jira-epic"
              className="field"
              value={epic}
              onChange={(e) => void loadIssues(e.target.value)}
              disabled={!project || loading === 'epics'}
            >
              <option value="">{loading === 'epics' ? 'Loading…' : 'Select an epic'}</option>
              {epics.map((e) => (
                <option key={e.key} value={e.key}>
                  {e.key} — {e.summary}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="mt-4 min-h-0 flex-1 overflow-y-auto rounded-xl border border-white/10 bg-black/20 p-2">
          {loading === 'issues' ? (
            <p className="p-6 text-center text-sm text-slate-500">Loading issues…</p>
          ) : issues.length === 0 ? (
            <p className="p-6 text-center text-sm text-slate-500">
              {epic ? 'This epic has no child issues.' : 'Pick an epic to see its stories.'}
            </p>
          ) : (
            <ul className="space-y-1">
              {issues.map((issue) => (
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
      </div>
    </div>
  )
}
