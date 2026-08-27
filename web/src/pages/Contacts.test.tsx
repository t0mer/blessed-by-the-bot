import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/lib/api'
import { I18nProvider } from '@/lib/i18n'
import Contacts from './Contacts'

const listContacts = vi.fn()
const createContact = vi.fn()
const updateContact = vi.fn()
const deleteContact = vi.fn()
const sendNow = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      listContacts: () => listContacts(),
      createContact: (c: unknown) => createContact(c),
      updateContact: (id: number, c: unknown) => updateContact(id, c),
      deleteContact: (id: number) => deleteContact(id),
      sendNow: (id: number) => sendNow(id),
    },
  }
})

function contact(overrides: Record<string, unknown> = {}) {
  return {
    id: 1, name: 'Dana', phone: '972501234567',
    event_date: '1990-05-17', event_type: 'birthday', language: 'he',
    relation: 'friend', importance: 3, gender: 'female',
    send_time: null, enabled: true,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const renderContacts = () =>
  render(<I18nProvider><Contacts /></I18nProvider>)

beforeEach(() => {
  listContacts.mockResolvedValue([])
  createContact.mockResolvedValue(contact())
  updateContact.mockResolvedValue(contact())
  deleteContact.mockResolvedValue(undefined)
  sendNow.mockResolvedValue({})
})

describe('listing', () => {
  it('offers an empty state with a way to start', async () => {
    renderContacts()
    expect(await screen.findByText(/No contacts yet/)).toBeInTheDocument()
    // Two: the header button and the empty-state call to action.
    expect(screen.getAllByRole('button', { name: 'Add contact' }).length).toBeGreaterThan(0)
  })

  it('shows a contact with its details', async () => {
    listContacts.mockResolvedValue([contact()])
    renderContacts()

    expect(await screen.findByText('Dana')).toBeInTheDocument()
    expect(screen.getByText('972501234567')).toBeInTheDocument()
    // Dates render DD/MM/YYYY, not the ISO form the API returns.
    expect(screen.getByText('17/05/1990')).toBeInTheDocument()
  })

  it('marks a disabled contact', async () => {
    listContacts.mockResolvedValue([contact({ enabled: false })])
    renderContacts()
    expect(await screen.findByText('Disabled')).toBeInTheDocument()
  })
})

describe('search and filter', () => {
  beforeEach(() => {
    listContacts.mockResolvedValue([
      contact({ id: 1, name: 'Dana', phone: '972501111111' }),
      contact({ id: 2, name: 'Yossi', phone: '972502222222', event_type: 'wedding' }),
    ])
  })

  it('filters by name', async () => {
    renderContacts()
    await screen.findByText('Dana')

    await userEvent.type(screen.getByPlaceholderText('Search'), 'yos')
    expect(screen.queryByText('Dana')).not.toBeInTheDocument()
    expect(screen.getByText('Yossi')).toBeInTheDocument()
  })

  it('filters by phone, which is how you find someone you did not name well', async () => {
    renderContacts()
    await screen.findByText('Dana')

    await userEvent.type(screen.getByPlaceholderText('Search'), '2222')
    expect(screen.getByText('Yossi')).toBeInTheDocument()
    expect(screen.queryByText('Dana')).not.toBeInTheDocument()
  })

  it('says so when nothing matches, rather than looking empty', async () => {
    renderContacts()
    await screen.findByText('Dana')

    await userEvent.type(screen.getByPlaceholderText('Search'), 'nobody')
    expect(screen.getByText(/No contacts match/)).toBeInTheDocument()
  })

  it('filters by event type', async () => {
    renderContacts()
    await screen.findByText('Dana')

    await userEvent.selectOptions(screen.getByRole('combobox'), 'wedding')
    expect(screen.getByText('Yossi')).toBeInTheDocument()
    expect(screen.queryByText('Dana')).not.toBeInTheDocument()
  })
})

describe('validation errors', () => {
  // The contract with the API: a `fields` entry must land under its own input,
  // not in a generic banner the user has to map back to a form control.
  it('puts a field error under the field it names', async () => {
    createContact.mockRejectedValue(
      new ApiError(422, 'validation_failed', 'one or more fields are invalid', [
        { field: 'phone', message: 'must be an international number' },
        { field: 'name', message: 'is required' },
      ]),
    )
    renderContacts()
    await screen.findByText(/No contacts yet/)

    await userEvent.click(screen.getAllByRole('button', { name: 'Add contact' })[0])
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(screen.getByText('must be an international number')).toBeInTheDocument()
    })
    expect(screen.getByText('is required')).toBeInTheDocument()
    // The hint it replaced is gone, so there is one message per input.
    expect(screen.queryByText(/International format, digits only/)).not.toBeInTheDocument()
  })

  it('shows a non-field failure as a banner', async () => {
    createContact.mockRejectedValue(new ApiError(500, 'internal_error', 'internal server error'))
    renderContacts()
    await screen.findByText(/No contacts yet/)

    await userEvent.click(screen.getAllByRole('button', { name: 'Add contact' })[0])
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('internal server error')).toBeInTheDocument()
  })
})

