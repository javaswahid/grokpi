import type { Metadata } from 'next'
import './globals.css'
import { LanguageProvider } from '@/lib/i18n/context'
import { ThemeProvider } from '@/lib/theme/context'

export const metadata: Metadata = {
  title: 'MasantoID Admin',
  description: 'MasantoID Administration Panel',
  icons: {
    icon: '/favicon.svg',
    shortcut: '/favicon.svg',
  },
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="id" suppressHydrationWarning>
      <body className="bg-background text-foreground font-sans antialiased min-h-screen relative">
        {/* Fluent 2 mesh gradient background */}
        <div className="fixed inset-0 pointer-events-none -z-10">
          <div className="absolute top-[-20%] left-[-10%] w-[50%] h-[50%] bg-[#005FB8]/5 rounded-full blur-[100px]" />
          <div className="absolute bottom-[-10%] right-[-10%] w-[60%] h-[60%] bg-[#0091FF]/5 rounded-full blur-[120px]" />
          <div className="absolute top-[20%] right-[10%] w-[40%] h-[40%] bg-[#C42B1C]/3 rounded-full blur-[90px]" />
        </div>

        <ThemeProvider>
          <LanguageProvider>
            {children}
          </LanguageProvider>
        </ThemeProvider>
      </body>
    </html>
  )
}
