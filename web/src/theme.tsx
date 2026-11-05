import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

// Light and dark, the user's choice in localStorage, the attribute the CSS variables hang on.

type Theme = 'light' | 'dark'

const ThemeCtx = createContext<{ theme: Theme; setTheme: (t: Theme) => void }>({ theme: 'light', setTheme: () => {} })

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => {
    const kept = localStorage.getItem('pack-ui-theme') as Theme | null
    if (kept === 'light' || kept === 'dark') return kept
    return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  })
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('pack-ui-theme', theme)
  }, [theme])
  return <ThemeCtx.Provider value={{ theme, setTheme }}>{children}</ThemeCtx.Provider>
}

export function useTheme() {
  return useContext(ThemeCtx)
}
