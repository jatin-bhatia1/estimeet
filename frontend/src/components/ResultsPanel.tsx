import { useState } from 'react'

import type { NoteKind, ParticipantView, SourceKind, SourceView, TopicView } from '../lib/types'
import { COFFEE_CARD, cardLabel } from '../lib/types'
import { DiscussionNotes } from './DiscussionNotes'
import { PlayingCard } from './PlayingCard'

interface ResultsPanelProps {
  topic: TopicView
  deck: string[]
  participants: ParticipantView[]
  isHost: boolean
  canAddNote: boolean
  /** The room's tracker connection, if any. */
  source?: SourceView
  onReset: () => void
  onEstimate: (value: string) => void
  onPushEstimate: (value: string) => Promise<boolean>
  onAddNote: (kind: NoteKind, body: string) => Promise<boolean>
  onDeleteNote: (noteId: string) => void
}

/** ResultsPanel shows the revealed hand, the statistics and the host's wrap-up controls. */
export function ResultsPanel({
  topic,
  deck,
  participants,
  isHost,
  canAddNote,
  source,
  onReset,
  onEstimate,
  onPushEstimate,
  onAddNote,
  onDeleteNote,
}: ResultsPanelProps) {
  const stats = topic.stats
  const maxCount = stats?.distribution.reduce((max, entry) => Math.max(max, entry.count), 0) ?? 0
  // The escape cards are answers, not sizes, so they cannot be agreed on.
  const agreeable = deck.filter((card) => card !== '?' && card !== COFFEE_CARD)
  const byId = new Map(participants.map((p) => [p.id, p]))

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap gap-3">
        {topic.votes.map((vote) => (
          <div key={vote.participantId} className="animate-flip flex flex-col items-center gap-1.5">
            <PlayingCard value={vote.value} size="md" />
            <span className="max-w-16 truncate text-xs text-slate-400" title={vote.participantName}>
              {byId.get(vote.participantId)?.name ?? vote.participantName}
            </span>
          </div>
        ))}
        {topic.votes.length === 0 && <p className="text-sm text-slate-500">Nobody played a card.</p>}
      </div>

      {stats && (
        <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_16rem]">
          <div className="panel p-4">
            <p className="label mb-3">Distribution</p>
            <div className="space-y-2">
              {stats.distribution.map((entry) => (
                <div key={entry.value} className="flex items-center gap-3">
                  <span className="w-8 shrink-0 text-right text-sm font-semibold text-slate-200">
                    {cardLabel(entry.value)}
                  </span>
                  <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-white/5">
                    <div
                      className="h-full rounded-full bg-accent-500/80"
                      style={{ width: `${maxCount ? (entry.count / maxCount) * 100 : 0}%` }}
                    />
                  </div>
                  <span className="w-6 text-xs text-slate-400">{entry.count}</span>
                </div>
              ))}
            </div>
          </div>

          <div className="panel space-y-2.5 p-4 text-sm">
            {stats.consensus ? (
              <p className="rounded-lg bg-emerald-500/10 px-3 py-2 text-emerald-300">Unanimous — nice.</p>
            ) : (
              <p className="rounded-lg bg-amber-500/10 px-3 py-2 text-amber-200">
                {stats.spread >= 3 ? 'Wide spread — worth a conversation.' : 'Close, but not unanimous.'}
              </p>
            )}
            <Row label="Average" value={stats.average?.toString() ?? '—'} />
            <Row label="Median" value={stats.median?.toString() ?? '—'} />
            <Row
              label="Range"
              value={stats.min && stats.max ? `${cardLabel(stats.min)} – ${cardLabel(stats.max)}` : '—'}
            />
            <Row label="Cards played" value={String(stats.voteCount)} />
          </div>
        </div>
      )}

      <DiscussionNotes topic={topic} isHost={isHost} canAdd={canAddNote} onAdd={onAddNote} onDelete={onDeleteNote} />

      {isHost && (
        <div className="panel p-4">
          <p className="label">
            {topic.finalEstimate ? 'Agreed estimate' : 'Agree on an estimate'}
            {stats?.suggested && !topic.finalEstimate && (
              <span className="ml-2 normal-case tracking-normal text-slate-500">
                suggestion: {cardLabel(stats.suggested)}
              </span>
            )}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            {agreeable.map((card) => (
              <button
                key={card}
                type="button"
                onClick={() => onEstimate(card)}
                className={[
                  'h-9 w-10 rounded-lg border text-sm font-semibold transition',
                  topic.finalEstimate === card
                    ? 'border-emerald-400 bg-emerald-500 text-slate-950'
                    : card === stats?.suggested
                      ? 'border-accent-500/50 bg-accent-500/10 text-accent-400 hover:bg-accent-500/20'
                      : 'border-white/10 bg-white/5 text-slate-300 hover:bg-white/10',
                ].join(' ')}
              >
                {card}
              </button>
            ))}
            <button type="button" onClick={onReset} className="btn-ghost ml-auto">
              Vote again
            </button>
          </div>
        </div>
      )}

      {isHost && topic.externalKey && (
        <SendToTracker topic={topic} agreeable={agreeable} source={source} onPush={onPushEstimate} />
      )}
    </div>
  )
}

