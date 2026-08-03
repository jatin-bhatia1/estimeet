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
