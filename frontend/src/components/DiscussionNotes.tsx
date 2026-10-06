import { type FormEvent, useState } from 'react'

import type { NoteKind, NoteView, TopicView } from '../lib/types'

const KINDS: { kind: NoteKind; label: string; plural: string; tone: string }[] = [
  { kind: 'question', label: 'Question', plural: 'Questions', tone: 'text-sky-300 border-sky-400/30 bg-sky-500/10' },
  { kind: 'concern', label: 'Concern', plural: 'Concerns', tone: 'text-amber-200 border-amber-400/30 bg-amber-500/10' },
  {
    kind: 'suggestion',
    label: 'Suggestion',
    plural: 'Suggestions',
    tone: 'text-emerald-300 border-emerald-400/30 bg-emerald-500/10',
  },
]

interface DiscussionNotesProps {
  topic: TopicView
  isHost: boolean
  canAdd: boolean
  /** Tucks the composer behind a button, for lists where every topic has one. */
  compact?: boolean
  onAdd: (kind: NoteKind, body: string) => Promise<boolean>
  onDelete: (noteId: string) => void
}

/**
 * DiscussionNotes collects questions, concerns and suggestions while a topic is
 * being estimated, and lays them out as the agenda for the conversation once the
 * cards are up.
 */
export function DiscussionNotes({ topic, isHost, canAdd, compact = false, onAdd, onDelete }: DiscussionNotesProps) {
  const [kind, setKind] = useState<NoteKind>('question')
  const [body, setBody] = useState('')
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState(!compact)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const text = body.trim()
    if (!text || busy) return
    setBusy(true)
    try {
      if (await onAdd(kind, text)) setBody('')
    } finally {
      setBusy(false)
    }
  }

  const grouped = KINDS.map((entry) => ({ ...entry, notes: topic.notes.filter((n) => n.kind === entry.kind) })).filter(
    (entry) => entry.notes.length > 0,
  )

  if (!canAdd && grouped.length === 0) return null

  return (
    <section className="space-y-3 rounded-xl border border-white/10 bg-black/20 p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="label !mb-0">{topic.revealed ? 'Points for the discussion' : 'Your notes for the discussion'}</p>
        {!topic.revealed && (
          <p className="text-xs text-slate-500">Only you see these until the cards are turned over.</p>
        )}
      </div>

      {grouped.map((group) => (
        <div key={group.kind} className="space-y-1.5">
          <p className="text-[11px] font-semibold uppercase tracking-wide text-slate-500">{group.plural}</p>
          <ul className="space-y-1.5">
            {group.notes.map((note) => (
              <NoteRow
                key={note.id}
                note={note}
                tone={group.tone}
                showAuthor={topic.revealed}
                removable={note.mine || isHost}
                onDelete={() => onDelete(note.id)}
              />
            ))}
          </ul>
        </div>
      ))}

      {canAdd &&
        (open ? (
          <form onSubmit={submit} className="space-y-2">
            <div className="flex flex-wrap gap-1.5" role="radiogroup" aria-label="Kind of note">
              {KINDS.map((entry) => (
                <button
                  key={entry.kind}
                  type="button"
                  role="radio"
                  aria-checked={kind === entry.kind}
                  onClick={() => setKind(entry.kind)}
                  className={[
                    'rounded-full border px-3 py-1 text-xs font-medium transition',
                    kind === entry.kind ? entry.tone : 'border-white/10 text-slate-400 hover:text-slate-200',
                  ].join(' ')}
                >
                  {entry.label}
                </button>
              ))}
            </div>
            <div className="flex gap-2">
              <input
                className="field"
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder={
                  kind === 'question'
                    ? 'What is unclear about this story?'
                    : kind === 'concern'
                      ? 'What could go wrong or take longer?'
                      : 'What would make this easier or smaller?'
                }
                maxLength={500}
              />
              <button type="submit" className="btn-ghost shrink-0" disabled={busy || body.trim() === ''}>
                Add
              </button>
            </div>
          </form>
        ) : (
          <button type="button" onClick={() => setOpen(true)} className="text-xs text-slate-400 underline hover:text-slate-200">
            Add a question, concern or suggestion
          </button>
        ))}
    </section>
  )
}

interface NoteRowProps {
  note: NoteView
  tone: string
  showAuthor: boolean
  removable: boolean
  onDelete: () => void
}

function NoteRow({ note, tone, showAuthor, removable, onDelete }: NoteRowProps) {
  return (
    <li className={['flex items-start justify-between gap-3 rounded-lg border px-3 py-2 text-sm', tone].join(' ')}>
      <p className="min-w-0 whitespace-pre-wrap break-words text-slate-100">
        {note.body}
        {showAuthor && <span className="ml-2 text-xs text-slate-400">{note.mine ? 'you' : note.participantName}</span>}
      </p>
      {removable && (
        <button
          type="button"
          onClick={onDelete}
          className="shrink-0 text-xs text-slate-500 transition hover:text-rose-300"
          title="Remove note"
        >
          ✕
        </button>
      )}
    </li>
  )
}