describe('send now', () => {
  it('reports a failure instead of silently doing nothing', async () => {
    listContacts.mockResolvedValue([contact()])
    sendNow.mockRejectedValue(
      new ApiError(409, 'already_sent', 'this contact already received a blessing this year'),
    )
    renderContacts()
    await screen.findByText('Dana')

    await userEvent.click(screen.getByRole('button', { name: 'Send now' }))
    expect(await screen.findByText(/already received a blessing/)).toBeInTheDocument()
  })

  it('confirms a success', async () => {
    listContacts.mockResolvedValue([contact()])
    renderContacts()
    await screen.findByText('Dana')

    await userEvent.click(screen.getByRole('button', { name: 'Send now' }))
    expect(await screen.findByText('Blessing sent.')).toBeInTheDocument()
    expect(sendNow).toHaveBeenCalledWith(1)
  })
})

describe('the add and edit dialog', () => {
  const openAddDialog = async () => {
    renderContacts()
    await userEvent.click(await screen.findByRole('button', { name: 'Add contact' }))
  }

  it('sends a null send time when no custom one is set, so the contact follows the general time', async () => {
    await openAddDialog()
    await userEvent.type(screen.getByLabelText('Name'), 'Noa')
    await userEvent.type(screen.getByLabelText('Phone'), '972500000002')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(createContact).toHaveBeenCalled())
    const sent = createContact.mock.calls[0][0] as Record<string, unknown>
    expect(sent).toMatchObject({ name: 'Noa', phone: '972500000002', send_time: null })
  })

  it('sends the custom send time once the switch is on', async () => {
    await openAddDialog()
    await userEvent.type(screen.getByLabelText('Name'), 'Noa')
    await userEvent.type(screen.getByLabelText('Phone'), '972500000002')
    await userEvent.click(screen.getByRole('switch', { name: 'Use a custom send time' }))

    const time = screen.getByLabelText('Send time')
    await userEvent.clear(time)
    await userEvent.type(time, '07:30')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(createContact).toHaveBeenCalled())
    expect((createContact.mock.calls[0][0] as { send_time: string }).send_time).toBe('07:30')
  })

  // Editing must update in place: creating a second Dana every time someone
  // fixes a typo is the failure this guards against.
  it('updates the existing contact rather than creating another', async () => {
    listContacts.mockResolvedValue([contact()])
    renderContacts()
    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }))

    const name = screen.getByLabelText('Name')
    expect(name).toHaveValue('Dana')
    await userEvent.clear(name)
    await userEvent.type(name, 'Dana Levi')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(updateContact).toHaveBeenCalled())
    expect(updateContact.mock.calls[0][0]).toBe(1)
    expect((updateContact.mock.calls[0][1] as { name: string }).name).toBe('Dana Levi')
    expect(createContact).not.toHaveBeenCalled()
  })

  it('deletes a contact and refreshes the list once the prompt is accepted', async () => {
    vi.stubGlobal('confirm', vi.fn(() => true))
    listContacts.mockResolvedValue([contact()])
    renderContacts()
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(deleteContact).toHaveBeenCalledWith(1))
    expect(listContacts.mock.calls.length).toBeGreaterThan(1)
  })

  // Deleting is irreversible, so the prompt has to be a real gate, not a formality.
  it('keeps the contact when the prompt is declined', async () => {
    vi.stubGlobal('confirm', vi.fn(() => false))
    listContacts.mockResolvedValue([contact()])
    renderContacts()
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))

    expect(deleteContact).not.toHaveBeenCalled()
  })
})