const TRACKER_NAME: Record<SourceKind, string> = { jira: 'Jira', azure: 'Azure DevOps', github: 'GitHub' }

interface SendToTrackerProps {
  topic: TopicView
  agreeable: string[]
  source?: SourceView
  onPush: (value: string) => Promise<boolean>
}

/**
 * SendToTracker writes a card from the deck onto the story the topic was
 * imported from, and makes it the agreed estimate in the same step.
 */
function SendToTracker({ topic, agreeable, source, onPush }: SendToTrackerProps) {
  const [choice, setChoice] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [sent, setSent] = useState<string | null>(null)

  if (!source) {
    return (
      <p className="text-xs text-slate-500">
        This story came from a tracker. Connect the room again to send estimates back to it.
      </p>
    )
  }

  const name = TRACKER_NAME[source.provider]
  // Jira and Azure keep the estimate in a number field, so letters cannot go there.
  const options = source.provider === 'github' ? agreeable : agreeable.filter((card) => !Number.isNaN(Number(card)))
  const value = choice ?? (topic.finalEstimate && options.includes(topic.finalEstimate) ? topic.finalEstimate : null) ??
    (topic.stats?.suggested && options.includes(topic.stats.suggested) ? topic.stats.suggested : null) ??
    options[0] ?? ''

  const send = async () => {
    if (!value || busy) return
    setBusy(true)
    try {
      setSent((await onPush(value)) ? value : null)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="panel flex flex-wrap items-center gap-3 p-4">
      <div className="min-w-0 flex-1">
        <p className="label !mb-0.5">Send to {name}</p>
        <p className="truncate text-xs text-slate-500">
          {source.provider === 'github'
            ? `Sets an "estimate: ${value || '…'}" label on ${topic.externalKey}.`
            : `Sets the story points of ${topic.externalKey}.`}
        </p>
      </div>
      <select
        className="field !w-24"
        value={value}
        onChange={(e) => {
          setChoice(e.target.value)
          setSent(null)
        }}
        aria-label={`Estimate to send to ${name}`}
      >
        {options.map((card) => (
          <option key={card} value={card}>
            {card}
          </option>
        ))}
      </select>
      <button type="button" className="btn-primary" onClick={() => void send()} disabled={busy || !value}>
        {busy ? 'Sending…' : sent === value ? `Sent to ${name}` : `Update ${name}`}
      </button>
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between border-b border-white/5 pb-1.5 last:border-0">
      <span className="text-xs uppercase tracking-wider text-slate-500">{label}</span>
      <span className="font-semibold text-slate-100">{value}</span>
    </div>
  )
}
