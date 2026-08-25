import { NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { CalendarHeart, Gift, LayoutDashboard, Moon, Settings as SettingsIcon, Sun, Users } from 'lucide-react'

import { Button } from '@/components/ui'
import { useI18n } from '@/lib/i18n'
import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'
import Blessings from '@/pages/Blessings'
import Contacts from '@/pages/Contacts'
import Dashboard from '@/pages/Dashboard'
import Groups from '@/pages/Groups'
import Settings from '@/pages/Settings'

const navItems = [
  { to: '/', key: 'nav.dashboard', Icon: LayoutDashboard },
  { to: '/contacts', key: 'nav.contacts', Icon: Users },
  { to: '/blessings', key: 'nav.blessings', Icon: Gift },
  { to: '/groups', key: 'nav.groups', Icon: CalendarHeart },
  { to: '/settings', key: 'nav.settings', Icon: SettingsIcon },
]

export default function App() {
  const { t, language, setLanguage } = useI18n()
  const { resolved, setTheme } = useTheme()

  return (
    <div className="min-h-dvh">
      <header className="sticky top-0 z-40 border-b bg-card/90 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-5xl items-center gap-3 px-4">
          <span className="text-lg" aria-hidden>🎉</span>
          <span className="truncate font-semibold">{t('app.title')}</span>

          <div className="ms-auto flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setLanguage(language === 'he' ? 'en' : 'he')}
              aria-label={t('settings.uiLanguage')}
            >
              {language === 'he' ? 'EN' : 'עב'}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setTheme(resolved === 'dark' ? 'light' : 'dark')}
              aria-label={t('settings.theme')}
            >
              {resolved === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
            </Button>
          </div>
        </div>

        {/* Desktop navigation. On a phone the bottom bar takes over: this app is
            used mostly from a pocket, so the primary targets sit under a thumb. */}
        <nav className="mx-auto hidden max-w-5xl gap-1 px-2 pb-2 sm:flex">
          {navItems.map(({ to, key, Icon }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition',
                  isActive ? 'bg-primary text-primary-foreground' : 'hover:bg-muted',
                )
              }
            >
              <Icon size={16} />
              {t(key)}
            </NavLink>
          ))}
        </nav>
      </header>

      <main className="mx-auto max-w-5xl px-4 pb-24 pt-5 sm:pb-10">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/contacts" element={<Contacts />} />
          <Route path="/blessings" element={<Blessings />} />
          <Route path="/groups" element={<Groups />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>

      <nav className="fixed inset-x-0 bottom-0 z-40 border-t bg-card/95 pb-[env(safe-area-inset-bottom)] backdrop-blur sm:hidden">
        <div className="flex">
          {navItems.map(({ to, key, Icon }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex flex-1 flex-col items-center gap-1 py-2 text-[11px]',
                  isActive ? 'text-primary' : 'text-muted-foreground',
                )
              }
            >
              <Icon size={20} />
              <span className="truncate px-1">{t(key)}</span>
            </NavLink>
          ))}
        </div>
      </nav>
    </div>
  )
}
