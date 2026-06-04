'use client'

import Link from 'next/link'
import { useSystemStatus } from '@/lib/hooks'
import { Badge } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import { usePathname } from 'next/navigation'
import { ArrowLeft, Menu, Moon, Sun } from 'lucide-react'
import { useTheme } from '@/lib/theme/context'

interface HeaderProps {
  onMenuClick?: () => void
}

export function Header({ onMenuClick }: HeaderProps) {
  const { data: status } = useSystemStatus()
  const { t, language, setLanguage } = useTranslation()
  const { theme, toggleTheme } = useTheme()
  const pathname = usePathname()

  const getPageTitle = () => {
    switch (true) {
      case pathname.startsWith('/dashboard'): return t.nav.dashboard
      case pathname.startsWith('/tokens'): return t.nav.tokens
      case pathname.startsWith('/apikeys'): return t.nav.apiKeys
      case pathname.startsWith('/function'): return t.nav.function
      case pathname.startsWith('/usage'): return t.nav.usage
      case pathname.startsWith('/cache'): return t.nav.cache
      case pathname.startsWith('/settings'): return t.nav.settings
      default: return t.nav.dashboard
    }
  }

  return (
    <header className="h-[48px] flex items-center px-4 sticky top-0 z-10 shrink-0 justify-between select-none">
      <div className="flex items-center gap-3">
        {onMenuClick && (
          <button
            type="button"
            onClick={onMenuClick}
            className="w-8 h-8 rounded-full hover:bg-black/8 dark:hover:bg-white/12 active:scale-95 flex items-center justify-center transition-all text-foreground md:hidden"
            aria-label="Toggle Menu"
          >
            <Menu className="w-5 h-5" />
          </button>
        )}
        {pathname !== '/dashboard' && (
          <Link
            href="/dashboard"
            className="w-8 h-8 rounded-full hover:bg-black/8 dark:hover:bg-white/12 active:scale-95 flex items-center justify-center transition-all text-foreground hidden md:flex"
          >
            <ArrowLeft className="w-4 h-4" />
          </Link>
        )}
        <h1 className="text-xl font-semibold text-foreground tracking-tight">
          {getPageTitle()}
        </h1>
      </div>

      <div className="flex items-center gap-3">
        {status && (
          <Badge
            variant={
              status.status === 'healthy'
                ? 'success'
                : status.status === 'degraded'
                  ? 'warning'
                  : 'destructive'
            }
          >
            {t.header[status.status]}
          </Badge>
        )}
        <span className="text-[12px] text-muted hidden sm:inline-block">
          {status?.version}
        </span>

        <button
          type="button"
          onClick={() => setLanguage(language === 'id' ? 'en' : 'id')}
          className="btn-fluent px-2 py-1 text-[12px] font-medium text-muted"
        >
          {language === 'id' ? 'EN' : 'ID'}
        </button>

        <button
          type="button"
          onClick={toggleTheme}
          className="btn-fluent p-1.5 text-muted"
          aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
          title={theme === 'dark' ? 'Light mode' : 'Dark mode'}
        >
          {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </button>
      </div>
    </header>
  )
}
