import { useState } from 'react'

import { COFFEE_CARD, cardLabel } from '../lib/types'

/** Mirrors the limits the API enforces in service.normaliseDeck. */
export const MAX_DECK_SIZE = 16
export const MAX_CARD_LEN = 6

export const DECK_PRESETS = [
  {
    id: 'fibonacci',
    label: 'Fibonacci',
    hint: 'The classic. Gaps that grow with the uncertainty.',
    cards: ['0', '1', '2', '3', '5', '8', '13', '21', '34', '55', '89', '?', COFFEE_CARD],
  },
  {
    id: 'tshirt',
    label: 'T-shirt sizes',
    hint: 'Relative sizing without numbers. No average is calculated.',
    cards: ['XS', 'S', 'M', 'L', 'XL', 'XXL', '?', COFFEE_CARD],
  },
  {
    id: 'powers',
    label: 'Powers of two',
    hint: 'Every step is a doubling.',
    cards: ['1', '2', '4', '8', '16', '32', '64', '?', COFFEE_CARD],
  },
] as const

export const DEFAULT_DECK: string[] = [...DECK_PRESETS[0].cards]

/** parseDeck turns whatever the host typed into a deck the API will accept. */
export function parseDeck(text: string): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const raw of text.split(/[\s,]+/)) {
    const card = raw.slice(0, MAX_CARD_LEN)
    const key = card.toLowerCase()
    if (!card || seen.has(key)) continue
    seen.add(key)
    out.push(card)
  }
  return out.slice(0, MAX_DECK_SIZE)
}

function sameDeck(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((card, i) => card === b[i])
}

/** presetOf names the deck if it is one of ours, so a reopened form looks familiar. */
function presetOf(deck: readonly string[]): string {
  return DECK_PRESETS.find((preset) => sameDeck(preset.cards, deck))?.id ?? 'custom'
}

interface DeckPickerProps {
  deck: string[]
  onChange: (deck: string[]) => void
  disabled?: boolean
}

export function DeckPicker({ deck, onChange, disabled = false }: DeckPickerProps) {
  const selected = presetOf(deck)
  // Kept apart from the deck so trailing separators survive while typing.
  const [draft, setDraft] = useState(() => deck.join(' '))

  const pick = (id: string) => {
    const preset = DECK_PRESETS.find((p) => p.id === id)
    const cards = preset ? [...preset.cards] : deck
    setDraft(cards.join(' '))
    onChange(cards)
  }

  const type = (text: string) => {
    setDraft(text)
    onChange(parseDeck(text))
  }

  return (
    <div className="space-y-2.5">
      <div className="flex flex-wrap gap-1.5">
        {[...DECK_PRESETS, { id: 'custom', label: 'Custom', hint: '', cards: [] }].map((preset) => (
          <button
            key={preset.id}
            type="button"
            disabled={disabled}
            onClick={() => pick(preset.id)}
            className={[
              'rounded-lg border px-2.5 py-1 text-xs font-medium transition disabled:opacity-50',
              selected === preset.id
                ? 'border-accent-500/60 bg-accent-500/15 text-slate-100'
                : 'border-white/10 bg-black/20 text-slate-400 hover:border-white/25',
            ].join(' ')}
          >
            {preset.label}
          </button>
        ))}
      </div>

      <input
        className="field !py-1.5 font-mono text-sm"
        value={draft}
        onChange={(e) => type(e.target.value)}
        onBlur={() => setDraft(deck.join(' '))}
        disabled={disabled}
        placeholder="0 1 2 3 5 8 ? coffee"
        aria-label="Cards in the deck"
      />

      <div className="flex flex-wrap items-center gap-1">
        {deck.map((card) => (
          <span
            key={card}
            className="rounded-md border border-white/10 bg-white/5 px-2 py-0.5 text-xs font-semibold text-slate-200"
          >
            {cardLabel(card)}
          </span>
        ))}
      </div>

      <p className="text-xs text-slate-500">
        {deck.length < 2
          ? 'A deck needs at least two cards.'
          : `${deck.length} cards, up to ${MAX_DECK_SIZE}. Anything that is not a number is left out of the average.`}
      </p>
    </div>
  )
}
