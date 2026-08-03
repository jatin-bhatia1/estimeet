import type { RoomActions } from '../lib/actions'
import type { RoomState } from '../lib/types'
import { Deck } from './PlayingCard'
import { ResultsPanel } from './ResultsPanel'

interface SyncBoardProps {
  state: RoomState
  actions: RoomActions
}

/**
 * SyncBoard is the live, host-driven table: one topic on screen, everybody
 * plays a card, the cards flip together.
 */
export function SyncBoard({ state, actions }: SyncBoardProps) {
  const { room, me, topics, participants } = state
  const topic = topics.find((t) => t.id === room.currentTopicId) ?? null
  const index = topic ? topics.findIndex((t) => t.id === topic.id) : -1

  if (!topic) {
    return (
      <div className="panel flex min-h-64 items-center justify-center p-8 text-center">
        <p className="text-slate-400">
          {topics.length === 0
            ? 'The backlog is empty. Add a topic or import from your tracker to get started.'
            : 'The host has not picked a topic yet.'}
        </p>
      </div>
    )
  }

  const nameById = new Map(participants.map((p) => [p.id, p.name]))
  const waitingOn = topic.pendingVoters.map((id) => nameById.get(id) ?? 'someone')

  return (
    <div className="panel space-y-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="mb-1 text-xs uppercase tracking-wider text-slate-500">
            Topic {index + 1} of {topics.length}
          </p>
          <h2 className="text-xl font-semibold text-slate-50">
            {topic.externalKey && (
              <a
                href={topic.externalUrl}
                target="_blank"
                rel="noreferrer noopener"
                className="mr-2 text-accent-400 hover:underline"
              >
                {topic.externalKey}
              </a>
            )}
            {topic.title}
          </h2>
          {topic.description && (
            <p className="mt-2 max-h-40 overflow-y-auto whitespace-pre-wrap text-sm leading-relaxed text-slate-400">
              {topic.description}
            </p>
          )}
        </div>

        {me.isHost && (
          <div className="flex shrink-0 gap-2">
            <button
              type="button"
              className="btn-ghost !px-3"
              onClick={() => actions.advance('prev')}
              disabled={index <= 0}
              title="Previous topic"
            >
              ←
            </button>
            <button
              type="button"
              className="btn-ghost !px-3"
              onClick={() => actions.advance('next')}
              disabled={index >= topics.length - 1}
              title="Next topic"
            >
              →
            </button>
          </div>
        )}
      </div>

      <AtTheTable state={state} votedBy={topic.votedBy} revealed={topic.revealed} />

      {topic.revealed ? (
        <ResultsPanel
          topic={topic}
          deck={room.deck}
          participants={participants}
          isHost={me.isHost}
          onReset={() => void actions.reset(topic.id)}
          onEstimate={(value) => void actions.estimate(topic.id, value)}
        />
      ) : (
        <div className="space-y-5">
          {me.isObserver ? (
            <p className="rounded-xl border border-white/10 bg-black/20 p-4 text-sm text-slate-400">
              You are observing this session.
            </p>
          ) : (
            <Deck
              deck={room.deck}
              myVote={topic.myVote}
              size="lg"
              disabled={!topic.canVote}
              onPick={(value) => void actions.vote(topic.id, value)}
              onClear={() => void actions.clearVote(topic.id)}
            />
          )}

          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/5 pt-4">
            <p className="text-sm text-slate-400">
              {waitingOn.length === 0 ? (
                <span className="text-emerald-300">Everybody has played.</span>
              ) : (
                <>
                  Waiting on <span className="text-slate-200">{waitingOn.join(', ')}</span>
                </>
              )}
            </p>

            {me.isHost && (
              <button
                type="button"
                className="btn-primary"
                onClick={() => void actions.reveal(topic.id)}
                disabled={topic.votedBy.length === 0}
              >
                Reveal cards
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

interface AtTheTableProps {
  state: RoomState
  votedBy: string[]
  revealed: boolean
}

/**
 * AtTheTable shows who has actually joined the live session, so the host does
 * not have to read the sidebar to know whether the room is complete. When the
 * host declared a roster, the people still missing are listed as outlines.
 */
function AtTheTable({ state, votedBy, revealed }: AtTheTableProps) {
  const { room, participants } = state
  const voted = new Set(votedBy)
  const here = new Set(participants.map((p) => p.name.trim().toLowerCase()))
  const missing = room.expectedNames.filter((name) => !here.has(name.trim().toLowerCase()))
  const players = participants.filter((p) => !p.isObserver)
  const seatsOpen = Math.max(0, room.expectedSize - players.length - missing.length)

  return (
    <div className="flex flex-wrap items-center gap-1.5 border-y border-white/5 py-3">
      <span className="mr-1 text-[11px] font-semibold uppercase tracking-wide text-slate-600">
        At the table{room.expectedSize > 0 ? ` ${players.length}/${room.expectedSize}` : ''}
      </span>

      {participants.map((p) => {
        const played = voted.has(p.id)
        return (
          <span
            key={p.id}
            className={[
              'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition',
              p.online ? '' : 'opacity-50',
              p.isObserver
                ? 'border-white/10 bg-white/[0.03] text-slate-400'
                : played && !revealed
                  ? 'border-emerald-400/30 bg-emerald-500/10 text-emerald-200'
                  : 'border-white/10 bg-white/5 text-slate-300',
            ].join(' ')}
            title={
              p.isObserver
                ? `${p.name} is observing`
                : `${p.name} is ${p.online ? 'online' : 'offline'}${revealed ? '' : played ? ' and has played' : ' and is still thinking'}`
            }
          >
            <span className={['h-1.5 w-1.5 rounded-full', p.online ? 'bg-emerald-400' : 'bg-slate-600'].join(' ')} />
            {p.name}
            {p.isHost && <span className="text-[10px] uppercase tracking-wide text-slate-500">host</span>}
          </span>
        )
      })}

      {missing.map((name) => (
        <span
          key={name}
          className="inline-flex items-center gap-1.5 rounded-full border border-dashed border-white/10 px-2.5 py-1 text-xs text-slate-500"
          title={`${name} has not joined yet`}
        >
          {name}
        </span>
      ))}

      {seatsOpen > 0 && (
        <span className="rounded-full border border-dashed border-white/10 px-2.5 py-1 text-xs text-slate-600">
          {seatsOpen} seat{seatsOpen === 1 ? '' : 's'} open
        </span>
      )}
    </div>
  )
}
