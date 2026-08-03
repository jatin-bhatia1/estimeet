import { type FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'

import { AsyncBoard } from '../components/AsyncBoard'
import { BacklogPanel } from '../components/BacklogPanel'
import { ParticipantsPanel } from '../components/ParticipantsPanel'
import { RoomHeader } from '../components/RoomHeader'
import { SourcePanel } from '../components/SourcePanel'
import { SyncBoard } from '../components/SyncBoard'
import { TopicComposer } from '../components/TopicComposer'
import { ApiError, api } from '../lib/api'
import type { RoomActions, TopicDraft } from '../lib/actions'
import { clearSession, loadSession, recallName, rememberName, saveSession } from '../lib/session'
import type { RoomState, RoomSummary } from '../lib/types'
import { useAppConfig } from '../lib/useAppConfig'
import { useRoomSocket } from '../lib/useRoomSocket'

export default function RoomPage() {
  const { code = '' } = useParams()
  const roomCode = code.toUpperCase()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const [token, setToken] = useState<string | null>(() => loadSession(roomCode)?.token ?? null)
  const [notice, setNotice] = useState<{ kind: 'info' | 'error'; text: string } | null>(null)

  const { state: liveState, status, applyState } = useRoomSocket(roomCode, token)
  const [fallbackState, setFallbackState] = useState<RoomState | null>(null)
  const state = liveState ?? fallbackState
  const config = useAppConfig()

  const notify = useCallback((text: string, kind: 'info' | 'error' = 'error') => {
    setNotice({ kind, text })
    window.setTimeout(() => setNotice(null), 6000)
  }, [])

  // Surface the outcome of the Jira OAuth redirect, then clean the URL.
  useEffect(() => {
    const jira = searchParams.get('jira')
    if (!jira) return
    if (jira === 'connected') notify('Jira connected.', 'info')
    else notify(searchParams.get('message') ?? 'Could not connect to Jira.')
    searchParams.delete('jira')
    searchParams.delete('message')
    setSearchParams(searchParams, { replace: true })
  }, [searchParams, setSearchParams, notify])

  // One REST fetch so the board is usable even before the socket opens.
  useEffect(() => {
    if (!token || liveState) return
    const controller = new AbortController()
    void (async () => {
      try {
        setFallbackState(await api.state(roomCode, token, controller.signal))
      } catch (err) {
        if (controller.signal.aborted) return
        if (err instanceof ApiError && (err.status === 403 || err.status === 404)) {
          clearSession(roomCode)
          setToken(null)
        }
      }
    })()
    return () => controller.abort()
  }, [roomCode, token, liveState])

  const run = useCallback(
    async (operation: () => Promise<RoomState>) => {
      try {
        applyState(await operation())
      } catch (err) {
        notify(err instanceof ApiError ? err.message : 'That did not work.')
      }
    },
    [applyState, notify],
  )

  const actions = useMemo<RoomActions>(() => {
    const t = token ?? ''
    return {
      vote: (topicId, value) => run(() => api.vote(roomCode, t, topicId, value)),
      clearVote: (topicId) => run(() => api.clearVote(roomCode, t, topicId)),
      reveal: (topicId) => run(() => api.reveal(roomCode, t, topicId)),
      reset: (topicId) => run(() => api.resetTopic(roomCode, t, topicId)),
      estimate: (topicId, value) => run(() => api.estimate(roomCode, t, topicId, value)),
      focusTopic: (topicId) => run(() => api.setCurrent(roomCode, t, { topicId })),
      advance: (direction) => run(() => api.setCurrent(roomCode, t, { direction })),
      addTopics: (topics: TopicDraft[]) => run(() => api.addTopics(roomCode, t, topics)),
      updateTopic: (topicId, draft) => run(() => api.updateTopic(roomCode, t, topicId, draft)),
      deleteTopic: (topicId) => run(() => api.deleteTopic(roomCode, t, topicId)),
      kick: (participantId) => run(() => api.kick(roomCode, t, participantId)),
      updateRoom: (input) => run(() => api.updateRoom(roomCode, t, input)),
      setRoster: (input) => run(() => api.setRoster(roomCode, t, input)),
      applyState,
    }
  }, [roomCode, token, run, applyState])

  const leave = () => {
    clearSession(roomCode)
    setToken(null)
    navigate('/')
  }

  if (!token) {
    return (
      <JoinGate
        roomCode={roomCode}
        onJoined={(newToken) => {
          setToken(newToken)
          setFallbackState(null)
        }}
      />
    )
  }

  if (!state) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-sm text-slate-500">Loading the table…</p>
      </div>
    )
  }

  const isSync = state.room.mode === 'sync'

  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-6">
      {notice && (
        <div
          role="status"
          className={[
            'mb-4 rounded-xl border px-4 py-3 text-sm',
            notice.kind === 'error'
              ? 'border-rose-500/30 bg-rose-500/10 text-rose-200'
              : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200',
          ].join(' ')}
        >
          {notice.text}
        </div>
      )}

      <RoomHeader state={state} status={status} onUpdateRoom={actions.updateRoom} onLeave={leave} />

      <div className="mt-5 grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <main className="min-w-0">
          {isSync ? <SyncBoard state={state} actions={actions} /> : <AsyncBoard state={state} actions={actions} />}
        </main>

        <aside className="space-y-4">
          <ParticipantsPanel
            state={state}
            onKick={(id) => void actions.kick(id)}
            onSetRoster={(input) => void actions.setRoster(input)}
          />

          {isSync && (
            <BacklogPanel
              state={state}
              onFocus={(id) => void actions.focusTopic(id)}
              onDelete={(id) => void actions.deleteTopic(id)}
            />
          )}

          {state.me.isHost && (
            <>
              <SourcePanel
                state={state}
                token={token}
                config={config}
                onImported={(next, imported, skipped) => {
                  applyState(next)
                  if (imported > 0 || skipped.length > 0) {
                    notify(
                      `Imported ${imported} topic${imported === 1 ? '' : 's'}` +
                        (skipped.length ? ` · ${skipped.length} already in the backlog` : ''),
                      'info',
                    )
                  }
                }}
                onError={notify}
              />
              <TopicComposer onAdd={actions.addTopics} />
            </>
          )}
        </aside>
      </div>
    </div>
  )
}

