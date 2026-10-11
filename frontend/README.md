# Фронтенд

Svelte 5 (руны) + TypeScript, сборка Vite 8. Инструменты: **bun**, **oxlint**, **oxfmt**, **svelte-check**.

```bash
bun install --frozen-lockfile
bun run dev          # Vite с HMR на :5173, /api проксируется на запущенный API
bun run verify       # lint + format:check + check + test + build — то же, что гоняет CI
```

| Команда | Что делает |
|---|---|
| `bun run lint` | oxlint (`correctness` и `suspicious` как ошибки), правила в `.oxlintrc.json` |
| `bun run format` / `format:check` | oxfmt, настройки в `.oxfmtrc.jsonc` (`.svelte` он не форматирует) |
| `bun run check` | svelte-check (`noImplicitAny`); число ошибок не должно расти относительно `.svelte-check-baseline` (сейчас 0) |
| `bun run test` | `bun test tests` |

## Типы API

Типы ответов сервера **генерируются из Go-структур** (`go/tygo.yaml`), руками их не правят:

```bash
just gen-types       # пишет src/lib/api/*.gen.ts; CI падает, если файлы устарели
```

`src/lib/js/api.ts` — клиент; возвращаемые типы берёт из `src/lib/api/*.gen.ts`. Новый эндпоинт: сначала Go-структура с
json-тегами (не `map[string]any`), потом `just gen-types`, потом функция в `api.ts` с `request<Тип>(…)`.

## Заметки

- Скрипты компонентов — `<script lang="ts">`; JSDoc-типов в коде не осталось, только аннотации TS.
- `esrap` закреплён на 2.3.0 в `overrides` (`package.json`): 2.4.0 ломает разбор приведений типов в шаблонах Svelte.
- svelte-check работает на TypeScript 5.9; нативный TS 7 его API пока не поддерживает.
- Явные `any` (сейчас около 728 вхождений) — известный долг: ищутся поиском `: any`, будут сужаться по мере правок.
