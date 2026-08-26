import { Link } from 'react-router-dom'

import { Badge, Button, Card, CardTitle, EmptyState, ErrorNote, Spinner } from '@/components/ui'
import { api } from '@/lib/api'
import type { Contact, Notice, SendLogEntry } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useApi } from '@/lib/useApi'
import { daysUntil, formatDateTime, nextOccurrence } from '@/lib/utils'

/** Live surfaces refresh on their own rather than making the user reload. */
const REFRESH_MS = 60_000
const UPCOMING_DAYS = 30

export default function Dashboard() {
  const { t } = useI18n()

  const status = useApi(() => api.providerStatus().catch(() => null), [], REFRESH_MS)
  const contacts = useApi(() => api.listContacts(), [], REFRESH_MS)
  const history = useApi(() => api.history(undefined, 10), [], REFRESH_MS)
  const notices = useApi(() => api.notices(), [], REFRESH_MS)

  return (
    <div className="space-y-4">
      <Notices items={notices.data ?? []} onDismissed={notices.reload} />
      <ProviderCard status={status.data} loading={status.loading} />
      <StatsRow contacts={contacts.data} history={history.data} />

      <Card>
        <CardTitle>{t('dashboard.upcoming')}</CardTitle>
        {contacts.loading ? (
          <Spinner label={t('common.loading')} />
        ) : contacts.error ? (
          <ErrorNote message={contacts.error.message} onRetry={contacts.reload} retryLabel={t('common.retry')} />
        ) : (
          <Upcoming contacts={contacts.data ?? []} />
        )}
      </Card>

      <Card>
        <CardTitle>{t('dashboard.recent')}</CardTitle>
        {history.loading ? (
          <Spinner label={t('common.loading')} />
        ) : (history.data ?? []).length === 0 ? (
          <EmptyState title={t('dashboard.recentEmpty')} />
        ) : (
          <ul className="divide-y">
            {(history.data ?? []).map((entry) => (
              <HistoryRow key={entry.id} entry={entry} />
            ))}
          </ul>
        )}
      </Card>
    </div>
  )
}

/**
 * Notices surfaces conditions the operator needs to act on — currently a
 * language fallback, where a blessing went out in English because the contact's
 * own language had no template (spec §6). A log line is invisible to someone
 * using the web UI, and the condition recurs every year until it is fixed.
 */
function Notices({ items, onDismissed }: { items: Notice[]; onDismissed: () => void }) {
  const { t } = useI18n()
  if (items.length === 0) return null

  const dismiss = async (id: number) => {
    await api.dismissNotice(id)
    onDismissed()
  }

  return (
    <Card className="border-warning/50 bg-warning/10">
      <CardTitle>{t('notices.title')}</CardTitle>
      <ul className="space-y-3">
        {items.map((n) => (
          <li key={n.id} className="flex flex-wrap items-start gap-2">
            <div className="min-w-0 grow">
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone={n.level === 'error' ? 'danger' : 'warning'}>
                  {t(`notices.${n.code}`)}
                </Badge>
                {n.occurrences > 1 ? (
                  <span className="text-xs text-muted-foreground">
                    {t('notices.seenTimes', { n: n.occurrences })}
                  </span>
                ) : null}
              </div>
              <p className="mt-1 text-sm">{n.message}</p>
              {n.detail ? (
                <p className="mt-0.5 text-xs text-muted-foreground">{n.detail}</p>
              ) : null}
            </div>
            <Button variant="ghost" size="sm" onClick={() => void dismiss(n.id)}>
              {t('notices.dismiss')}
            </Button>
          </li>
        ))}
      </ul>
    </Card>
  )
}

