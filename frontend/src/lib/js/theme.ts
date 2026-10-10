/**
 * Theme switcher. Themes live in styles/themes/<name>.css as [data-theme="…"].
 * Import extra theme CSS from app.css when you add one.
 */

export const DEFAULT_THEME = 'gruvbox-yellow'

/** @typedef {{ id: string, label: string, scheme: 'dark' | 'light', swatches: [string, string, string] }} ThemeMeta */

/** @type {ThemeMeta[]} */
export const THEMES = [
  {
    id: 'gruvbox-yellow',
    label: 'Gruvbox Yellow',
    scheme: 'dark',
    swatches: ['#1d2021', '#fabd2f', '#ebdbb2'],
  },
  {
    id: 'gruvbox-dark',
    label: 'Gruvbox Dark',
    scheme: 'dark',
    swatches: ['#282828', '#fe8019', '#ebdbb2'],
  },
  {
    id: 'gruvbox-green',
    label: 'Gruvbox Green',
    scheme: 'dark',
    swatches: ['#1d2021', '#b8bb26', '#8ec07c'],
  },
  {
    id: 'gruvbox-light',
    label: 'Gruvbox Light',
    scheme: 'light',
    swatches: ['#fbf1c7', '#b57614', '#3c3836'],
  },
  {
    id: 'nord',
    label: 'Nord',
    scheme: 'dark',
    swatches: ['#2e3440', '#88c0d0', '#eceff4'],
  },
  {
    id: 'catppuccin-mocha',
    label: 'Catppuccin Mocha',
    scheme: 'dark',
    swatches: ['#1e1e2e', '#cba6f7', '#cdd6f4'],
  },
  {
    id: 'cyberpunk',
    label: 'Cyberpunk',
    scheme: 'dark',
    swatches: ['#0a0014', '#00f0ff', '#ff2bd6'],
  },
  {
    id: 'monokai',
    label: 'Monokai',
    scheme: 'dark',
    swatches: ['#272822', '#a6e22e', '#f92672'],
  },
  {
    id: 'rose-pine',
    label: 'Rosé Pine',
    scheme: 'dark',
    swatches: ['#191724', '#ebbcba', '#e0def4'],
  },
  {
    id: 'forest',
    label: 'Forest',
    scheme: 'dark',
    swatches: ['#121a14', '#7cb87c', '#e4efe4'],
  },
  {
    id: 'neon-grove',
    label: 'Neon Grove',
    scheme: 'dark',
    swatches: ['#07140c', '#39ff14', '#00f5ff'],
  },
  {
    id: 'parchment',
    label: 'Parchment',
    scheme: 'light',
    swatches: ['#f3ebe0', '#8b4518', '#2c241c'],
  },
  {
    id: 'solarized-light',
    label: 'Solarized Light',
    scheme: 'light',
    swatches: ['#fdf6e3', '#268bd2', '#657b83'],
  },
]

const THEME_IDS = new Set(THEMES.map((t) => t.id))

export function applyTheme(name = DEFAULT_THEME) {
  const id = THEME_IDS.has(name) ? name : DEFAULT_THEME
  document.documentElement.dataset.theme = id
  try {
    localStorage.setItem('quests.theme', id)
  } catch {
    /* ignore */
  }
  return id
}

export function loadSavedTheme() {
  try {
    const saved = localStorage.getItem('quests.theme')
    if (saved && THEME_IDS.has(saved)) return saved
  } catch {
    /* ignore */
  }
  return DEFAULT_THEME
}

export function currentThemeId() {
  return document.documentElement.dataset.theme || loadSavedTheme()
}
