import { useAppConfig } from '../lib/useAppConfig'

/**
 * AppFooter closes every page with why this exists and where to complain. The
 * email is only rendered when the operator set one, so a public deployment does
 * not publish an address by accident.
 */
export function AppFooter() {
  const config = useAppConfig()

  return (
    <footer className="mt-10 border-t border-white/5 px-5 py-6">
      <p className="mx-auto max-w-3xl text-center text-xs leading-relaxed text-slate-500">
        I built this app for fun, to make Scrum estimation less painful.
        {config?.contactEmail && (
          <>
            {' '}
            Feedback or an idea to share? Ping me at{' '}
            <a href={`mailto:${config.contactEmail}`} className="text-accent-400 hover:underline">
              {config.contactEmail}
            </a>
            .
          </>
        )}{' '}
        Found a problem?{' '}
        <a
          href={config?.issuesUrl ?? 'https://github.com/jatin-bhatia1/estimeet/issues'}
          target="_blank"
          rel="noreferrer noopener"
          className="text-accent-400 hover:underline"
        >
          Open an issue
        </a>
        .
      </p>
    </footer>
  )
}
