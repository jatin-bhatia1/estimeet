import type { ReactNode } from 'react'

import { cardLabel } from '../lib/types'

const SIZES = {
  sm: 'h-12 w-9 text-sm',
  md: 'h-20 w-14 text-lg',
  lg: 'h-28 w-20 text-2xl',
} as const

interface PlayingCardProps {
  value: string
  size?: keyof typeof SIZES
  selected?: boolean
  disabled?: boolean
  faceDown?: boolean
  onClick?: () => void
  footer?: ReactNode
}

/** PlayingCard renders one card of the room's deck, face up or face down. */
export function PlayingCard({
  value,
  size = 'md',
  selected = false,
  disabled = false,
  faceDown = false,
  onClick,
  footer,
}: PlayingCardProps) {
  const classes = [
    'relative flex flex-col items-center justify-center rounded-xl border font-semibold transition',
    SIZES[size],
    faceDown
      ? 'border-white/10 bg-[repeating-linear-gradient(45deg,rgba(255,255,255,0.05)_0px,rgba(255,255,255,0.05)_6px,transparent_6px,transparent_12px)] text-transparent'
      : selected
        ? 'border-accent-500 bg-accent-500 text-slate-950 shadow-lg shadow-sky-500/20'
        : 'border-white/15 bg-white/5 text-slate-100',
    onClick && !disabled ? 'cursor-pointer hover:-translate-y-1 hover:border-accent-500/60' : '',
    disabled && onClick ? 'opacity-40' : '',
  ].join(' ')

  const content = faceDown ? '•' : cardLabel(value)

  if (!onClick) {
    return (
      <div className={classes} aria-label={faceDown ? 'Hidden card' : `Card ${cardLabel(value)}`}>
        <span>{content}</span>
        {footer}
      </div>
    )
  }

  return (
    <button type="button" className={classes} onClick={onClick} disabled={disabled} aria-pressed={selected}>
      <span>{content}</span>
      {footer}
    </button>
  )
}

interface DeckProps {
  deck: string[]
  myVote: string | null
  disabled?: boolean
  size?: keyof typeof SIZES
  onPick: (value: string) => void
  onClear?: () => void
}

/** Deck is the row of cards a player picks from. */
export function Deck({ deck, myVote, disabled = false, size = 'md', onPick, onClear }: DeckProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {deck.map((value) => (
        <PlayingCard
          key={value}
          value={value}
          size={size}
          selected={myVote === value}
          disabled={disabled}
          onClick={() => onPick(value)}
        />
      ))}
      {myVote && onClear && !disabled && (
        <button type="button" onClick={onClear} className="ml-1 text-xs text-slate-400 underline hover:text-slate-200">
          clear
        </button>
      )}
    </div>
  )
}
