import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '@/lib/api'
import { I18nProvider } from '@/lib/i18n'
import Blessings from './Blessings'

const listBlessings = vi.fn()
const createBlessing = vi.fn()
const deleteBlessing = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      listBlessings: () => listBlessings(),
      createBlessing: (b: unknown) => createBlessing(b),
      updateBlessing: vi.fn(),
      deleteBlessing: (id: number) => deleteBlessing(id),
    },
  }
})

function blessing(overrides: Record<string, unknown> = {}) {
  return {
    id: 1, event_type: 'birthday', language: 'he',
    gender: null, relation: null, text: 'מזל טוב {{name}}',
    enabled: true,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const renderBlessings = () => render(<I18nProvider><Blessings /></I18nProvider>)

/**
 * typeTemplate sets the textarea directly instead of using userEvent.type:
 * user-event reads "{{" as the escape for a literal brace, so typing
 * "{{name}}" would produce "{name}}" and quietly test the wrong thing.
 */
function typeTemplate(value: string) {
  const textarea = screen.getByRole('textbox', { name: 'Text' })
  fireEvent.change(textarea, { target: { value } })
  return textarea
}

beforeEach(() => {
  listBlessings.mockResolvedValue([])
  createBlessing.mockResolvedValue(blessing())
  deleteBlessing.mockResolvedValue(undefined)
})

describe('grouping by event type', () => {
  it('shows only the selected tab', async () => {
    listBlessings.mockResolvedValue([
      blessing({ id: 1, event_type: 'birthday', text: 'birthday one' }),
      blessing({ id: 2, event_type: 'wedding', text: 'wedding one' }),
    ])
    renderBlessings()

    expect(await screen.findByText('birthday one')).toBeInTheDocument()
    expect(screen.queryByText('wedding one')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Wedding' }))
    expect(screen.getByText('wedding one')).toBeInTheDocument()
    expect(screen.queryByText('birthday one')).not.toBeInTheDocument()
  })

  it('offers an empty state per tab, not a blank panel', async () => {
    listBlessings.mockResolvedValue([blessing({ event_type: 'birthday' })])
    renderBlessings()
    await screen.findByText('מזל טוב {{name}}')

    await userEvent.click(screen.getByRole('button', { name: 'Anniversary' }))
    expect(screen.getByText(/No templates for this event type/)).toBeInTheDocument()
  })
})

describe('targeting and group usability', () => {
  it('marks a template with no placeholder as usable for group echo', async () => {
    listBlessings.mockResolvedValue([blessing({ text: 'מזל טוב לכולם!' })])
    renderBlessings()
    expect(await screen.findByText('Usable for group echo')).toBeInTheDocument()
  })

  // A template with {{name}} cannot be posted to a group — the bot does not
  // know whose birthday it is — so the badge must not appear.
  it('does not mark a name-carrying template as group-usable', async () => {
    listBlessings.mockResolvedValue([blessing({ text: 'מזל טוב {{name}}' })])
    renderBlessings()
    await screen.findByText('מזל טוב {{name}}')
    expect(screen.queryByText('Usable for group echo')).not.toBeInTheDocument()
  })

  it('shows gender and relation targeting', async () => {
    listBlessings.mockResolvedValue([blessing({ gender: 'female', relation: 'close_friend' })])
    renderBlessings()
    expect(await screen.findByText('Female')).toBeInTheDocument()
    expect(screen.getByText('Close friend')).toBeInTheDocument()
  })

  it('flags a disabled template', async () => {
    listBlessings.mockResolvedValue([blessing({ enabled: false })])
    renderBlessings()
    expect(await screen.findByText('Disabled')).toBeInTheDocument()
  })
})

describe('the editor', () => {
  it('previews the placeholder filled in, so the author sees the real message', async () => {
    renderBlessings()
    await screen.findByText(/No templates for this event type/)

    await userEvent.click(screen.getAllByRole('button', { name: 'Add blessing' })[0])
    typeTemplate('Happy birthday {{name}}!')

    // The sample name replaces the placeholder in the preview only.
    await waitFor(() => {
      expect(screen.getByText('Happy birthday Dana!')).toBeInTheDocument()
    })
  })

  it('tells the author when their template will also work in groups', async () => {
    renderBlessings()
    await screen.findByText(/No templates for this event type/)

    await userEvent.click(screen.getAllByRole('button', { name: 'Add blessing' })[0])
    typeTemplate('Congratulations!')
    expect(await screen.findByText(/can also be posted into groups/)).toBeInTheDocument()

    typeTemplate('Congratulations {{name}}!')
    await waitFor(() => {
      expect(screen.queryByText(/can also be posted into groups/)).not.toBeInTheDocument()
    })
  })

  it('sends null, not an empty string, for untargeted fields', async () => {
    renderBlessings()
    await screen.findByText(/No templates for this event type/)

    await userEvent.click(screen.getAllByRole('button', { name: 'Add blessing' })[0])
    typeTemplate('Congratulations!')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(createBlessing).toHaveBeenCalled())
    const sent = createBlessing.mock.calls[0][0] as Record<string, unknown>
    expect(sent.gender).toBeNull()
    expect(sent.relation).toBeNull()
  })

  it('puts a server field error under its own input', async () => {
    createBlessing.mockRejectedValue(
      new ApiError(422, 'validation_failed', 'invalid', [{ field: 'text', message: 'is required' }]),
    )
    renderBlessings()
    await screen.findByText(/No templates for this event type/)

    await userEvent.click(screen.getAllByRole('button', { name: 'Add blessing' })[0])
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByText('is required')).toBeInTheDocument()
  })

  // Adding from a tab should start on that tab, not reset to birthday.
  it('defaults a new template to the tab in view', async () => {
    renderBlessings()
    await screen.findByText(/No templates for this event type/)

    await userEvent.click(screen.getByRole('button', { name: 'Wedding' }))
    await userEvent.click(screen.getAllByRole('button', { name: 'Add blessing' })[0])
    typeTemplate('Congratulations!')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(createBlessing).toHaveBeenCalled())
    expect((createBlessing.mock.calls[0][0] as Record<string, unknown>).event_type).toBe('wedding')
  })
})
