import { useEffect, useState } from 'react'

import type { ParticipantView, RoomState } from '../lib/types'

interface ParticipantsPanelProps {
  state: RoomState
  onKick: (participantId: string) => void
  onSetRoster: (input: { size: number; names: string[] }) => void
}

/**
 * ParticipantsPanel doubles as the presence list and, in asynchronous rooms,
 * the progress board: it shows how far each teammate has worked through the
 * backlog. When the host has declared a roster it also shows the other half of
 * the picture — who was expected and has not turned up.
 */
export function ParticipantsPanel({ state, onKick, onSetRoster }: ParticipantsPanelProps) {
  const { room, me, participants, topics } = state
  const isAsync = room.mode === 'async'
  const currentTopic = topics.find((t) => t.id === room.currentTopicId)
  const votable = topics.length
  const [editing, setEditing] = useState(false)

  const players = participants.filter((p) => !p.isObserver)
  const here = new Set(participants.map((p) => p.name.trim().toLowerCase()))
  const missingNames = room.expectedNames.filter((name) => !here.has(name.trim().toLowerCase()))
  // Seats nobody has claimed and nobody was named for.
  const unnamedSeats = Math.max(0, room.expectedSize - players.length - missingNames.length)
  const rosterSet = room.expectedSize > 0 || room.expectedNames.length > 0

  return (
    <div className="panel p-4">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-slate-200">
          Team{' '}
          <span className="text-slate-500">
            ({participants.length}
            {rosterSet && room.expectedSize > 0 ? ` of ${room.expectedSize}` : ''})
          </span>
        </h2>
        <div className="flex items-center gap-2">
          <span className="text-xs text-slate-500">{participants.filter((p) => p.online).length} online</span>
          {me.isHost && (
            <button
              type="button"
              onClick={() => setEditing((v) => !v)}
              className="rounded-full border border-white/10 px-2 py-0.5 text-[11px] text-slate-400 transition hover:border-white/25 hover:text-slate-200"
              title="Who is expected in this session?"
            >
              {rosterSet ? 'edit expected' : '+ who is expected'}
            </button>
          )}
        </div>
      </div>

      {editing && (
        <RosterEditor
          size={room.expectedSize}
          names={room.expectedNames}
          onCancel={() => setEditing(false)}
          onSave={(input) => {
            onSetRoster(input)
            setEditing(false)
          }}
        />
      )}

      <ul className="space-y-1.5">
        {participants.map((participant) => (
          <li
            key={participant.id}
            className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 transition hover:bg-white/5"
          >
            <span
              className={[
                'h-2 w-2 shrink-0 rounded-full',
                participant.online ? 'bg-emerald-400' : 'bg-slate-600',
              ].join(' ')}
              title={participant.online ? 'Online' : 'Offline'}
            />
            <span className="min-w-0 flex-1 truncate text-sm text-slate-200">
              {participant.name}
              {participant.id === me.id && <span className="text-slate-500"> (you)</span>}
            </span>

            {participant.isHost && <span className="chip !px-2 !py-0.5 text-[10px]">host</span>}
            {participant.isObserver ? (
              <span className="chip !px-2 !py-0.5 text-[10px]">observer</span>
            ) : (
              <ProgressBadge
                participant={participant}
                isAsync={isAsync}
                total={votable}
                votedCurrent={currentTopic?.votedBy.includes(participant.id) ?? false}
                revealed={currentTopic?.revealed ?? false}
              />
            )}

            {me.isHost && participant.id !== me.id && (
              <button
                type="button"
                onClick={() => onKick(participant.id)}
                className="text-xs text-slate-600 transition hover:text-rose-300"
                title={`Remove ${participant.name}`}
              >
                ✕
              </button>
            )}
          </li>
        ))}
      </ul>

      {(missingNames.length > 0 || unnamedSeats > 0) && (
        <div className="mt-3 border-t border-white/5 pt-3">
          <p className="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-slate-600">Not here yet</p>
          <div className="flex flex-wrap gap-1.5">
            {missingNames.map((name) => (
              <span
                key={name}
                className="rounded-full border border-dashed border-white/10 px-2 py-0.5 text-xs text-slate-500"
              >
                {name}
              </span>
            ))}
            {unnamedSeats > 0 && (
              <span className="rounded-full border border-dashed border-white/10 px-2 py-0.5 text-xs text-slate-600">
                {unnamedSeats} seat{unnamedSeats === 1 ? '' : 's'} open
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

interface RosterEditorProps {
  size: number
  names: string[]
  onSave: (input: { size: number; names: string[] }) => void
  onCancel: () => void
}

/**
 * RosterEditor lets the host say how many people are coming and, if they know
 * them, who. Names are free text on purpose: they are matched against display
 * names only to grey out who is missing, never to let anyone in or keep them
 * out.
 */
function RosterEditor({ size, names, onSave, onCancel }: RosterEditorProps) {
  const [count, setCount] = useState(String(size || ''))
  const [text, setText] = useState(names.join('\n'))

  // Reopening the editor after somebody else changed the roster should show the
  // current value, not a stale draft.
  useEffect(() => {
    setCount(String(size || ''))
    setText(names.join('\n'))
  }, [size, names])

  const parsed = text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        onSave({ size: Number(count) || parsed.length, names: parsed })
      }}
      className="mb-3 space-y-2 rounded-lg border border-white/10 bg-white/[0.02] p-3"
    >
      <label className="block">
        <span className="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          People expected
        </span>
        <input
          type="number"
          min={0}
          max={100}
          value={count}
          onChange={(e) => setCount(e.target.value)}
          placeholder="0"
          className="w-24 rounded-md border border-white/10 bg-slate-900/60 px-2 py-1 text-sm text-slate-100 outline-none focus:border-indigo-400/60"
        />
      </label>
      <label className="block">
        <span className="mb-1 block text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          Names <span className="normal-case tracking-normal text-slate-600">(one per line, optional)</span>
        </span>
        <textarea
          rows={4}
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={'Ada\nJay'}
          className="w-full resize-y rounded-md border border-white/10 bg-slate-900/60 px-2 py-1 text-sm text-slate-100 outline-none focus:border-indigo-400/60"
        />
      </label>
      <div className="flex justify-end gap-2">
        <button type="button" onClick={onCancel} className="btn-ghost !px-3 !py-1 text-xs">
          Cancel
        </button>
        <button type="submit" className="btn-primary !px-3 !py-1 text-xs">
          Save
        </button>
      </div>
    </form>
  )
}

interface ProgressBadgeProps {
  participant: ParticipantView
  isAsync: boolean
  total: number
  votedCurrent: boolean
  revealed: boolean
}

function ProgressBadge({ participant, isAsync, total, votedCurrent, revealed }: ProgressBadgeProps) {
  if (isAsync) {
    const done = participant.votedTopics
    const complete = total > 0 && done >= total
    return (
      <span
        className={[
          'shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold',
          complete ? 'bg-emerald-500/15 text-emerald-300' : 'bg-white/5 text-slate-400',
        ].join(' ')}
        title="Topics estimated"
      >
        {done}/{total}
      </span>
    )
  }

  if (revealed) return null
  return (
    <span
      className={[
        'shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold',
        votedCurrent ? 'bg-emerald-500/15 text-emerald-300' : 'bg-white/5 text-slate-500',
      ].join(' ')}
    >
      {votedCurrent ? 'ready' : 'thinking…'}
    </span>
  )
}
