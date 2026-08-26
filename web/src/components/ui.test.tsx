import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { Badge, Button, Dialog, EmptyState, ErrorNote, Field, Switch } from './ui'

describe('Button', () => {
  it('lets a caller override a default class', () => {
    render(<Button className="h-20">go</Button>)
    expect(screen.getByRole('button')).toHaveClass('h-20')
    expect(screen.getByRole('button')).not.toHaveClass('h-10')
  })

  it('does not fire while disabled', async () => {
    const onClick = vi.fn()
    render(<Button disabled onClick={onClick}>go</Button>)
    await userEvent.click(screen.getByRole('button')).catch(() => {})
    expect(onClick).not.toHaveBeenCalled()
  })
})

describe('Switch', () => {
  it('exposes its state to assistive technology', () => {
    render(<Switch checked onChange={() => {}} label="Enabled" />)
    const toggle = screen.getByRole('switch', { name: 'Enabled' })
    expect(toggle).toHaveAttribute('aria-checked', 'true')
  })

  it('toggles to the opposite value', async () => {
    const onChange = vi.fn()
    render(<Switch checked={false} onChange={onChange} label="Enabled" />)
    await userEvent.click(screen.getByRole('switch'))
    expect(onChange).toHaveBeenCalledWith(true)
  })

  // Logical properties, not left/right: the knob must sit on the correct side
  // when the interface mirrors for Hebrew.
  it('positions the knob with start/end so it mirrors in RTL', () => {
    const { container, rerender } = render(<Switch checked={false} onChange={() => {}} label="x" />)
    const knob = () => container.querySelector('span') as HTMLElement

    expect(knob().className).toContain('start-0.5')
    expect(knob().className).not.toMatch(/\bleft-|\bright-/)

    rerender(<Switch checked onChange={() => {}} label="x" />)
    expect(knob().className).toContain('start-[22px]')
    expect(knob().className).not.toMatch(/\bleft-|\bright-/)
  })
})

describe('Dialog', () => {
  it('renders nothing when closed', () => {
    render(<Dialog open={false} onClose={() => {}} title="Add"><p>body</p></Dialog>)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('is a labelled modal when open', () => {
    render(<Dialog open onClose={() => {}} title="Add contact"><p>body</p></Dialog>)
    const dialog = screen.getByRole('dialog', { name: 'Add contact' })
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    expect(screen.getByText('body')).toBeInTheDocument()
  })

  it('closes from the header button', async () => {
    const onClose = vi.fn()
    render(<Dialog open onClose={onClose} title="Add"><p>body</p></Dialog>)
    await userEvent.click(screen.getByRole('button', { name: 'close' }))
    expect(onClose).toHaveBeenCalled()
  })
})

describe('Field', () => {
  // An error must replace the hint, not sit beside it — two competing messages
  // under one input is worse than either alone.
  it('shows the hint until there is an error', () => {
    const { rerender } = render(
      <Field label="Phone" hint="digits only"><input /></Field>,
    )
    expect(screen.getByText('digits only')).toBeInTheDocument()

    rerender(<Field label="Phone" hint="digits only" error="is required"><input /></Field>)
    expect(screen.getByText('is required')).toBeInTheDocument()
    expect(screen.queryByText('digits only')).not.toBeInTheDocument()
  })
})

describe('EmptyState and ErrorNote', () => {
  it('carries a call to action rather than just a message', async () => {
    const onClick = vi.fn()
    render(<EmptyState title="No contacts yet." action={<Button onClick={onClick}>Add</Button>} />)
    expect(screen.getByText('No contacts yet.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Add' }))
    expect(onClick).toHaveBeenCalled()
  })

  it('offers a retry only when one is possible', () => {
    const { rerender } = render(<ErrorNote message="boom" />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()

    rerender(<ErrorNote message="boom" onRetry={() => {}} retryLabel="Retry" />)
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })
})

describe('Badge', () => {
  it('renders its tone', () => {
    render(<Badge tone="danger">Failed</Badge>)
    expect(screen.getByText('Failed').className).toContain('danger')
  })
})
