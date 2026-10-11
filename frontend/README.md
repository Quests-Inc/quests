# Фронтенд

Svelte 5 (руны) + TypeScript, сборка Vite 8. Инструменты: **bun**, **oxlint**, **oxfmt**, **svelte-check**.

```bash
bun install --frozen-lockfile
bun run dev          # Vite с HMR на :5173, /api проксируется на запущенный API
bun run verify       # lint + format:check + check + test + build — то же, что гоняет CI
```

| Команда                           | Что делает                                                                                                                  |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `bun run lint`                    | oxlint (`correctness` и `suspicious` как ошибки), правила в `.oxlintrc.json`                                                |
| `bun run format` / `format:check` | oxfmt, настройки в `.oxfmtrc.jsonc` (`.svelte` он не форматирует)                                                           |
| `bun run check`                   | svelte-check (`noImplicitAny`); число ошибок не должно расти относительно `.svelte-check-baseline` (сейчас 0)               |
| `bun run check:any`               | число явных `any` не должно расти относительно `.any-baseline`                                                              |
| `bun run test`                    | `tests/unit` (чистая логика) и `tests/components` (Svelte-компоненты на happy-dom, загрузчик — `tests/components/setup.ts`) |

## Структура (FSD, ADR-0001)

```
src/
  main.ts        точка входа (монтирует app/App.svelte)
  app/           оболочка: App.svelte, глобальные стили и темы (app/styles)
  pages/         экраны: graph, notes, attachments, history, runners, auth
  widgets/       крупные блоки страницы: journal-header, quest-sidebar, quest-detail, activity-calendar
  features/      пользовательские сценарии (модалки): quest-edit, questline-edit, templates, secrets, tags, settings, assistant, find-entity, note-icon
  entities/      предметные сущности: quest, note, attachment, template, runner, questline, tag, history, graph (ui/ + model/)
  shared/        ui, api (клиент и сгенерированные типы), lib (утилиты без предметной области)
```

Импорт — только вниз по слоям (`app → pages → widgets → features → entities → shared`), в слайс заходят через его `index.ts`,
слайсы одного слоя друг друга не знают; между сущностями — только через `@x` (`entities/runner/@x/note.ts`).
Алиас `@/` указывает на `src/`. Проверяет `bun run check:fsd` (входит в `verify`); сейчас нарушений 0.

## Типы API

Типы ответов сервера **генерируются из Go-структур** (`go/tygo.yaml`), руками их не правят:

```bash
just gen-types       # пишет src/shared/api/*.gen.ts; CI падает, если файлы устарели
```

`src/shared/api/client.ts` — клиент; возвращаемые типы берёт из `src/shared/api/*.gen.ts`. Новый эндпоинт: сначала Go-структура с
json-тегами (не `map[string]any`), потом `just gen-types`, потом функция в `api.ts` с `request<Тип>(…)`.

## Заметки

- Скрипты компонентов — `<script lang="ts">`; JSDoc-типов в коде не осталось, только аннотации TS.
- `esrap` закреплён на 2.3.0 в `overrides` (`package.json`): 2.4.0 ломает разбор приведений типов в шаблонах Svelte.
- svelte-check работает на TypeScript 5.9; нативный TS 7 его API пока не поддерживает.
- Явные `any` — известный долг; их число зажато гейтом `check:any` (планка в `.any-baseline`), сужайте по мере правок и опускайте планку.
