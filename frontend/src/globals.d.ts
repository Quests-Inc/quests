// Hooks the native shells (Android HubActivity) call into the page.
interface Window {
  questsNav?: {
    setTab: (name: string) => void
    currentTab: () => string
  }
}

// no bundled types
declare module 'qrcode'
