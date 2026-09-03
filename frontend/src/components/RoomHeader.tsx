import { useState } from 'react'

import { DeckPicker } from './DeckPicker'
import type { ConnectionStatus } from '../lib/useRoomSocket'
import type { RoomState } from '../lib/types'

const STATUS_LABEL: Record<ConnectionStatus, { text: string; dot: string }> = {
  connecting: { text: 'Connecting', dot: 'bg-amber-400 animate-pulse' },
  open: { text: 'Live', dot: 'bg-emerald-400' },
  reconnecting: { text: 'Reconnecting', dot: 'bg-amber-400 animate-pulse' },
  offline: { text: 'Offline', dot: 'bg-rose-400' },
}

interface RoomHeaderProps {
  state: RoomState
  status: ConnectionStatus
  onUpdateRoom: (input: { name: string; autoReveal: boolean }) => Promise<void>
  onSetDeck: (cards: string[]) => Promise<void>
  onLeave: () => void
}

export function RoomHeader({ state, status, onUpdateRoom, onSetDeck, onLeave }: RoomHeaderProps) {
  const { room, me } = state
  const [copied, setCopied] = useState<'code' | 'link' | null>(null)
  const [editing, setEditing] = useState(false)
  const [draftName, setDraftName] = useState(room.name)
  const [deckOpen, setDeckOpen] = useState(false)
  const [draftDeck, setDraftDeck] = useState(room.deck)
  const [savingDeck, setSavingDeck] = useState(false)

  const copy = async (what: 'code' | 'link') => {
    const value = what === 'code' ? room.code : window.location.href
    try {
      await navigator.clipboard.writeText(value)
      setCopied(what)
      window.setTimeout(() => setCopied(null), 1800)
    } catch {
      // Clipboard access can be denied; the code is visible on screen anyway.
    }
  }

  const save = async () => {
    await onUpdateRoom({ name: draftName, autoReveal: room.autoReveal })
    setEditing(false)
  }

  const openDeck = () => {
    setDraftDeck(room.deck)
    setDeckOpen(true)
  }

  const saveDeck = async () => {
    setSavingDeck(true)
    try {
      await onSetDeck(draftDeck)
      setDeckOpen(false)
    } finally {
      setSavingDeck(false)
    }
  }

  return (
    <header className="panel flex flex-wrap items-center gap-4 p-4">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2.5">
          {editing ? (
            <input
              className="field max-w-xs !py-1.5"
              value={draftName}
              onChange={(e) => setDraftName(e.target.value)}
              onBlur={() => void save()}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void save()
                if (e.key === 'Escape') {
                  setDraftName(room.name)
                  setEditing(false)
                }
              }}
              maxLength={80}
              autoFocus
            />
          ) : (
            <h1
              className={['truncate text-lg font-semibold text-slate-50', me.isHost ? 'cursor-text' : ''].join(' ')}
              onClick={() => me.isHost && setEditing(true)}
              title={me.isHost ? 'Click to rename' : undefined}
            >
              {room.name}
            </h1>
          )}

          <span
            className={[
              'shrink-0 rounded-full px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider',
              room.mode === 'sync' ? 'bg-sync-500/15 text-sync-500' : 'bg-async-500/15 text-async-500',
            ].join(' ')}
          >
            {room.mode === 'sync' ? 'synchronous' : 'asynchronous'}
          </span>
        </div>

        <div className="mt-1.5 flex items-center gap-3 text-xs text-slate-500">
          <span className="inline-flex items-center gap-1.5">
            <span className={`h-1.5 w-1.5 rounded-full ${STATUS_LABEL[status].dot}`} />
            {STATUS_LABEL[status].text}
          </span>
          {me.isHost && (
            <label className="inline-flex cursor-pointer items-center gap-1.5">
              <input
                type="checkbox"
                checked={room.autoReveal}
                onChange={(e) => void onUpdateRoom({ name: room.name, autoReveal: e.target.checked })}
                className="h-3.5 w-3.5 rounded border-white/20 bg-black/40 accent-sky-500"
              />
              auto-reveal
            </label>
          )}
          {me.isHost && (
            <button
              type="button"
              onClick={() => (deckOpen ? setDeckOpen(false) : openDeck())}
              className="underline decoration-dotted underline-offset-4 transition hover:text-slate-300"
            >
              deck
            </button>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => void copy('code')}
          className="rounded-xl border border-white/10 bg-black/25 px-3 py-2 font-mono text-lg font-semibold tracking-[0.25em] text-slate-100 transition hover:border-accent-500/50"
          title="Copy the room code"
        >
          {copied === 'code' ? 'copied' : room.code}
        </button>
        <button type="button" className="btn-ghost" onClick={() => void copy('link')}>
          {copied === 'link' ? 'Copied' : 'Share link'}
        </button>
        <button type="button" className="btn-ghost !px-3" onClick={onLeave} title="Leave this session">
          Leave
        </button>
      </div>

      {me.isHost && deckOpen && (
        <div className="w-full rounded-xl border border-white/10 bg-black/20 p-3.5">
          <DeckPicker deck={draftDeck} onChange={setDraftDeck} disabled={savingDeck} />
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <button
              type="button"
              className="btn-primary !px-3 !py-1.5 !text-sm"
              onClick={() => void saveDeck()}
              disabled={savingDeck || draftDeck.length < 2}
            >
              {savingDeck ? 'Saving…' : 'Use this deck'}
            </button>
            <button type="button" className="btn-ghost !px-3 !py-1.5 !text-sm" onClick={() => setDeckOpen(false)}>
              Cancel
            </button>
            <span className="text-xs text-slate-500">Cards already played stay put; the new deck is for what comes next.</span>
          </div>
        </div>
      )}
    </header>
  )
}
