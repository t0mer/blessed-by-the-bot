import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { I18nProvider } from '@/lib/i18n'
import Dashboard from './Dashboard'

const listContacts = vi.fn()
const providerStatus = vi.fn()
const history = vi.fn()
const notices = vi.fn()
const dismissNotice = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      listContacts: () => listContacts(),
      providerStatus: () => providerStatus(),
      history: (...args: unknown[]) => history(...args),
      notices: () => notices(),
      dismissNotice: (id: number) => dismissNotice(id),
    },
  }
})

function notice(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    key: 'language_fallback:birthday:ru',
    level: 'warning',
    code: 'language_fallback',
    message: 'No birthday blessing in "ru"; using the en template instead.',
    detail: 'Add a birthday template in "ru" under Blessings.',
    occurrences: 1,
    first_seen_at: '2026-08-26T08:00:00Z',
    last_seen_at: '2026-08-26T08:00:00Z',
    dismissed_at: null,
    ...overrides,
  }
}

// The provider status card links to Settings, so a router context is required.
const renderDashboard = () =>
  render(
    <MemoryRouter>
      <I18nProvider>
        <Dashboard />
      </I18nProvider>
    </MemoryRouter>,
  )

beforeEach(() => {
  listContacts.mockResolvedValue([])
  providerStatus.mockRejectedValue(new Error('no provider'))
  history.mockResolvedValue([])
  notices.mockResolvedValue([])
  dismissNotice.mockResolvedValue(undefined)
})

describe('notices banner', () => {
  it('stays out of the way when there is nothing to report', async () => {
    renderDashboard()
    await waitFor(() => expect(notices).toHaveBeenCalled())
    expect(screen.queryByText('Needs your attention')).not.toBeInTheDocument()
  })

  it('shows the message and what to do about it', async () => {
    notices.mockResolvedValue([notice()])
    renderDashboard()

    expect(await screen.findByText('Needs your attention')).toBeInTheDocument()
    expect(screen.getByText(/No birthday blessing in "ru"/)).toBeInTheDocument()
    // The detail is the actionable half; a message without it is just noise.
    expect(screen.getByText(/Add a birthday template/)).toBeInTheDocument()
  })

  it('counts repeats rather than listing them', async () => {
    notices.mockResolvedValue([notice({ occurrences: 7 })])
    renderDashboard()
    expect(await screen.findByText('seen 7 times')).toBeInTheDocument()
  })

  it('hides the repeat count for a one-off', async () => {
    notices.mockResolvedValue([notice({ occurrences: 1 })])
    renderDashboard()
    await screen.findByText('Needs your attention')
    expect(screen.queryByText(/seen \d+ times/)).not.toBeInTheDocument()
  })

  it('dismisses and reloads', async () => {
    notices.mockResolvedValue([notice()])
    renderDashboard()
    await screen.findByText('Needs your attention')

    notices.mockResolvedValue([])
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }))

    expect(dismissNotice).toHaveBeenCalledWith(1)
    await waitFor(() => {
      expect(screen.queryByText('Needs your attention')).not.toBeInTheDocument()
    })
  })
})

describe('provider card', () => {
  // A failed status call on a fresh install means "not set up yet", which is a
  // call to action rather than an error.
  it('invites configuration when no provider answers', async () => {
    renderDashboard()
    expect(await screen.findByText('Not configured')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Configure it in Settings/ })).toBeInTheDocument()
  })

  it('reports a healthy connection', async () => {
    providerStatus.mockResolvedValue({
      provider: 'gowa', connected: true, state: 'authorized', needs_qr: false,
    })
    renderDashboard()
    expect(await screen.findByText('Connected')).toBeInTheDocument()
  })

  // Needing a QR scan is a normal state, not a failure.
  it('surfaces a QR-scan requirement with a way to act on it', async () => {
    providerStatus.mockResolvedValue({
      provider: 'gowa', connected: false, state: 'notLoggedIn', needs_qr: true,
    })
    renderDashboard()
    expect(await screen.findByText('Needs QR scan')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Configure it in Settings/ })).toBeInTheDocument()
  })
})

describe('upcoming events', () => {
  const contact = (overrides: Record<string, unknown> = {}) => ({
    id: 1, name: 'Dana', phone: '972501234567',
    event_date: '1990-05-17', event_type: 'birthday', language: 'he',
    relation: 'friend', importance: 3, gender: 'female',
    send_time: null, enabled: true,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  })

  it('offers a real empty state rather than a blank panel', async () => {
    renderDashboard()
    expect(await screen.findByText('No events in the next 30 days.')).toBeInTheDocument()
  })

  it("labels today's event as Today", async () => {
    const today = new Date()
    const md = `${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
    listContacts.mockResolvedValue([contact({ event_date: `1990-${md}` })])

    renderDashboard()
    expect(await screen.findByText('Dana')).toBeInTheDocument()
    expect(screen.getByText('Today')).toBeInTheDocument()
  })

  // A disabled contact is muted on purpose; showing it as upcoming would imply
  // a message that will never be sent.
  it('leaves disabled contacts out', async () => {
    const today = new Date()
    const md = `${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
    listContacts.mockResolvedValue([contact({ event_date: `1990-${md}`, enabled: false })])

    renderDashboard()
    expect(await screen.findByText('No events in the next 30 days.')).toBeInTheDocument()
  })
})
