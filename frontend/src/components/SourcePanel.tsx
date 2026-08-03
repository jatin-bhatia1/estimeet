import { useCallback, useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import type { FormEvent, ReactNode } from 'react'

import { ApiError, api } from '../lib/api'
import type { AppConfig, RoomState, SourceContainer, SourceDescriptor, SourceItem } from '../lib/types'

interface SourcePanelProps {
  state: RoomState
  token: string
  config: AppConfig | null
  onImported: (state: RoomState, imported: number, skipped: string[]) => void
  onError: (message: string) => void
}

/**
 * SourcePanel is the optional backlog source: connect the room to Jira, Azure
 * DevOps or GitHub, find an epic or milestone and pull its work in as topics.
 * Everything after the first click happens in one centered window, which turns
 * from the connect form into the browser as soon as the room is linked. Only
 * the host sees it.
 */
export function SourcePanel({ state, token, config, onImported, onError }: SourcePanelProps) {
  const { room, me } = state
  const [dialogOpen, setDialogOpen] = useState(false)

  const descriptor = useMemo(
    () => config?.sources.find((s) => s.kind === room.source?.provider) ?? null,
    [config, room.source],
  )

  if (!me.isHost) return null

  if (config && config.sources.length === 0) {
    return (
      <div className="panel p-4">
        <h2 className="text-sm font-semibold text-slate-200">Backlog import</h2>
        <p className="mt-1.5 text-xs leading-relaxed text-slate-500">
          No backlog integration is available on this server.
        </p>
      </div>
    )
  }

  const disconnect = async () => {
    try {
      onImported(await api.sourceDisconnect(room.code, token), 0, [])
    } catch (err) {
      onError(err instanceof ApiError ? err.message : 'Could not disconnect.')
    }
  }

  return (
    <div className="panel p-4">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-slate-200">Backlog import</h2>
        {room.source && (
          <button type="button" onClick={() => void disconnect()} className="text-xs text-slate-600 hover:text-rose-300">
            disconnect
          </button>
        )}
      </div>

      {room.source ? (
        <>
          <p className="mb-1 truncate text-xs text-slate-500">
            Connected to <span className="text-slate-300">{room.source.name}</span>
          </p>
          <p className="mb-1 truncate text-[11px] text-slate-600">
            {room.source.authType === 'oauth' ? 'OAuth' : 'Access token'}
            {room.source.account ? ` · ${room.source.account}` : ''}
          </p>
          <p className="mb-3 text-[11px] text-slate-600">
            Credentials are deleted {formatExpiry(room.source.expiresAt)}.
          </p>
          <button type="button" className="btn-ghost w-full" onClick={() => setDialogOpen(true)} disabled={!config}>
            Search and import
          </button>
        </>
      ) : (
        <>
          <p className="mb-3 text-xs leading-relaxed text-slate-500">
            Connect the room to Jira, Azure DevOps or GitHub to pull a backlog straight in.
          </p>
          <button type="button" className="btn-ghost w-full" onClick={() => setDialogOpen(true)} disabled={!config}>
            Connect a backlog
          </button>
        </>
      )}

      {dialogOpen && config && (
        <Modal
          label={room.source ? 'Import from your backlog' : 'Connect a backlog'}
          onClose={() => setDialogOpen(false)}
        >
          {room.source && descriptor ? (
            <ImportPane
              roomCode={room.code}
              token={token}
              descriptor={descriptor}
              onClose={() => setDialogOpen(false)}
              onImported={onImported}
              onError={onError}
            />
          ) : (
            <ConnectPane
              roomCode={room.code}
              token={token}
              config={config}
              onClose={() => setDialogOpen(false)}
              // The window stays open: the new state flips it to the browser.
              onConnected={(next) => onImported(next, 0, [])}
              onError={onError}
            />
          )}
        </Modal>
      )}
    </div>
  )
}

/** formatExpiry renders the retention deadline the way a person would say it. */
function formatExpiry(iso: string): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return 'automatically'
  const hours = Math.round((at.getTime() - Date.now()) / 3_600_000)
  const when = at.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
  return hours > 0 ? `in ${hours}h (${when})` : `at ${when}`
}

// ------------------------------------------------------------------ connect

interface ConnectPaneProps {
  roomCode: string
  token: string
  config: AppConfig
  onClose: () => void
  onConnected: (state: RoomState) => void
  onError: (message: string) => void
}

/**
 * ConnectPane builds its form from the provider descriptors the server sends,
 * so adding a tracker on the backend needs no change here.
 */
