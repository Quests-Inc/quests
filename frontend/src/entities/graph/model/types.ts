// Формы данных карты связей на клиенте: узел с раскладкой, ребро, шар группы, режим связывания.

export interface GraphNodeView {
  key: string
  type: string
  id: number | string
  title: string
  desc?: string
  status?: string
  progress?: string
  mime?: string
  icon?: string
  icon_url?: string
  color?: string
  draft?: boolean
  group?: string
  op?: number
  more?: Record<string, string | string[]>
  links: number
  depth: number
  x: number
  y: number
  w: number
  h: number
}

export interface GraphGroupView {
  key: string
  type: string
  owner: string
  title: string
  count: number
  x: number
  y: number
  w: number
  h: number
}

export interface GraphEdgeView {
  from: string
  to: string
  kind: string
  draft?: string
  d: string
  sx: number
  sy: number
}

export interface GraphNoteEditing {
  key: string
  title: string
  text: string
  fresh?: boolean
  orig?: { title: string; description: string }
  opId?: number
  payload?: unknown
}
