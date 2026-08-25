// Typed client for /api/v1. Every UI action maps to a documented endpoint
// (docs/api.md); nothing here talks to the database or invents state.

export type EventType = 'birthday' | 'wedding' | 'anniversary' | 'custom'
export type Gender = 'male' | 'female' | 'other'
export type Relation = 'friend' | 'close_friend' | 'family' | 'coworker'

export interface Contact {
  id: number
  name: string
  phone: string
  event_date: string
  event_type: EventType
  language: string
  relation: Relation
  importance: number
  gender: Gender
  send_time: string | null
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface Blessing {
  id: number
  event_type: EventType
  language: string
  gender: Gender | null
  relation: Relation | null
  text: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface Group {
  id: number
  name: string
  chat_id: string
  language: string
  threshold: number | null
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface WishPattern {
  id: number
  language: string
  pattern: string
  enabled: boolean
}

export interface SendLogEntry {
  id: number
  kind: 'scheduled' | 'group_echo'
  contact_id: number | null
  group_id: number | null
  blessing_id: number | null
  provider: string
  chat_id: string
  status: 'sent' | 'failed'
  error: string | null
  event_year: number | null
  sent_at: string
}

export interface ProviderStatus {
  provider: string
  connected: boolean
  state: string
  needs_qr: boolean
  detail?: string
}

export interface AvailableGroup {
  chat_id: string
  name: string
}

export interface Settings {
  provider: 'greenapi' | 'gowa'
  greenapi: {
    api_url: string
    id_instance: string
    api_token: string
    mode: 'polling' | 'webhook'
    webhook_auth_header: string
  }
  gowa: {
    base_url: string
    username: string
    password: string
    device_id: string
    webhook_secret: string
  }
  scheduler: { timezone: string; send_time: string }
  group_echo: { threshold: number; window_hours: number; cooldown_hours: number }
  provider_error?: string
}

export interface Health {
  status: string
  version: string
  database?: string
}

/** The mask the API returns in place of a stored secret. Sending it back means
 *  "leave this unchanged", so inputs must round-trip it untouched. */
export const SECRET_MASK = '••••'

export interface FieldError {
  field: string
  message: string
}

/** ApiError carries the server's error envelope so forms can show per-field
 *  messages instead of a generic failure. */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly fields: FieldError[]

  constructor(status: number, code: string, message: string, fields: FieldError[] = []) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.fields = fields
  }

  /** fieldMessage returns the message for one field, if the server named it. */
  fieldMessage(field: string): string | undefined {
    return this.fields.find((f) => f.field === field)?.message
  }
}

const BASE = '/api/v1'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (res.status === 204) return undefined as T

  const text = await res.text()
  const parsed: unknown = text ? JSON.parse(text) : null

  if (!res.ok) {
    const envelope = parsed as { error?: { code?: string; message?: string; fields?: FieldError[] } }
    throw new ApiError(
      res.status,
      envelope?.error?.code ?? 'unknown',
      envelope?.error?.message ?? res.statusText,
      envelope?.error?.fields ?? [],
    )
  }
  return parsed as T
}

export const api = {
  health: () => request<Health>('GET', '/healthz'),

  listContacts: () => request<Contact[]>('GET', '/contacts'),
  createContact: (c: Partial<Contact>) => request<Contact>('POST', '/contacts', c),
  updateContact: (id: number, c: Partial<Contact>) => request<Contact>('PUT', `/contacts/${id}`, c),
  deleteContact: (id: number) => request<void>('DELETE', `/contacts/${id}`),
  sendNow: (id: number, force = false) =>
    request<SendLogEntry>('POST', `/contacts/${id}/send-now${force ? '?force=true' : ''}`),

  listBlessings: () => request<Blessing[]>('GET', '/blessings'),
  createBlessing: (b: Partial<Blessing>) => request<Blessing>('POST', '/blessings', b),
  updateBlessing: (id: number, b: Partial<Blessing>) => request<Blessing>('PUT', `/blessings/${id}`, b),
  deleteBlessing: (id: number) => request<void>('DELETE', `/blessings/${id}`),

  listGroups: () => request<Group[]>('GET', '/groups'),
  availableGroups: () => request<AvailableGroup[]>('GET', '/groups/available'),
  createGroup: (g: Partial<Group>) => request<Group>('POST', '/groups', g),
  updateGroup: (id: number, g: Partial<Group>) => request<Group>('PUT', `/groups/${id}`, g),
  deleteGroup: (id: number) => request<void>('DELETE', `/groups/${id}`),

  listWishPatterns: () => request<WishPattern[]>('GET', '/wish-patterns'),
  createWishPattern: (p: Partial<WishPattern>) => request<WishPattern>('POST', '/wish-patterns', p),
  updateWishPattern: (id: number, p: Partial<WishPattern>) =>
    request<WishPattern>('PUT', `/wish-patterns/${id}`, p),
  deleteWishPattern: (id: number) => request<void>('DELETE', `/wish-patterns/${id}`),

  getSettings: () => request<Settings>('GET', '/settings'),
  saveSettings: (s: Partial<Settings>) => request<Settings>('PUT', '/settings', s),

  providerStatus: () => request<ProviderStatus>('GET', '/provider/status'),
  providerTest: (phone: string, message?: string) =>
    request<{ provider: string; chat_id: string; message_id: string }>('POST', '/provider/test', {
      phone,
      message: message ?? '',
    }),

  history: (kind?: string, limit = 25) =>
    request<SendLogEntry[]>(
      'GET',
      `/history?limit=${limit}${kind ? `&kind=${encodeURIComponent(kind)}` : ''}`,
    ),
}