interface JoinGateProps {
  roomCode: string
  onJoined: (token: string) => void
}

/** JoinGate is shown when the browser has no token for this room yet. */
function JoinGate({ roomCode, onJoined }: JoinGateProps) {
  const [summary, setSummary] = useState<RoomSummary | null>(null)
  const [name, setName] = useState(recallName)
  const [asObserver, setAsObserver] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    void (async () => {
      try {
        setSummary(await api.roomSummary(roomCode, controller.signal))
      } catch (err) {
        if (!controller.signal.aborted) {
          setError(err instanceof ApiError ? err.message : 'That room could not be found.')
        }
      }
    })()
    return () => controller.abort()
  }, [roomCode])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const session = await api.joinRoom(roomCode, { name, asObserver })
      saveSession(session.roomCode, {
        token: session.token,
        participantId: session.participant.id,
        name: session.participant.name,
      })
      rememberName(session.participant.name)
      onJoined(session.token)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not join the session.')
      setBusy(false)
    }
  }

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-md flex-col justify-center px-5 py-10">
      <div className="panel p-6">
        <p className="text-xs uppercase tracking-[0.3em] text-accent-500">Estimeet</p>
        <h1 className="mt-2 text-2xl font-semibold text-slate-50">{summary?.name ?? roomCode}</h1>
        {summary && (
          <p className="mt-1 text-sm text-slate-500">
            {summary.mode === 'sync' ? 'Synchronous' : 'Asynchronous'} session · {summary.participants} player
            {summary.participants === 1 ? '' : 's'} · {summary.topics} topic{summary.topics === 1 ? '' : 's'}
          </p>
        )}

        <form onSubmit={submit} className="mt-6 space-y-4">
          <div>
            <label className="label" htmlFor="join-name">
              Your display name
            </label>
            <input
              id="join-name"
              className="field"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={40}
              required
              autoFocus
            />
          </div>

          <label className="flex items-start gap-3 rounded-xl border border-white/10 bg-black/20 p-3.5">
            <input
              type="checkbox"
              checked={asObserver}
              onChange={(e) => setAsObserver(e.target.checked)}
              className="mt-0.5 h-4 w-4 rounded border-white/20 bg-black/40 accent-sky-500"
            />
            <span className="text-sm text-slate-300">Join as an observer</span>
          </label>

          {error && <p className="text-sm text-rose-300">{error}</p>}

          <button type="submit" className="btn-primary w-full" disabled={busy || !summary}>
            {busy ? 'Joining…' : 'Join session'}
          </button>
        </form>
      </div>
    </div>
  )
}
