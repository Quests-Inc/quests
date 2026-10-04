<script>
  import add from '../../assets/icons/add.svg?raw'
  import alert from '../../assets/icons/alert.svg?raw'
  import arrowUp from '../../assets/icons/arrow-up.svg?raw'
  import attachment from '../../assets/icons/attachment.svg?raw'
  import calendar from '../../assets/icons/calendar.svg?raw'
  import check from '../../assets/icons/check.svg?raw'
  import checkmark from '../../assets/icons/checkmark.svg?raw'
  import chevronDown from '../../assets/icons/chevron-down.svg?raw'
  import chevronRight from '../../assets/icons/chevron-right.svg?raw'
  import close from '../../assets/icons/close.svg?raw'
  import copy from '../../assets/icons/copy.svg?raw'
  import deleteIcon from '../../assets/icons/delete.svg?raw'
  import document from '../../assets/icons/document.svg?raw'
  import download from '../../assets/icons/download.svg?raw'
  import edit from '../../assets/icons/edit.svg?raw'
  import eye from '../../assets/icons/eye.svg?raw'
  import eyeOff from '../../assets/icons/eye-off.svg?raw'
  import fileDownload from '../../assets/icons/file-download.svg?raw'
  import filter from '../../assets/icons/filter.svg?raw'
  import flag from '../../assets/icons/flag.svg?raw'
  import folder from '../../assets/icons/folder.svg?raw'
  import key from '../../assets/icons/key.svg?raw'
  import lock from '../../assets/icons/lock.svg?raw'
  import layers from '../../assets/icons/layers.svg?raw'
  import list from '../../assets/icons/list.svg?raw'
  import map from '../../assets/icons/map.svg?raw'
  import more from '../../assets/icons/more.svg?raw'
  import pin from '../../assets/icons/pin.svg?raw'
  import pinFilled from '../../assets/icons/pin-filled.svg?raw'
  import renew from '../../assets/icons/renew.svg?raw'
  import repeat from '../../assets/icons/repeat.svg?raw'
  import save from '../../assets/icons/save.svg?raw'
  import scroll from '../../assets/icons/scroll.svg?raw'
  import settings from '../../assets/icons/settings.svg?raw'
  import sidebar from '../../assets/icons/sidebar.svg?raw'
  import subtract from '../../assets/icons/subtract.svg?raw'
  import target from '../../assets/icons/target.svg?raw'
  import terminal from '../../assets/icons/terminal.svg?raw'
  import undo from '../../assets/icons/undo.svg?raw'

  const ICONS = {
    add,
    alert,
    'arrow-up': arrowUp,
    attachment,
    calendar,
    check,
    checkmark,
    'chevron-down': chevronDown,
    'chevron-right': chevronRight,
    close,
    copy,
    delete: deleteIcon,
    document,
    download,
    edit,
    eye,
    'eye-off': eyeOff,
    'file-download': fileDownload,
    filter,
    flag,
    folder,
    key,
    lock,
    layers,
    list,
    map,
    more,
    pin,
    'pin-filled': pinFilled,
    renew,
    repeat,
    save,
    scroll,
    settings,
    sidebar,
    subtract,
    target,
    terminal,
    undo,
  }

  /** @type {{ name: string, size?: number, label?: string }} */
  let { name, size = 16, label = '' } = $props()

  let svg = $derived(ICONS[name] ?? '')
</script>

<span
  class="icon"
  style:--icon-size="{size}px"
  aria-hidden={label ? undefined : 'true'}
  aria-label={label || undefined}
  role={label ? 'img' : undefined}
>
  {@html svg}
</span>

<style>
  .icon {
    display: inline-flex;
    flex-shrink: 0;
    width: var(--icon-size, 16px);
    height: var(--icon-size, 16px);
    color: inherit;
  }

  .icon :global(svg) {
    width: 100%;
    height: 100%;
    display: block;
  }
</style>
