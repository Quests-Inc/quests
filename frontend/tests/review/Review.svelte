<script lang="ts">
 import TemplatesModal from '@/features/templates/ui/TemplatesModal.svelte'
 import MarkdownBody from '@/shared/ui/MarkdownBody.svelte'
 let tab=$state('templates')
 let width=$state('wide')
 let theme=$state('gruvbox-yellow')
 const prose=String.raw`# Markdown и формулы

> Это пояснение внутри заметки.
>
> Вложенное **выделение**, несколько абзацев и [ссылка](https://example.com).

## Возможности

- [x] Статический чекбокс
- [ ] Второй пункт с ~~зачёркнутым~~ текстом

##### Заголовок пятого уровня
###### Заголовок шестого уровня

Среднегодовая стоимость ОПФ:

$$\Phi_{ср} = \Phi_{н} + \frac{\sum \Phi_{вв} \cdot t}{12} - \frac{\sum \Phi_{выб} \cdot t}{12}$$

**Фондоотдача** $= \dfrac{4384}{8926{,}58} = \mathbf{0{,}491}$

$$\Phi_{ср} = 8825 + \frac{1324}{12} - \frac{105}{12} = 8825 + 110{,}333 - 8{,}75 = \mathbf{8926{,}58}\ \text{тыс. руб.}$$

Ошибочная формула $\unsupported{x}$.

## Диагностика

Запуск: script=1{диагностика}

<!-- table-id: diagnostics -->
| Состояние | Решение |
|---|---|
| <!-- row-id: ok -->Все службы отвечают | Ничего не делать |
| <!-- row-id: restart -->Один сервис не отвечает | Перезапустить и проверить журнал |

---

Цена $10 и $20 — обычный текст.
`
</script>
<svelte:head><style>html,body,#app{height:auto!important;max-height:none!important;overflow:auto!important}</style></svelte:head>
<div data-theme={theme} class="review">
 <nav aria-label="Проверка интерфейса">
  <button onclick={()=>tab='templates'}>Шаблоны</button><button onclick={()=>tab='markdown'}>Markdown</button>
  <button onclick={()=>width=width==='wide'?'narrow':'wide'}>{width==='wide'?'Узкий экран':'Широкий экран'}</button>
  <button onclick={()=>theme=theme==='gruvbox-yellow'?'gruvbox-light':'gruvbox-yellow'}>Сменить тему</button>
 </nav>
 <main class:narrow={width==='narrow'}>
 {#if tab==='templates'}<TemplatesModal open embedded onClose={()=>{}} onChanged={()=>{}} />{:else}<MarkdownBody source={prose} />{/if}
 </main>
</div>
<style>
.review { min-height:100vh; background:var(--color-bg);color:var(--color-fg);padding:1rem; }
nav {display:flex;gap:0.5rem;flex-wrap:wrap;margin-bottom:1rem;}
nav button{background:var(--color-bg-muted);border:1px solid var(--color-border);border-radius:4px;padding:0.5rem;color:inherit;cursor:pointer;}
main {max-width:76rem;margin:auto;min-width:0;}
main.narrow{max-width:360px;}
</style>
