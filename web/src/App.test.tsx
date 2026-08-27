import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

import { I18nProvider } from '@/lib/i18n'
import { ThemeProvider } from '@/lib/theme'
import App from './App'

// The shell is what is under test here — routing, the two navigations and the
// language and theme toggles. Each page has its own suite, so stub them rather
// than standing up five pages' worth of API mocks.
vi.mock('@/pages/Dashboard', () => ({ default: () => <p>dashboard page</p> }))
vi.mock('@/pages/Contacts', () => ({ default: () => <p>contacts page</p> }))
vi.mock('@/pages/Blessings', () => ({ default: () => <p>blessings page</p> }))
vi.mock('@/pages/Groups', () => ({ default: () => <p>groups page</p> }))
vi.mock('@/pages/Settings', () => ({ default: () => <p>settings page</p> }))

function renderApp(path = '/') {
  return render(
    <ThemeProvider>
      <I18nProvider>
        <MemoryRouter initialEntries={[path]}>
          <App />
        </MemoryRouter>
      </I18nProvider>
    </ThemeProvider>,
  )
}

// Both navigations render every destination — a bar for the desktop and one for
// the thumb — so each label matches twice. Clicking either is the same route.
const navLink = (name: RegExp) => screen.getAllByRole('link', { name })[0]

describe('routing', () => {
  it('shows the dashboard at the root', () => {
    renderApp()
    expect(screen.getByText('dashboard page')).toBeInTheDocument()
  })

  it.each([
    [/contacts/i, 'contacts page'],
    [/blessings/i, 'blessings page'],
    [/groups/i, 'groups page'],
    [/settings/i, 'settings page'],
  ])('navigates to %s', async (label, expected) => {
    renderApp()
    await userEvent.click(navLink(label))
    expect(screen.getByText(expected)).toBeInTheDocument()
  })

  it('sends an unknown path back to the dashboard', () => {
    renderApp('/no-such-page')
    expect(screen.getByText('dashboard page')).toBeInTheDocument()
  })
})

describe('toggles', () => {
  it('switches the interface to Hebrew and flips the document direction', async () => {
    renderApp()
    expect(document.documentElement.dir).toBe('ltr')

    await userEvent.click(screen.getByRole('button', { name: /Interface language|שפת הממשק/ }))

    expect(document.documentElement.dir).toBe('rtl')
    expect(document.documentElement.lang).toBe('he')
    // The button now offers the way back.
    expect(screen.getByRole('button', { name: /Interface language|שפת הממשק/ })).toHaveTextContent('EN')
  })

  it('turns dark mode on and off', async () => {
    renderApp()
    const toggle = () => screen.getByRole('button', { name: /Theme|ערכת נושא/ })

    await userEvent.click(toggle())
    expect(document.documentElement).toHaveClass('dark')

    await userEvent.click(toggle())
    expect(document.documentElement).not.toHaveClass('dark')
  })
})