function ProviderCard({
  status,
  loading,
}: {
  status: Awaited<ReturnType<typeof api.providerStatus>> | null | undefined
  loading: boolean
}) {
  const { t } = useI18n()

  if (loading) {
    return (
      <Card>
        <Spinner label={t('common.loading')} />
      </Card>
    )
  }

  // A null status means the request failed — on a fresh install that is simply
  // "no provider yet", which is a call to action rather than an error.
  if (!status) {
    return (
      <Card>
        <div className="flex flex-wrap items-center gap-3">
          <Badge tone="warning">{t('dashboard.notConfigured')}</Badge>
          <Link className="text-sm text-primary underline" to="/settings">
            {t('dashboard.configure')}
          </Link>
        </div>
      </Card>
    )
  }

  const tone = status.connected ? 'success' : status.needs_qr ? 'warning' : 'danger'
  const label = status.connected
    ? t('dashboard.connected')
    : status.needs_qr
      ? t('dashboard.needsQR')
      : t('dashboard.disconnected')

  return (
    <Card>
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm font-medium">{t('dashboard.provider')}</span>
        <span className="text-sm text-muted-foreground">{status.provider}</span>
        <Badge tone={tone}>{label}</Badge>
        {status.state ? <span className="text-xs text-muted-foreground">{status.state}</span> : null}
        {status.needs_qr ? (
          <Link className="text-sm text-primary underline" to="/settings">
            {t('dashboard.configure')}
          </Link>
        ) : null}
      </div>
    </Card>
  )
}

function StatsRow({ contacts, history }: { contacts?: Contact[]; history?: SendLogEntry[] }) {
  const { t } = useI18n()
  const today = new Date().toDateString()
  const todays = (history ?? []).filter((e) => new Date(e.sent_at).toDateString() === today)

  const stats = [
    { label: t('dashboard.contacts'), value: (contacts ?? []).filter((c) => c.enabled).length },
    { label: t('dashboard.sentToday'), value: todays.filter((e) => e.status === 'sent').length },
    { label: t('dashboard.failedToday'), value: todays.filter((e) => e.status === 'failed').length },
  ]

  return (
    <div className="grid grid-cols-3 gap-3">
      {stats.map((s) => (
        <Card key={s.label} className="text-center">
          <div className="text-2xl font-semibold">{s.value}</div>
          <div className="mt-1 text-xs text-muted-foreground">{s.label}</div>
        </Card>
      ))}
    </div>
  )
}

function Upcoming({ contacts }: { contacts: Contact[] }) {
  const { t } = useI18n()

  const upcoming = contacts
    .filter((c) => c.enabled)
    .map((c) => ({ contact: c, days: daysUntil(nextOccurrence(c.event_date)) }))
    .filter((row) => row.days >= 0 && row.days <= UPCOMING_DAYS)
    .sort((a, b) => a.days - b.days)

  if (upcoming.length === 0) {
    return <EmptyState title={t('dashboard.upcomingEmpty')} />
  }

  return (
    <ul className="divide-y">
      {upcoming.map(({ contact, days }) => (
        <li key={contact.id} className="flex items-center gap-3 py-2.5">
          <span className="grow truncate font-medium">{contact.name}</span>
          <Badge>{t(`event.${contact.event_type}`)}</Badge>
          <span className="shrink-0 text-sm text-muted-foreground">
            {days === 0
              ? t('dashboard.today')
              : days === 1
                ? t('dashboard.tomorrow')
                : t('dashboard.inDays', { n: days })}
          </span>
        </li>
      ))}
    </ul>
  )
}

function HistoryRow({ entry }: { entry: SendLogEntry }) {
  const { t } = useI18n()
  return (
    <li className="flex flex-wrap items-center gap-2 py-2.5 text-sm">
      <Badge tone={entry.status === 'sent' ? 'success' : 'danger'}>{t(`status.${entry.status}`)}</Badge>
      <span className="text-muted-foreground">{t(`kind.${entry.kind}`)}</span>
      <span className="truncate font-mono text-xs text-muted-foreground" dir="ltr">
        {entry.chat_id}
      </span>
      <span className="ms-auto shrink-0 text-xs text-muted-foreground" dir="ltr">
        {formatDateTime(entry.sent_at)}
      </span>
      {entry.error ? <span className="w-full truncate text-xs text-danger">{entry.error}</span> : null}
    </li>
  )
}
