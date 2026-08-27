import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { I18nProvider } from '@/lib/i18n'
import { ThemeProvider } from '@/lib/theme'
import Settings from './Settings'

const getSettings = vi.fn()
const saveSettings = vi.fn()
const providerStatus = vi.fn()
const providerTest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      getSettings: () => getSettings(),
      saveSettings: (s: unknown) => saveSettings(s),
      providerStatus: () => providerStatus(),
      providerTest: (phone: string, message?: string) => providerTest(phone, message),
    },
  }
})

const MASK = '••••'

function settings(overrides: Record<string, unknown> = {}) {
  return {
    provider: 'greenapi',
    greenapi: {
      api_url: 'https://api.green-api.com',
      id_instance: '7103123456',
      api_token: MASK, // set, and masked by the server
      mode: 'polling',
      webhook_auth_header: '',
    },
    gowa: { base_url: '', username: '', password: '', device_id: '', webhook_secret: '' },
    scheduler: { timezone: 'Asia/Jerusalem', send_time: '09:00' },
    group_echo: { threshold: 3, window_hours: 6, cooldown_hours: 20 },
    ...overrides,
  }
}

const renderSettings = () =>
  render(
    <ThemeProvider>
      <I18nProvider>
        <Settings />
      </I18nProvider>
    </ThemeProvider>,
  )

beforeEach(() => {
  getSettings.mockResolvedValue(settings())
  saveSettings.mockImplementation(async (s: Record<string, unknown>) => s)
  providerStatus.mockResolvedValue({ provider: 'gowa', state: 'connected', connected: true })
  providerTest.mockResolvedValue({
    provider: 'gowa', chat_id: '972500000001@c.us', message_id: 'ABC123',
  })
})

describe('secret handling', () => {
  it('shows the mask for a stored secret, never a value', async () => {
    renderSettings()
    const token = await screen.findByDisplayValue(MASK)
    expect(token).toBeInTheDocument()
  })

  // The round-trip contract: sending the mask back means "leave it alone". If
  // the form mangled it, saving an unrelated field would destroy the credential.
  it('sends the mask back untouched when the field is not edited', async () => {
    renderSettings()
    await screen.findByDisplayValue(MASK)

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(saveSettings).toHaveBeenCalled())
    const sent = saveSettings.mock.calls[0][0] as ReturnType<typeof settings>
    expect(sent.greenapi.api_token).toBe(MASK)
  })

  // Typing must replace the mask, not append to it — "••••newtoken" would be
  // stored verbatim and every provider call would fail.
  it('clears the mask on focus so typing replaces it', async () => {
    renderSettings()
    const token = await screen.findByDisplayValue(MASK)

    await userEvent.click(token)
    await userEvent.type(token, 'brand-new-token')

    expect((token as HTMLInputElement).value).toBe('brand-new-token')
  })

  it('never puts provider_error in the payload it sends back', async () => {
    getSettings.mockResolvedValue(settings({ provider_error: 'instance id is required' }))
    renderSettings()
    await screen.findByDisplayValue(MASK)

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(saveSettings).toHaveBeenCalled())

    expect(saveSettings.mock.calls[0][0]).not.toHaveProperty('provider_error')
  })
})

describe('provider fields', () => {
  it('shows GreenAPI fields when GreenAPI is selected', async () => {
    renderSettings()
    expect(await screen.findByDisplayValue('7103123456')).toBeInTheDocument()
  })

  it('swaps the field set when the provider changes', async () => {
    renderSettings()
    await screen.findByDisplayValue('7103123456')

    const selects = screen.getAllByRole('combobox')
    await userEvent.selectOptions(selects[2], 'gowa')

    // The GreenAPI instance field is gone; GOWA's own fields are shown.
    expect(screen.queryByDisplayValue('7103123456')).not.toBeInTheDocument()
    expect(screen.getByText('Base URL')).toBeInTheDocument()
    expect(screen.getByText(/Webhook secret/)).toBeInTheDocument()
  })
})

describe('feedback', () => {
  it('reports a save', async () => {
    renderSettings()
    await screen.findByDisplayValue(MASK)

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Settings saved.')).toBeInTheDocument()
  })

  // A valid save whose provider rebuild failed still succeeded; the UI must say
  // what went wrong rather than claiming everything is fine.
  it('surfaces a provider error returned alongside a successful save', async () => {
    saveSettings.mockResolvedValue(settings({ provider_error: 'instance id is not a number' }))
    renderSettings()
    await screen.findByDisplayValue(MASK)

    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('instance id is not a number')).toBeInTheDocument()
  })

  it('shows a load failure with a way to retry', async () => {
    getSettings.mockRejectedValue(new Error('database is unreachable'))
    renderSettings()
    expect(await screen.findByText('database is unreachable')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })
})

// Spec §9: the settings page must be able to prove the stored configuration
// works — a connection check and a real message — without saving first.
describe('provider tools', () => {
  it('reports the live provider state', async () => {
    renderSettings()
    await userEvent.click(await screen.findByRole('button', { name: 'Test connection' }))

    expect(providerStatus).toHaveBeenCalled()
    expect(await screen.findByText('gowa: connected ✓')).toBeInTheDocument()
  })

  it('surfaces a failed connection check instead of staying silent', async () => {
    providerStatus.mockRejectedValue(new Error('no provider configured'))
    renderSettings()
    await userEvent.click(await screen.findByRole('button', { name: 'Test connection' }))

    expect(await screen.findByText('no provider configured')).toBeInTheDocument()
  })

  it('keeps the send button disabled until a destination is entered', async () => {
    renderSettings()
    const send = await screen.findByRole('button', { name: 'Send test message' })
    expect(send).toBeDisabled()

    await userEvent.type(screen.getByPlaceholderText('Phone or chat ID'), '972500000001')
    expect(send).toBeEnabled()
  })

  it('sends a test message to the number given and shows where it landed', async () => {
    renderSettings()
    await userEvent.type(
      await screen.findByPlaceholderText('Phone or chat ID'), '972500000001')
    await userEvent.click(screen.getByRole('button', { name: 'Send test message' }))

    expect(providerTest).toHaveBeenCalledWith('972500000001', undefined)
    expect(await screen.findByText('972500000001@c.us → ABC123')).toBeInTheDocument()
  })
})

describe('conditional fields', () => {
  // Polling needs no inbound URL, so the auth header only makes sense — and is
  // only offered — once the user switches GreenAPI to webhook mode.
  it('offers the webhook auth header only in webhook mode', async () => {
    renderSettings()
    await screen.findByLabelText('Incoming messages')
    expect(screen.queryByLabelText('Webhook auth header')).not.toBeInTheDocument()

    await userEvent.selectOptions(screen.getByLabelText('Incoming messages'), 'webhook')
    expect(screen.getByLabelText('Webhook auth header')).toBeInTheDocument()
  })

  it('saves an edited group-echo threshold', async () => {
    renderSettings()
    const threshold = await screen.findByLabelText('Threshold')
    await userEvent.clear(threshold)
    await userEvent.type(threshold, '5')
    await userEvent.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => expect(saveSettings).toHaveBeenCalled())
    const sent = saveSettings.mock.calls[0][0] as { group_echo: { threshold: number } }
    expect(sent.group_echo.threshold).toBe(5)
  })
})
