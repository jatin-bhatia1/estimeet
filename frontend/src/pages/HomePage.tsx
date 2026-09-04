import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { DEFAULT_DECK, DeckPicker } from '../components/DeckPicker'
import { ApiError, api } from '../lib/api'
import { recallName, rememberName, saveSession } from '../lib/session'
import { cardLabel, type Mode, type SessionResponse } from '../lib/types'

const MODE_COPY: Record<Mode, { title: string; blurb: string; accent: string }> = {
  sync: {
    title: 'Synchronous',
    blurb: 'The whole team is in the call. The host walks the backlog one topic at a time and everybody plays a card on the same story before the reveal.',
    accent: 'text-sync-500',
  },
  async: {
    title: 'Asynchronous',
    blurb: 'Everything is open at once. Each teammate works through the backlog whenever they have time; a topic flips as soon as the last person has voted.',
    accent: 'text-async-500',
  },
}

export default function HomePage() {
  const navigate = useNavigate()
  const [tab, setTab] = useState<'create' | 'join'>('create')

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-5xl flex-col px-5 py-10">
      <header className="mb-10">
        <p className="mb-2 text-xs font-semibold uppercase tracking-[0.3em] text-accent-500">Estimeet</p>
        <h1 className="text-4xl font-semibold tracking-tight text-slate-50 sm:text-5xl">
          Estimate together, in the room or across time zones.
        </h1>
        <p className="mt-4 max-w-2xl text-slate-400">
          <span className="block text-slate-200">A room in two clicks, an estimate in two minutes.</span>
          <span className="mt-2 block">
            No accounts, no setup. Share a room, everyone plays a card, and the numbers you disagreed on
            are the only thing left to discuss.
          </span>
          <span className="mt-2 block">
            Import from Jira, Azure DevOps or GitHub, or write the list yourself.
          </span>
        </p>
      </header>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <section className="panel p-6">
          <div className="mb-6 inline-flex rounded-xl border border-white/10 bg-black/20 p-1">
            <button
              type="button"
              onClick={() => setTab('create')}
              className={tabClass(tab === 'create')}
            >
              Start a session
            </button>
            <button type="button" onClick={() => setTab('join')} className={tabClass(tab === 'join')}>
              Join a session
            </button>
          </div>

          {tab === 'create' ? <CreateForm navigate={navigate} /> : <JoinForm navigate={navigate} />}
        </section>

        <aside className="space-y-4">
          {(Object.keys(MODE_COPY) as Mode[]).map((mode) => (
            <div key={mode} className="panel p-5">
              <h2 className={`text-sm font-semibold uppercase tracking-wider ${MODE_COPY[mode].accent}`}>
                {MODE_COPY[mode].title}
              </h2>
              <p className="mt-2 text-sm leading-relaxed text-slate-400">{MODE_COPY[mode].blurb}</p>
            </div>
          ))}
          <div className="panel p-5">
            <h2 className="text-sm font-semibold uppercase tracking-wider text-slate-300">The deck</h2>
            <div className="mt-3 flex flex-wrap gap-1.5">
              {DEFAULT_DECK.map((card) => (
                <span
                  key={card}
                  className="flex h-9 w-7 items-center justify-center rounded-md border border-white/10 bg-white/5 text-xs font-semibold text-slate-300"
                >
                  {cardLabel(card)}
                </span>
              ))}
            </div>
            <p className="mt-3 text-sm leading-relaxed text-slate-400">
              Fibonacci unless you say otherwise. Pick T-shirt sizes, powers of two or your own cards when
              you start the session, or change them later from the room.
            </p>
          </div>
        </aside>
      </div>
    </div>
  )
}

function tabClass(active: boolean): string {
  return [
    'rounded-lg px-4 py-2 text-sm font-medium transition',
    active ? 'bg-accent-500 text-slate-950' : 'text-slate-400 hover:text-slate-200',
  ].join(' ')
}

type Navigate = ReturnType<typeof useNavigate>

function persistAndGo(navigate: Navigate, session: SessionResponse) {
  saveSession(session.roomCode, {
    token: session.token,
    participantId: session.participant.id,
    name: session.participant.name,
  })
  rememberName(session.participant.name)
  navigate(`/room/${session.roomCode}`)
}

