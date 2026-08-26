import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/lib/api'
import { I18nProvider } from '@/lib/i18n'
import Groups from './Groups'

const listGroups = vi.fn()
const availableGroups = vi.fn()
const createGroup = vi.fn()
const listWishPatterns = vi.fn()
const createWishPattern = vi.fn()
const updateWishPattern = vi.fn()
const deleteWishPattern = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      listGroups: () => listGroups(),
      availableGroups: () => availableGroups(),
      createGroup: (g: unknown) => createGroup(g),
      updateGroup: vi.fn(),
      deleteGroup: vi.fn(),
      listWishPatterns: () => listWishPatterns(),
      createWishPattern: (p: unknown) => createWishPattern(p),
      updateWishPattern: (id: number, p: unknown) => updateWishPattern(id, p),
      deleteWishPattern: (id: number) => deleteWishPattern(id),
    },
  }
})

function group(overrides: Record<string, unknown> = {}) {
  return {
    id: 1, name: 'Family', chat_id: '120363001234567890@g.us',
    language: 'he', threshold: null, enabled: true,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const renderGroups = () => render(<I18nProvider><Groups /></I18nProvider>)

beforeEach(() => {
  listGroups.mockResolvedValue([])
  availableGroups.mockResolvedValue(null)
  createGroup.mockResolvedValue(group())
  listWishPatterns.mockResolvedValue([])
  createWishPattern.mockResolvedValue({ id: 1, language: 'he', pattern: 'x', enabled: true })
  updateWishPattern.mockResolvedValue({})
  deleteWishPattern.mockResolvedValue(undefined)
})

describe('listing', () => {
  it('offers an empty state with a way to start', async () => {
    renderGroups()
    expect(await screen.findByText(/No groups watched yet/)).toBeInTheDocument()
  })

  it('shows a group and its chat id', async () => {
    listGroups.mockResolvedValue([group()])
    renderGroups()
    expect(await screen.findByText('Family')).toBeInTheDocument()
    expect(screen.getByText('120363001234567890@g.us')).toBeInTheDocument()
  })

  // A null threshold means "use the global default", which must read as that
  // rather than as a blank or a zero.
  it('spells out an unset threshold', async () => {
    listGroups.mockResolvedValue([group({ threshold: null })])
    renderGroups()
    expect(await screen.findByText(/Use the global default/)).toBeInTheDocument()
  })

  it('shows a per-group threshold when one is set', async () => {
    listGroups.mockResolvedValue([group({ threshold: 5 })])
    renderGroups()
    expect(await screen.findByText(/Threshold: 5/)).toBeInTheDocument()
  })
})

describe('the group picker', () => {
  // On a fresh install the provider cannot list groups; the UI must say so and
  // leave manual entry available rather than looking broken.
  it('falls back to manual entry when the provider cannot list groups', async () => {
    availableGroups.mockResolvedValue(null)
    renderGroups()
    await screen.findByText(/No groups watched yet/)

    await userEvent.click(screen.getAllByRole('button', { name: 'Add group' })[0])
    expect(await screen.findByText(/cannot list groups/)).toBeInTheDocument()
  })

  it('offers the provider’s groups when it can list them', async () => {
    availableGroups.mockResolvedValue([
      { chat_id: '120363001111111111@g.us', name: 'Work Crew' },
    ])
    renderGroups()
    await screen.findByText(/No groups watched yet/)

    await userEvent.click(screen.getAllByRole('button', { name: 'Add group' })[0])
    expect(await screen.findByRole('option', { name: 'Work Crew' })).toBeInTheDocument()
  })

  it('fills the chat id and name from the picked group', async () => {
    availableGroups.mockResolvedValue([
      { chat_id: '120363001111111111@g.us', name: 'Work Crew' },
    ])
    renderGroups()
    await screen.findByText(/No groups watched yet/)
    await userEvent.click(screen.getAllByRole('button', { name: 'Add group' })[0])
    await screen.findByRole('option', { name: 'Work Crew' })

    const picker = screen.getAllByRole('combobox')[0]
    await userEvent.selectOptions(picker, '120363001111111111@g.us')

    expect(screen.getByDisplayValue('120363001111111111@g.us')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Work Crew')).toBeInTheDocument()
  })
})

describe('creating a group', () => {
  it('sends null for an omitted threshold, so the global default applies', async () => {
    renderGroups()
    await screen.findByText(/No groups watched yet/)
    await userEvent.click(screen.getAllByRole('button', { name: 'Add group' })[0])

    await userEvent.type(screen.getByRole('textbox', { name: 'Name' }), 'Family')
    await userEvent.type(screen.getByRole('textbox', { name: 'Chat ID' }), '120363001234567890@g.us')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(createGroup).toHaveBeenCalled())
    expect((createGroup.mock.calls[0][0] as Record<string, unknown>).threshold).toBeNull()
  })

  it('puts a server field error under its own input', async () => {
    createGroup.mockRejectedValue(
      new ApiError(422, 'validation_failed', 'invalid', [
        { field: 'chat_id', message: 'must be a WhatsApp group id ending in @g.us' },
      ]),
    )
    renderGroups()
    await screen.findByText(/No groups watched yet/)
    await userEvent.click(screen.getAllByRole('button', { name: 'Add group' })[0])
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText(/must be a WhatsApp group id/)).toBeInTheDocument()
  })

  // A duplicate chat id is a 409, which the user can fix — it must read as a
  // message, not vanish.
  it('surfaces a conflict', async () => {
    createGroup.mockRejectedValue(
      new ApiError(409, 'conflict', 'a group with this chat id already exists', [
        { field: 'chat_id', message: 'must be unique' },
      ]),
    )
    renderGroups()
    await screen.findByText(/No groups watched yet/)
    await userEvent.click(screen.getAllByRole('button', { name: 'Add group' })[0])
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('must be unique')).toBeInTheDocument()
  })
})

