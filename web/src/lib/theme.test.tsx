import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ThemeProvider, useTheme } from './theme'

function Probe() {
  const { theme, resolved, setTheme } = useTheme()
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="resolved">{resolved}</span>
      <button onClick={() => setTheme('dark')}>dark</button>
      <button onClick={() => setTheme('system')}>system</button>
    </div>
  )
}

const renderProbe = () => render(<ThemeProvider><Probe /></ThemeProvider>)

/** stubSystemDark makes matchMedia report a dark system preference. */
function stubSystemDark(dark: boolean) {
  vi.stubGlobal('matchMedia', vi.fn().mockImplementation((query: string) => ({
    matches: dark,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })))
}

describe('theme', () => {
  it('follows the system by default', () => {
    renderProbe()
    expect(screen.getByTestId('theme').textContent).toBe('system')
    expect(screen.getByTestId('resolved').textContent).toBe('light')
  })

  it('resolves to dark when the system prefers it', () => {
    stubSystemDark(true)
    renderProbe()
    expect(screen.getByTestId('resolved').textContent).toBe('dark')
  })

  it('lets an explicit choice override the system', async () => {
    stubSystemDark(false)
    renderProbe()
    await userEvent.click(screen.getByRole('button', { name: 'dark' }))

    expect(screen.getByTestId('resolved').textContent).toBe('dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('remembers the choice', async () => {
    renderProbe()
    await userEvent.click(screen.getByRole('button', { name: 'dark' }))
    expect(localStorage.getItem('blessedbot.theme')).toBe('dark')
  })

  it('restores a stored choice on mount', () => {
    localStorage.setItem('blessedbot.theme', 'dark')
    renderProbe()
    expect(screen.getByTestId('resolved').textContent).toBe('dark')
  })

  it('ignores a corrupt stored value', () => {
    localStorage.setItem('blessedbot.theme', 'neon')
    renderProbe()
    expect(screen.getByTestId('theme').textContent).toBe('system')
  })

  // Going back to "system" must drop the class so the media query governs again.
  it('removes the dark class when returning to system on a light system', async () => {
    stubSystemDark(false)
    renderProbe()
    await userEvent.click(screen.getByRole('button', { name: 'dark' }))
    expect(document.documentElement.classList.contains('dark')).toBe(true)

    await userEvent.click(screen.getByRole('button', { name: 'system' }))
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})
