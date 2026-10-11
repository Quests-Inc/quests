<script lang="ts">
  import Icon from './Icon.svelte'

  let {
    icon = 'document',
    iconUrl = null,
    size = 'md',
    alt = '',
  }: {
    icon?: string | null,
    iconUrl?: string | null,
    size?: 'sm' | 'md' | 'lg',
    alt?: string,
  } = $props()

  let custom = $derived(Boolean(iconUrl))
</script>

{#if custom}
  <!--
    <img> ignores page CSS color; mask + background uses currentColor
    so questline --line-color / color: inherit works.
  -->
  <span
    class="ql-icon ql-icon--custom ql-icon--{size}"
    style:--ql-mask="url({iconUrl})"
    role={alt ? 'img' : undefined}
    aria-label={alt || undefined}
    aria-hidden={alt ? undefined : 'true'}
  ></span>
{:else}
  <span class="ql-icon ql-icon--svg ql-icon--{size}" aria-hidden="true">
    <Icon name={icon || 'document'} size={size === 'sm' ? 12 : size === 'lg' ? 22 : 16} />
  </span>
{/if}

<style>
  .ql-icon {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: center;
  }

  .ql-icon--svg.ql-icon--md,
  .ql-icon--svg.ql-icon--lg {
    width: 2.25rem;
    height: 2.25rem;
    border-radius: 0.35rem;
    color: var(--line-color, currentColor);
    background: color-mix(in srgb, var(--line-color, #9a9a9a) 18%, transparent);
  }

  .ql-icon--custom {
    background-color: currentColor;
    color: var(--line-color, currentColor);
    -webkit-mask-image: var(--ql-mask);
    mask-image: var(--ql-mask);
    -webkit-mask-size: contain;
    mask-size: contain;
    -webkit-mask-repeat: no-repeat;
    mask-repeat: no-repeat;
    -webkit-mask-position: center;
    mask-position: center;
  }

  .ql-icon--custom.ql-icon--sm {
    width: 0.85rem;
    height: 0.85rem;
  }

  .ql-icon--custom.ql-icon--md {
    width: 2.25rem;
    height: 2.25rem;
  }

  /* Detail head: a bit larger than builtin box, not full header height. */
  .ql-icon--custom.ql-icon--lg {
    width: 2.75rem;
    height: 2.75rem;
  }
</style>