describe('wish patterns', () => {
  it('lists the seeded patterns with their language', async () => {
    listWishPatterns.mockResolvedValue([
      { id: 1, language: 'he', pattern: 'מזל טוב', enabled: true },
      { id: 2, language: 'en', pattern: 'happy birthday', enabled: true },
    ])
    renderGroups()

    expect(await screen.findByText('מזל טוב')).toBeInTheDocument()
    expect(screen.getByText('happy birthday')).toBeInTheDocument()
  })

  it('adds a pattern and clears the input', async () => {
    renderGroups()
    await screen.findByText('Wish patterns')

    const input = screen.getByPlaceholderText('Pattern')
    await userEvent.type(input, 'בשעה טובה')
    await userEvent.click(screen.getByRole('button', { name: 'Add' }))

    await waitFor(() => expect(createWishPattern).toHaveBeenCalled())
    expect((createWishPattern.mock.calls[0][0] as Record<string, unknown>).pattern).toBe('בשעה טובה')
    await waitFor(() => expect((input as HTMLInputElement).value).toBe(''))
  })

  it('will not add an empty pattern', async () => {
    renderGroups()
    await screen.findByText('Wish patterns')
    expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled()
  })

  // The API rejects an uncompilable regex; the user needs to see why.
  it('reports a rejected pattern', async () => {
    createWishPattern.mockRejectedValue(
      new ApiError(422, 'validation_failed', 'one or more fields are invalid', [
        { field: 'pattern', message: 'is not a valid regular expression' },
      ]),
    )
    renderGroups()
    await screen.findByText('Wish patterns')

    // fireEvent, not userEvent.type: "[" opens a key descriptor in user-event,
    // so typing a regex would throw before it ever reached the input.
    fireEvent.change(screen.getByPlaceholderText('Pattern'), { target: { value: '/[unclosed/' } })
    await userEvent.click(screen.getByRole('button', { name: 'Add' }))

    expect(await screen.findByText(/one or more fields are invalid/)).toBeInTheDocument()
  })

  it('toggles a pattern without deleting it', async () => {
    listWishPatterns.mockResolvedValue([
      { id: 3, language: 'he', pattern: 'מזל טוב', enabled: true },
    ])
    renderGroups()
    await screen.findByText('מזל טוב')

    await userEvent.click(screen.getAllByRole('switch')[0])

    await waitFor(() => expect(updateWishPattern).toHaveBeenCalled())
    expect(updateWishPattern.mock.calls[0][0]).toBe(3)
    expect((updateWishPattern.mock.calls[0][1] as Record<string, unknown>).enabled).toBe(false)
  })
})
