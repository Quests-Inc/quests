import { mount } from 'svelte'

import '@/app/app.css'
import App from '@/app/App.svelte'
import { applyTheme, loadSavedTheme } from '@/shared/lib/theme.ts'

applyTheme(loadSavedTheme())

const target = document.getElementById('app')
if (!target) throw new Error('#app not found')

const app = mount(App, { target })

export default app
