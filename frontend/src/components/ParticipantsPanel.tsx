import type { ParticipantView, RoomState } from '../lib/types'

interface ParticipantsPanelProps {
  state: RoomState
  onKick: (participantId: string) => void
}

/**
 * ParticipantsPanel doubles as the presence list and, in asynchronous rooms,
 * the progress board: it shows how far each teammate has worked through the backlog.
 */
export function ParticipantsPanel({ state, onKick }: ParticipantsPanelProps) {
  const { room, me, participants, topics } = state
  const isAsync = room.mode === 'async'
  const currentTopic = topics.find((t) => t.id === room.currentTopicId)
  const votable = topics.length

  return (
    <div className="panel p-4">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-slate-200">
          Team <span className="text-slate-500">({participants.length})</span>
        </h2>
        <span className="text-xs text-slate-500">{participants.filter((p) => p.online).length} online</span>
      </div>

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
    </div>
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
