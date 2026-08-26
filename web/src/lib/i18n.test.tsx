import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { I18nProvider, useI18n } from './i18n'

function Probe({ k, vars }: { k: string; vars?: Record<string, string | number> }) {
  const { t, language, dir, setLanguage } = useI18n()
  return (
    <div>
      <span data-testid="value">{t(k, vars)}</span>
      <span data-testid="language">{language}</span>
      <span data-testid="dir">{dir}</span>
      <button onClick={() => setLanguage(language === 'he' ? 'en' : 'he')}>toggle</button>
    </div>
  )
}

const renderProbe = (k: string, vars?: Record<string, string | number>) =>
  render(
    <I18nProvider>
      <Probe k={k} vars={vars} />
    </I18nProvider>,
  )

describe('translation', () => {
  it('resolves a key', () => {
    renderProbe('nav.contacts')
    expect(screen.getByTestId('value').textContent).toBe('Contacts')
  })

  it('interpolates variables', () => {
    renderProbe('dashboard.inDays', { n: 5 })
    expect(screen.getByTestId('value').textContent).toBe('in 5 days')
  })

  // Better a visible key than a blank space, which reads as a broken layout.
  it('falls back to the key itself when it is unknown', () => {
    renderProbe('does.not.exist')
    expect(screen.getByTestId('value').textContent).toBe('does.not.exist')
  })
})

describe('language switching', () => {
  it('starts in English and flips direction with the language', async () => {
    renderProbe('nav.contacts')
    expect(screen.getByTestId('dir').textContent).toBe('ltr')

    await userEvent.click(screen.getByRole('button', { name: 'toggle' }))

    expect(screen.getByTestId('language').textContent).toBe('he')
    expect(screen.getByTestId('dir').textContent).toBe('rtl')
    expect(screen.getByTestId('value').textContent).toBe('אנשי קשר')
  })

  // The whole page must mirror, not just the strings.
  it('sets lang and dir on the document element', async () => {
    renderProbe('nav.contacts')
    expect(document.documentElement.lang).toBe('en')
    expect(document.documentElement.dir).toBe('ltr')

    await userEvent.click(screen.getByRole('button', { name: 'toggle' }))
    expect(document.documentElement.lang).toBe('he')
    expect(document.documentElement.dir).toBe('rtl')
  })

  it('remembers the choice', async () => {
    renderProbe('nav.contacts')
    await userEvent.click(screen.getByRole('button', { name: 'toggle' }))
    expect(localStorage.getItem('blessedbot.language')).toBe('he')
  })

  it('restores a stored choice on mount', () => {
    localStorage.setItem('blessedbot.language', 'he')
    renderProbe('nav.contacts')
    expect(screen.getByTestId('language').textContent).toBe('he')
  })

  it('ignores a corrupt stored value rather than rendering a broken UI', () => {
    localStorage.setItem('blessedbot.language', 'klingon')
    renderProbe('nav.contacts')
    expect(['en', 'he']).toContain(screen.getByTestId('language').textContent)
  })
})

/**
 * The two dictionaries are maintained by hand, so drift is the likely failure:
 * a key added in English and forgotten in Hebrew silently renders the English
 * string (or the raw key) to a Hebrew-speaking user.
 */
describe('dictionary parity', () => {
  // Read through the provider rather than exporting the dictionaries, so the
  // test exercises the same lookup path the app uses.
  function lookup(language: 'en' | 'he', key: string): string {
    localStorage.setItem('blessedbot.language', language)
    const { unmount } = render(
      <I18nProvider>
        <Probe k={key} />
      </I18nProvider>,
    )
    const value = screen.getByTestId('value').textContent ?? ''
    unmount()
    return value
  }

  // Every key the pages actually use. A key missing from Hebrew resolves to the
  // English string, which this catches.
  const keys = [
    'app.title', 'nav.dashboard', 'nav.contacts', 'nav.blessings', 'nav.groups', 'nav.settings',
    'common.add', 'common.edit', 'common.delete', 'common.save', 'common.cancel', 'common.search',
    'common.enabled', 'common.disabled', 'common.language', 'common.name', 'common.all',
    'common.loading', 'common.retry', 'common.saving', 'common.optional', 'common.close',
    'notices.title', 'notices.dismiss', 'notices.language_fallback',
    'dashboard.provider', 'dashboard.connected', 'dashboard.needsQR', 'dashboard.notConfigured',
    'dashboard.upcoming', 'dashboard.upcomingEmpty', 'dashboard.recent', 'dashboard.recentEmpty',
    'dashboard.sentToday', 'dashboard.failedToday', 'dashboard.contacts', 'dashboard.today',
    'contacts.title', 'contacts.add', 'contacts.empty', 'contacts.phone', 'contacts.eventDate',
    'contacts.eventType', 'contacts.relation', 'contacts.gender', 'contacts.importance',
    'contacts.sendTime', 'contacts.sendNow', 'contacts.confirmDelete', 'contacts.phoneHint',
    'blessings.title', 'blessings.add', 'blessings.empty', 'blessings.text', 'blessings.preview',
    'blessings.anyGender', 'blessings.anyRelation', 'blessings.placeholderHint',
    'blessings.groupUsable', 'blessings.confirmDelete',
    'groups.title', 'groups.add', 'groups.empty', 'groups.chatId', 'groups.threshold',
    'groups.thresholdHint', 'groups.useDefault', 'groups.pick', 'groups.manual',
    'groups.patterns', 'groups.patternsHint', 'groups.pattern', 'groups.confirmDelete',
    'settings.title', 'settings.provider', 'settings.apiUrl', 'settings.idInstance',
    'settings.apiToken', 'settings.mode', 'settings.baseUrl', 'settings.password',
    'settings.deviceId', 'settings.webhookSecret', 'settings.scheduler', 'settings.timezone',
    'settings.generalSendTime', 'settings.groupEcho', 'settings.threshold', 'settings.window',
    'settings.cooldown', 'settings.test', 'settings.sendTest', 'settings.saved',
    'settings.secretKept', 'settings.uiLanguage', 'settings.theme',
    'event.birthday', 'event.wedding', 'event.anniversary', 'event.custom',
    'gender.male', 'gender.female', 'gender.other',
    'relation.friend', 'relation.close_friend', 'relation.family', 'relation.coworker',
    'kind.scheduled', 'kind.group_echo', 'status.sent', 'status.failed',
  ]

  it('has a Hebrew string for every key the UI uses', () => {
    const missing = keys.filter((key) => {
      const en = lookup('en', key)
      const he = lookup('he', key)
      // Untranslated shows as the English string or the bare key. Provider and
      // brand names are legitimately identical, so allow an explicit few.
      const sameIsFine = ['settings.greenapi', 'settings.gowa'].includes(key)
      return he === key || (!sameIsFine && he === en && /[a-zA-Z]/.test(en))
    })
    expect(missing, `keys with no Hebrew translation: ${missing.join(', ')}`).toEqual([])
  })

  it('has an English string for every key too', () => {
    const missing = keys.filter((key) => lookup('en', key) === key)
    expect(missing, `keys with no English translation: ${missing.join(', ')}`).toEqual([])
  })
})
