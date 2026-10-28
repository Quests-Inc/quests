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
  /** Size the person gave by stretching the card; without it the card is as big as its content. */
  sw?: number
  sh?: number
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
  /** End point and the direction the edge arrives in (for the arrowhead). */
  ex: number
  ey: number
  fx: number
  fy: number
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
