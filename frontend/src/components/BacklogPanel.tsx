import type { RoomState, TopicStatus, TopicView } from '../lib/types'
import { cardLabel } from '../lib/types'

const STATUS_DOT: Record<TopicStatus, string> = {
  pending: 'bg-slate-600',
  voting: 'bg-amber-400',
  revealed: 'bg-sky-400',
  estimated: 'bg-emerald-400',
}

interface BacklogPanelProps {
  state: RoomState
  onFocus: (topicId: string) => void
  onDelete: (topicId: string) => void
}

/** BacklogPanel is the ordered list of topics with their lifecycle at a glance. */
export function BacklogPanel({ state, onFocus, onDelete }: BacklogPanelProps) {
  const { room, me, topics, summary } = state
  const isSync = room.mode === 'sync'

  return (
    <div className="panel flex min-h-0 flex-col p-4">
      <div className="mb-3 flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-slate-200">Backlog</h2>
        <span className="text-xs text-slate-500">
          {summary.estimatedTopics}/{summary.totalTopics} estimated
          {summary.totalPoints !== undefined && ` · ${summary.totalPoints} pts`}
        </span>
      </div>

      {topics.length === 0 ? (
        <p className="py-6 text-center text-sm text-slate-500">
          No topics yet.
          {me.isHost ? ' Add some below, or import an epic from Jira.' : ' Waiting for the host.'}
        </p>
      ) : (
        <ol className="-mx-1 max-h-[26rem] space-y-1 overflow-y-auto px-1">
          {topics.map((topic, index) => (
            <BacklogRow
              key={topic.id}
              topic={topic}
              index={index}
              selectable={isSync && me.isHost}
              deletable={me.isHost}
              onFocus={() => onFocus(topic.id)}
              onDelete={() => onDelete(topic.id)}
            />
          ))}
        </ol>
      )}
    </div>
  )
}

interface BacklogRowProps {
  topic: TopicView
  index: number
  selectable: boolean
  deletable: boolean
  onFocus: () => void
  onDelete: () => void
}

function BacklogRow({ topic, index, selectable, deletable, onFocus, onDelete }: BacklogRowProps) {
  return (
    <li
      className={[
        'group flex items-center gap-2.5 rounded-lg border px-2.5 py-2 transition',
        topic.isCurrent ? 'border-accent-500/50 bg-accent-500/10' : 'border-transparent hover:bg-white/5',
      ].join(' ')}
    >
      <span className={`h-2 w-2 shrink-0 rounded-full ${STATUS_DOT[topic.status]}`} title={topic.status} />

      <button
        type="button"
        onClick={selectable ? onFocus : undefined}
        disabled={!selectable}
        className="min-w-0 flex-1 text-left disabled:cursor-default"
      >
        <span className="block truncate text-sm text-slate-200">
          <span className="mr-1.5 text-slate-600">{index + 1}.</span>
          {topic.externalKey && <span className="mr-1.5 text-xs font-semibold text-accent-400">{topic.externalKey}</span>}
          {topic.title}
        </span>
      </button>

      {topic.finalEstimate && (
        <span className="shrink-0 rounded-md bg-emerald-500/15 px-1.5 py-0.5 text-xs font-semibold text-emerald-300">
          {cardLabel(topic.finalEstimate)}
        </span>
      )}

      {deletable && (
        <button
          type="button"
          onClick={onDelete}
          className="shrink-0 text-xs text-slate-700 opacity-0 transition group-hover:opacity-100 hover:text-rose-300"
          title="Remove topic"
        >
          ✕
        </button>
      )}
    </li>
  )
}
