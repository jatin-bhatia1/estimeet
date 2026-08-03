import { useAppConfig } from '../lib/useAppConfig'

/**
 * AppFooter closes every page with why this exists and where to complain. The
 * email is only rendered when the operator set one, so a public deployment does
 * not publish an address by accident.
 */
export function AppFooter() {
  const config = useAppConfig()
  const issuesUrl = config?.issuesUrl ?? 'https://github.com/jatin-bhatia1/estimeet/issues'

  return (
    <footer className="mt-16 px-5 pb-8">
      <div className="relative mx-auto max-w-3xl overflow-hidden rounded-2xl border border-white/10 bg-gradient-to-br from-white/[0.06] via-white/[0.02] to-transparent p-6 text-center">
        {/* A soft glow behind the card so the footer reads as a closing note
            rather than leftover space at the bottom of the page. */}
        <div
          aria-hidden
          className="pointer-events-none absolute -top-24 left-1/2 h-48 w-96 -translate-x-1/2 rounded-full bg-accent-500/10 blur-3xl"
        />

        <div className="relative">
          <p className="text-sm font-semibold tracking-tight text-slate-100">
            Built for fun, to make estimation less painful.
          </p>
          <p className="mx-auto mt-1.5 max-w-md text-xs leading-relaxed text-slate-500">
            No accounts, no tracking, no meeting that ends with “let’s just say five”.
          </p>

          <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
            {config?.contactEmail && (
              <a
                href={`mailto:${config.contactEmail}`}
                className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-4 py-1.5 text-xs font-medium text-slate-300 transition hover:border-accent-500/40 hover:bg-white/10 hover:text-accent-400"
              >
                Share an idea
              </a>
            )}
            <a
              href={issuesUrl}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-4 py-1.5 text-xs font-medium text-slate-300 transition hover:border-accent-500/40 hover:bg-white/10 hover:text-accent-400"
            >
              Report a problem
            </a>
          </div>

          {config && config.roomRetentionDays > 0 && (
            <p className="mt-4 text-[11px] text-slate-600">
              Sessions are kept for {config.roomRetentionDays} days after the last activity, then deleted.
            </p>
          )}
        </div>
      </div>
    </footer>
  )
}
