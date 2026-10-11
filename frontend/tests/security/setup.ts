// Среда проверок безопасности: настоящий DOM (jsdom), в котором DOMPurify санитизирует так же, как в браузере.
// happy-dom для этого не годится: DOMPurify в нём «не поддерживается» и возвращает разметку как есть.
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/' })
Object.assign(globalThis, { window: dom.window, document: dom.window.document, navigator: dom.window.navigator })
