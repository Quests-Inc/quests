import { mount } from 'svelte'

import './app.css'
import App from './App.svelte'
import { applyTheme, loadSavedTheme } from './lib/js/theme.ts'

applyTheme(loadSavedTheme())

const target = document.getElementById('app')
if (!target) throw new Error('#app not found')

const app = mount(App, { target })

export default app