function ConnectPane({ roomCode, token, config, onClose, onConnected, onError }: ConnectPaneProps) {
  const [kind, setKind] = useState(config.sources[0]?.kind ?? 'jira')
  const [values, setValues] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState<'oauth' | 'token' | null>(null)
  const [error, setError] = useState<string | null>(null)

  const descriptor = config.sources.find((s) => s.kind === kind) ?? config.sources[0]

  const startOAuth = async () => {
    setBusy('oauth')
    setError(null)
    try {
      const { authorizeUrl } = await api.jiraConnect(roomCode, token)
      window.location.href = authorizeUrl
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not start the connection.')
      setBusy(null)
    }
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy('token')
    setError(null)
    try {
      onConnected(
        await api.sourceConnect(roomCode, token, {
          provider: kind,
          baseUrl: values.baseUrl ?? '',
          account: values.account ?? '',
          token: values.token ?? '',
        }),
      )
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Could not connect.'
      setError(message)
      onError(message)
      setBusy(null)
    }
  }

  if (!descriptor) return null

  return (
    <>
      <ModalHeader title="Connect a backlog" onClose={onClose} />

      <div className="min-h-0 overflow-y-auto">
        <div className="mb-5 flex gap-2" role="tablist" aria-label="Backlog source">
          {config.sources.map((s) => (
            <button
              key={s.kind}
              type="button"
              role="tab"
              aria-selected={s.kind === kind}
              onClick={() => {
                setKind(s.kind)
                setValues({})
                setError(null)
              }}
              className={
                s.kind === kind
                  ? 'flex-1 rounded-lg border border-accent-400/60 bg-accent-400/10 px-3 py-2 text-sm text-slate-100'
                  : 'flex-1 rounded-lg border border-white/10 px-3 py-2 text-sm text-slate-400 hover:bg-white/5'
              }
            >
              {s.name}
            </button>
          ))}
        </div>

        {/* Credentials are borrowed, not kept: say so before they are typed. */}
        <p className="mb-5 rounded-xl border border-amber-400/30 bg-amber-400/5 p-3 text-xs leading-relaxed text-amber-200/90">
          <strong className="font-semibold">Important.</strong> Your token is encrypted, usable only by this room, and
          never shown again. It is deleted automatically {config.credentialTtlHours} hours after you connect, and as
          soon as the session closes — so import what you need at the start of the session, and expect to reconnect for
          a later one.
        </p>

        {kind === 'jira' && config.jiraOauthAvailable && (
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

        <form onSubmit={submit} className="space-y-4">
          {descriptor.fields.map((field) => (
            <div key={field.name}>
              <label className="label" htmlFor={`source-${field.name}`}>
                {field.label}
              </label>
              <input
                id={`source-${field.name}`}
                type={field.type ?? 'text'}
                className="field"
                value={values[field.name] ?? ''}
                onChange={(e) => setValues((prev) => ({ ...prev, [field.name]: e.target.value }))}
                placeholder={field.placeholder}
                autoComplete={field.type === 'password' ? 'off' : 'on'}
                required
              />
              {field.help && (
                <p className="mt-1.5 text-xs leading-relaxed text-slate-500">
                  {field.help}
                  {field.helpUrl && (
                    <>
                      {' '}
                      <a
                        href={field.helpUrl}
                        target="_blank"
                        rel="noreferrer noopener"
                        className="text-accent-400 hover:underline"
                      >
                        Create one here
                      </a>
                      .
                    </>
                  )}
                </p>
              )}
            </div>
          ))}

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
    </>
  )
}

// ------------------------------------------------------------------- import

interface ImportPaneProps {
  roomCode: string
  token: string
  descriptor: SourceDescriptor
  onClose: () => void
  onImported: (state: RoomState, imported: number, skipped: string[]) => void
  onError: (message: string) => void
}

/**
 * ImportPane walks the three levels of any tracker: container, group, items.
 * The first two levels are searched rather than scrolled, because a real
 * organisation has far more projects and repositories than fit in a list.
 */
function ImportPane({ roomCode, token, descriptor, onClose, onImported, onError }: ImportPaneProps) {
  const [container, setContainer] = useState<SourceContainer | null>(null)
  const [containerQuery, setContainerQuery] = useState('')
  const [containers, setContainers] = useState<SourceContainer[]>([])

  const [groupQuery, setGroupQuery] = useState('')
  const [groups, setGroups] = useState<SourceItem[]>([])
  const [group, setGroup] = useState<SourceItem | null>(null)

  const [items, setItems] = useState<SourceItem[]>([])
  const [itemFilter, setItemFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState<'containers' | 'groups' | 'items' | 'import' | null>('containers')

  const fail = useCallback(
    (err: unknown, fallback: string) => onError(err instanceof ApiError ? err.message : fallback),
    [onError],
  )

  const containerWord = descriptor.container.toLowerCase()
  const groupWord = descriptor.group.toLowerCase()

  // The searches run on the server so something far down the list can still be
  // found by name; the debounce keeps a typed word from becoming ten requests.
  useEffect(() => {
    if (container) return
    const controller = new AbortController()
    setLoading('containers')
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const { containers: list } = await api.sourceContainers(
            roomCode,
            token,
            containerQuery.trim(),
            controller.signal,
          )
          setContainers(list)
        } catch (err) {
          if (controller.signal.aborted) return
          setContainers([])
          fail(err, `Could not load ${containerWord}s.`)
        } finally {
          if (!controller.signal.aborted) setLoading(null)
        }
      })()
    }, 300)
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [roomCode, token, containerQuery, container, containerWord, fail])

  useEffect(() => {
    if (!container || group) return
    const controller = new AbortController()
    setLoading('groups')
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const { groups: list } = await api.sourceGroups(
            roomCode,
            token,
            container.key,
            groupQuery.trim(),
            controller.signal,
          )
          setGroups(list)
        } catch (err) {
          if (controller.signal.aborted) return
          setGroups([])
          fail(err, `Could not search ${groupWord}s.`)
        } finally {
          if (!controller.signal.aborted) setLoading(null)
        }
      })()
    }, 300)
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [roomCode, token, container, group, groupQuery, groupWord, fail])

  const openGroup = async (next: SourceItem) => {
    if (!container) return
    setGroup(next)
    setItemFilter('')
    setSelected(new Set())
    setLoading('items')
    try {
      const { items: list } = await api.sourceItems(roomCode, token, container.key, next.key)
      setItems(list)
      setSelected(new Set(list.map((item) => item.key)))
    } catch (err) {
      setItems([])
      fail(err, `Could not load the ${descriptor.items} of that ${groupWord}.`)
    } finally {
      setLoading(null)
    }
  }

  const visibleItems = useMemo(() => {
    const needle = itemFilter.trim().toLowerCase()
    if (!needle) return items
    return items.filter((item) =>
      [item.key, item.title, item.status ?? ''].some((field) => field.toLowerCase().includes(needle)),
    )
  }, [items, itemFilter])

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
      for (const item of visibleItems) {
        if (checked) next.add(item.key)
        else next.delete(item.key)
      }
      return next
    })
  }

  const runImport = async () => {
    if (!container || !group) return
    setLoading('import')
    try {
      const { result, state } = await api.sourceImport(roomCode, token, container.key, group.key, [...selected])
      onImported(state, result.imported, result.skipped)
      onClose()
    } catch (err) {
      fail(err, 'The import failed.')
      setLoading(null)
    }
  }

  if (!container) {
    return (
      <>
        <ModalHeader title={`Choose ${article(containerWord)} ${containerWord}`} onClose={onClose} />
        <input
          className="field"
          value={containerQuery}
          onChange={(e) => setContainerQuery(e.target.value)}
          placeholder={`Search ${containerWord}s…`}
          aria-label={`Search ${containerWord}s`}
          autoFocus
        />
        <PickList
          loading={loading === 'containers'}
          empty={`No ${containerWord} matched. Try another name.`}
          rows={containers.map((c) => ({
            key: c.key,
            title: c.name,
            badge: c.key === c.name ? undefined : c.key,
            onPick: () => {
              setContainer(c)
              setGroupQuery('')
              setGroups([])
            },
          }))}
        />
        <div className="mt-4 flex justify-end">
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
        </div>
      </>
    )
  }

  if (!group) {
    return (
      <>
        <ModalHeader title={`Find ${article(groupWord)} ${groupWord}`} onClose={onClose} />
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" className="btn-ghost !py-1.5 text-xs" onClick={() => setContainer(null)}>
            ← {container.name}
          </button>
          <input
            className="field !py-1.5 min-w-40 flex-1 text-sm"
            value={groupQuery}
            onChange={(e) => setGroupQuery(e.target.value)}
            placeholder={`Search ${groupWord}s…`}
            aria-label={`Search ${groupWord}s`}
            autoFocus
          />
        </div>
        <PickList
          loading={loading === 'groups'}
          empty={`No ${groupWord} matched. Try another name.`}
          rows={groups.map((g) => ({
            key: g.key,
            title: g.title,
            badge: g.key,
            subtitle: g.status,
            onPick: () => void openGroup(g),
          }))}
        />
        <div className="mt-4 flex justify-end">
          <button type="button" className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
        </div>
      </>
    )
  }

  return (
    <>
      <ModalHeader title={`${capitalize(descriptor.items)} in ${group.key}`} onClose={onClose} />
      <div className="flex flex-wrap items-center gap-2">
        <button type="button" className="btn-ghost !py-1.5 text-xs" onClick={() => setGroup(null)}>
          ← Back
        </button>
        <input
          className="field !py-1.5 min-w-40 flex-1 text-sm"
          value={itemFilter}
          onChange={(e) => setItemFilter(e.target.value)}
          placeholder={`Filter ${descriptor.items}…`}
          aria-label={`Filter ${descriptor.items}`}
        />
        <button type="button" className="btn-ghost !py-1.5 text-xs" onClick={() => setAllVisible(true)}>
          Select all
        </button>
        <button type="button" className="btn-ghost !py-1.5 text-xs" onClick={() => setAllVisible(false)}>
          Clear
        </button>
      </div>

      <div className="mt-3 min-h-0 flex-1 overflow-y-auto rounded-xl border border-white/10 bg-black/20 p-2">
        {loading === 'items' ? (
          <p className="p-6 text-center text-sm text-slate-500">Loading…</p>
        ) : visibleItems.length === 0 ? (
          <p className="p-6 text-center text-sm text-slate-500">
            {items.length === 0 ? `This ${groupWord} is empty.` : 'Nothing matches that filter.'}
          </p>
        ) : (
          <ul className="space-y-1">
            {visibleItems.map((item) => (
              <li key={item.key}>
                <label className="flex cursor-pointer items-start gap-3 rounded-lg px-2.5 py-2 hover:bg-white/5">
                  <input
                    type="checkbox"
                    checked={selected.has(item.key)}
                    onChange={() => toggle(item.key)}
                    className="mt-0.5 h-4 w-4 rounded border-white/20 bg-black/40 accent-sky-500"
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm text-slate-200">
                      <span className="mr-2 font-semibold text-accent-400">{item.key}</span>
                      {item.title}
                    </span>
                    <span className="mt-0.5 block text-xs text-slate-500">
                      {[item.type, item.status].filter(Boolean).join(' · ')}
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
  )
}

interface PickRow {
  key: string
  title: string
  badge?: string
  subtitle?: string
  onPick: () => void
}

/** PickList is the scrollable result list shared by the two search steps. */
function PickList({ loading, empty, rows }: { loading: boolean; empty: string; rows: PickRow[] }) {
  return (
    <div className="mt-4 min-h-0 flex-1 overflow-y-auto rounded-xl border border-white/10 bg-black/20 p-2">
      {loading ? (
        <p className="p-6 text-center text-sm text-slate-500">Searching…</p>
      ) : rows.length === 0 ? (
        <p className="p-6 text-center text-sm text-slate-500">{empty}</p>
      ) : (
        <ul className="space-y-1">
          {rows.map((row) => (
            <li key={row.key}>
              <button
                type="button"
                onClick={row.onPick}
                className="flex w-full items-start gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-white/5"
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm text-slate-200">
                    {row.badge && <span className="mr-2 font-semibold text-accent-400">{row.badge}</span>}
                    {row.title}
                  </span>
                  {row.subtitle && <span className="mt-0.5 block text-xs text-slate-500">{row.subtitle}</span>}
                </span>
                <span className="mt-0.5 shrink-0 text-xs text-slate-600">→</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

const capitalize = (word: string) => word.charAt(0).toUpperCase() + word.slice(1)
const article = (word: string) => ('aeiou'.includes(word.charAt(0).toLowerCase()) ? 'an' : 'a')

// -------------------------------------------------------------------- modal

interface ModalProps {
  label: string
  onClose: () => void
  children: ReactNode
}

/**
 * Modal centers its content on the viewport. It renders into document.body on
 * purpose: the surrounding panels use backdrop-blur, which makes them the
 * containing block for fixed positioning, and the window would otherwise be
 * anchored to, and clipped by, the sidebar it was opened from.
 */
function Modal({ label, onClose, children }: ModalProps) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    const { overflow } = document.body.style
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = overflow
    }
  }, [onClose])

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-label={label}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="panel flex max-h-[85vh] w-full max-w-2xl flex-col bg-surface-900 p-6">{children}</div>
    </div>,
    document.body,
  )
}

function ModalHeader({ title, onClose }: { title: string; onClose: () => void }) {
  return (
    <div className="mb-4 flex items-center justify-between gap-4">
      <h2 className="truncate text-lg font-semibold text-slate-50">{title}</h2>
      <button
        type="button"
        onClick={onClose}
        aria-label="Close"
        className="shrink-0 text-slate-500 hover:text-slate-200"
      >
        ✕
      </button>
    </div>
  )
}
