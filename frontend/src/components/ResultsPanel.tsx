import type { ParticipantView, TopicView } from '../lib/types'
import { cardLabel } from '../lib/types'
import { PlayingCard } from './PlayingCard'

interface ResultsPanelProps {
  topic: TopicView
  deck: string[]
  participants: ParticipantView[]
  isHost: boolean
  onReset: () => void
  onEstimate: (value: string) => void
}

/** ResultsPanel shows the revealed hand, the statistics and the host's wrap-up controls. */
export function ResultsPanel({ topic, deck, participants, isHost, onReset, onEstimate }: ResultsPanelProps) {
  const stats = topic.stats
  const maxCount = stats?.distribution.reduce((max, entry) => Math.max(max, entry.count), 0) ?? 0
  const numericDeck = deck.filter((card) => card !== '?' && card !== 'coffee')
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
            {numericDeck.map((card) => (
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
