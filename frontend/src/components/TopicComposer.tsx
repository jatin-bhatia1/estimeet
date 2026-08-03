import { type FormEvent, useState } from 'react'

import type { TopicDraft } from '../lib/actions'

interface TopicComposerProps {
  onAdd: (topics: TopicDraft[]) => Promise<void>
}

/**
 * TopicComposer is the manual alternative to the Jira import: type one topic,
 * or paste a whole list and get one topic per line.
 */
export function TopicComposer({ onAdd }: TopicComposerProps) {
  const [bulk, setBulk] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [list, setList] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()

    const drafts: TopicDraft[] = bulk
      ? list
          .split('\n')
          .map((line) => line.trim())
          .filter(Boolean)
          .map((line) => ({ title: line, description: '' }))
      : [{ title: title.trim(), description: description.trim() }]

    if (drafts.length === 0 || !drafts[0].title) return

    setBusy(true)
    try {
      await onAdd(drafts)
      setTitle('')
      setDescription('')
      setList('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="panel space-y-3 p-4">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold text-slate-200">Add topics</h2>
        <button
          type="button"
          onClick={() => setBulk((v) => !v)}
          className="text-xs text-slate-400 underline hover:text-slate-200"
        >
          {bulk ? 'single topic' : 'paste a list'}
        </button>
      </div>

      {bulk ? (
        <textarea
          className="field min-h-28 resize-y font-mono text-xs"
          value={list}
          onChange={(e) => setList(e.target.value)}
          placeholder={'One topic per line:\nCheckout redesign\nRate limiting\nAudit log export'}
        />
      ) : (
        <>
          <input
            className="field"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="What are we estimating?"
            maxLength={200}
          />
          <textarea
            className="field min-h-20 resize-y"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Context, acceptance criteria, unknowns… (optional)"
            maxLength={4000}
          />
        </>
      )}

      <button type="submit" className="btn-primary w-full" disabled={busy}>
        {busy ? 'Adding…' : bulk ? 'Add all' : 'Add topic'}
      </button>
    </form>
  )
}
