import { useState } from 'react'

import type { RoomActions } from '../lib/actions'
import type { RoomState, TopicView } from '../lib/types'
import { cardLabel } from '../lib/types'
import { Deck } from './PlayingCard'
import { ResultsPanel } from './ResultsPanel'

type Filter = 'todo' | 'all' | 'done'

interface AsyncBoardProps {
  state: RoomState
  actions: RoomActions
}

/**
 * AsyncBoard opens the whole backlog at once: everybody estimates at their own
 * pace and each topic flips as soon as the last teammate has voted.
 */
export function AsyncBoard({ state, actions }: AsyncBoardProps) {
  const [filter, setFilter] = useState<Filter>('todo')
  const { room, me, topics, participants, summary } = state

  const visible = topics.filter((topic) => {
    if (filter === 'all') return true
    if (filter === 'done') return topic.revealed
    return !topic.revealed && (topic.myVote === null || me.isObserver)
  })

  return (
    <div className="space-y-4">
      <div className="panel flex flex-wrap items-center justify-between gap-3 p-4">
        <div>
          <p className="text-sm text-slate-300">
            {summary.totalTopics === 0 ? (
              'Nothing to estimate yet.'
            ) : me.isObserver ? (
              'You are observing this session.'
            ) : summary.myRemaining === 0 ? (
              <span className="text-emerald-300">You have estimated everything. Nice.</span>
            ) : (
              <>
                <span className="font-semibold text-slate-100">{summary.myRemaining}</span> topic
                {summary.myRemaining === 1 ? '' : 's'} still waiting for your card.
              </>
            )}
          </p>
          <div className="mt-2 h-1.5 w-56 overflow-hidden rounded-full bg-white/5">
            <div
              className="h-full rounded-full bg-async-500 transition-all"
              style={{
                width: `${summary.totalTopics ? ((summary.totalTopics - summary.myRemaining) / summary.totalTopics) * 100 : 0}%`,
              }}
            />
          </div>
        </div>

        <div className="inline-flex rounded-xl border border-white/10 bg-black/20 p-1">
          {(['todo', 'all', 'done'] as Filter[]).map((value) => (
            <button
              key={value}
              type="button"
              onClick={() => setFilter(value)}
              className={[
                'rounded-lg px-3 py-1.5 text-xs font-medium capitalize transition',
                filter === value ? 'bg-white/10 text-slate-100' : 'text-slate-500 hover:text-slate-300',
              ].join(' ')}
            >
              {value === 'todo' ? 'to estimate' : value}
            </button>
          ))}
        </div>
      </div>

      {visible.length === 0 ? (
        <div className="panel p-8 text-center text-sm text-slate-500">
          {topics.length === 0
            ? 'No topics yet. Add some, or import them from your tracker.'
            : 'Nothing here — try another filter.'}
        </div>
      ) : (
        visible.map((topic) => (
          <AsyncTopicCard
            key={topic.id}
            topic={topic}
            state={state}
            actions={actions}
            participants={participants}
            deck={room.deck}
          />
        ))
      )}
    </div>
  )
}

interface AsyncTopicCardProps {
  topic: TopicView
  state: RoomState
  actions: RoomActions
  participants: RoomState['participants']
  deck: string[]
}

function AsyncTopicCard({ topic, state, actions, participants, deck }: AsyncTopicCardProps) {
  const [expanded, setExpanded] = useState(false)
  const { me } = state
  const voters = participants.filter((p) => !p.isObserver).length

  return (
    <article className="panel space-y-4 p-5">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h3 className="text-base font-semibold text-slate-50">
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
          </h3>

          {topic.description && (
            <>
              <p
                className={[
                  'mt-1.5 whitespace-pre-wrap text-sm leading-relaxed text-slate-400',
                  expanded ? '' : 'line-clamp-2',
                ].join(' ')}
              >
                {topic.description}
              </p>
              <button
                type="button"
                onClick={() => setExpanded((v) => !v)}
                className="mt-1 text-xs text-slate-500 underline hover:text-slate-300"
              >
                {expanded ? 'less' : 'more'}
              </button>
            </>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-2">
          {topic.finalEstimate ? (
            <span className="rounded-lg bg-emerald-500/15 px-2.5 py-1 text-sm font-semibold text-emerald-300">
              {cardLabel(topic.finalEstimate)}
            </span>
          ) : (
            <span className="chip">
              {topic.votedBy.length}/{voters} voted
            </span>
          )}
          {me.isHost && (
            <button
              type="button"
              onClick={() => void actions.deleteTopic(topic.id)}
              className="text-xs text-slate-700 transition hover:text-rose-300"
              title="Remove topic"
            >
              ✕
            </button>
          )}
        </div>
      </div>

      {topic.revealed ? (
        <ResultsPanel
          topic={topic}
          deck={deck}
          participants={participants}
          isHost={me.isHost}
          onReset={() => void actions.reset(topic.id)}
          onEstimate={(value) => void actions.estimate(topic.id, value)}
        />
      ) : me.isObserver ? null : (
        <div className="flex flex-wrap items-end justify-between gap-3">
          <Deck
            deck={deck}
            myVote={topic.myVote}
            size="sm"
            disabled={!topic.canVote}
            onPick={(value) => void actions.vote(topic.id, value)}
            onClear={() => void actions.clearVote(topic.id)}
          />
          {me.isHost && topic.votedBy.length > 0 && (
            <button type="button" className="btn-ghost !py-1.5 text-xs" onClick={() => void actions.reveal(topic.id)}>
              Reveal now
            </button>
          )}
        </div>
      )}
    </article>
  )
}