function CreateForm({ navigate }: { navigate: Navigate }) {
  const [name, setName] = useState('')
  const [hostName, setHostName] = useState(recallName)
  const [mode, setMode] = useState<Mode>('sync')
  const [autoReveal, setAutoReveal] = useState(true)
  const [deck, setDeck] = useState<string[]>(DEFAULT_DECK)
  const [expected, setExpected] = useState('')
  const [expectedNames, setExpectedNames] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const names = expectedNames
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const session = await api.createRoom({
        name,
        mode,
        hostName,
        autoReveal,
        expectedSize: Number(expected) || names.length,
        expectedNames: names,
        deck,
      })
      persistAndGo(navigate, session)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not create the session.')
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="space-y-5">
      <div>
        <label className="label" htmlFor="session-name">
          Session name
        </label>
        <input
          id="session-name"
          className="field"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Sprint 42 refinement"
          maxLength={80}
        />
      </div>

      <div>
        <label className="label" htmlFor="host-name">
          Your display name
        </label>
        <input
          id="host-name"
          className="field"
          value={hostName}
          onChange={(e) => setHostName(e.target.value)}
          placeholder="Ada"
          maxLength={40}
          required
        />
      </div>
      <p className="-mt-3 text-xs text-slate-500">
        You create the session, so you are its host: only you can reveal the cards.
      </p>

      <fieldset>
        <legend className="label">Mode</legend>
        <div className="grid gap-3 sm:grid-cols-2">
          {(Object.keys(MODE_COPY) as Mode[]).map((value) => (
            <button
              key={value}
              type="button"
              onClick={() => setMode(value)}
              className={[
                'rounded-xl border p-4 text-left transition',
                mode === value
                  ? 'border-accent-500/60 bg-accent-500/10'
                  : 'border-white/10 bg-black/20 hover:border-white/20',
              ].join(' ')}
            >
              <span className="block text-sm font-semibold text-slate-100">{MODE_COPY[value].title}</span>
              <span className="mt-1 block text-xs leading-relaxed text-slate-400">
                {value === 'sync' ? 'One topic at a time, host-led.' : 'All topics open, vote any time.'}
              </span>
            </button>
          ))}
        </div>
      </fieldset>

      <label className="flex items-start gap-3 rounded-xl border border-white/10 bg-black/20 p-3.5">
        <input
          type="checkbox"
          checked={autoReveal}
          onChange={(e) => setAutoReveal(e.target.checked)}
          className="mt-0.5 h-4 w-4 rounded border-white/20 bg-black/40 accent-sky-500"
        />
        <span className="text-sm text-slate-300">
          Flip the cards automatically
          <span className="mt-0.5 block text-xs text-slate-500">
            {mode === 'sync'
              ? 'Reveals once every connected player has voted.'
              : 'Reveals once every member of the room has voted.'}
          </span>
        </span>
      </label>

      <fieldset className="rounded-xl border border-white/10 bg-black/20 p-3.5">
        <legend className="px-1 text-sm text-slate-300">The deck</legend>
        <DeckPicker deck={deck} onChange={setDeck} />
      </fieldset>

      {/* The roster is optional and only ever used to show who is still
          missing, so it sits in its own quiet block below the real settings. */}      <fieldset className="rounded-xl border border-white/10 bg-black/20 p-3.5">
        <legend className="px-1 text-sm text-slate-300">
          Who are you expecting? <span className="text-xs text-slate-500">(optional)</span>
        </legend>
        <div className="grid gap-3 sm:grid-cols-[7rem_minmax(0,1fr)]">
          <label className="block">
            <span className="label !mb-1 !text-[11px]">People</span>
            <input
              type="number"
              min={0}
              max={100}
              className="field"
              value={expected}
              onChange={(e) => setExpected(e.target.value)}
              placeholder="6"
            />
          </label>
          <label className="block">
            <span className="label !mb-1 !text-[11px]">Names, one per line</span>
            <textarea
              rows={3}
              className="field resize-y"
              value={expectedNames}
              onChange={(e) => setExpectedNames(e.target.value)}
              placeholder={'Ada\nJay'}
            />
          </label>
        </div>
        <p className="mt-2 text-xs text-slate-500">
          The room will show who has joined and who is still missing. Nobody is kept out by this — you
          can change it later from the team panel.
        </p>
      </fieldset>

      {error && <p className="text-sm text-rose-300">{error}</p>}

      <button type="submit" className="btn-primary w-full" disabled={busy || deck.length < 2}>
        {busy ? 'Creating…' : 'Create session'}
      </button>
    </form>
  )
}

function JoinForm({ navigate }: { navigate: Navigate }) {
  const [code, setCode] = useState('')
  const [name, setName] = useState(recallName)
  const [asObserver, setAsObserver] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const session = await api.joinRoom(code.trim().toUpperCase(), { name, asObserver })
      persistAndGo(navigate, session)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not join the session.')
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="space-y-5">
      <div>
        <label className="label" htmlFor="room-code">
          Room code
        </label>
        <input
          id="room-code"
          className="field text-center text-2xl font-semibold uppercase tracking-[0.4em]"
          value={code}
          onChange={(e) => setCode(e.target.value.toUpperCase())}
          placeholder="AB3D7K"
          maxLength={6}
          required
        />
      </div>

      <div>
        <label className="label" htmlFor="player-name">
          Your display name
        </label>
        <input
          id="player-name"
          className="field"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Grace"
          maxLength={40}
          required
        />
      </div>

      <label className="flex items-start gap-3 rounded-xl border border-white/10 bg-black/20 p-3.5">
        <input
          type="checkbox"
          checked={asObserver}
          onChange={(e) => setAsObserver(e.target.checked)}
          className="mt-0.5 h-4 w-4 rounded border-white/20 bg-black/40 accent-sky-500"
        />
        <span className="text-sm text-slate-300">
          Join as an observer
          <span className="mt-0.5 block text-xs text-slate-500">
            Watch the session without playing cards or holding up the reveal.
          </span>
        </span>
      </label>

      {error && <p className="text-sm text-rose-300">{error}</p>}

      <button type="submit" className="btn-primary w-full" disabled={busy}>
        {busy ? 'Joining…' : 'Join session'}
      </button>
    </form>
  )
}
