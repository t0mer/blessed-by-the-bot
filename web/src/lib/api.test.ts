import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError, SECRET_MASK, api } from './api'

/** mockFetch installs a fetch double and returns the calls it received. */
function mockFetch(response: { status?: number; body?: unknown; text?: string }) {
  const calls: Array<{ url: string; init?: RequestInit }> = []
  const status = response.status ?? 200
  const text = response.text ?? (response.body === undefined ? '' : JSON.stringify(response.body))

  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init })
      return {
        ok: status >= 200 && status < 300,
        status,
        statusText: `status ${status}`,
        text: async () => text,
      } as Response
    }),
  )
  return calls
}

afterEach(() => vi.unstubAllGlobals())

describe('request plumbing', () => {
  it('prefixes every path with /api/v1', async () => {
    const calls = mockFetch({ body: [] })
    await api.listContacts()
    expect(calls[0].url).toBe('/api/v1/contacts')
  })

  it('sends JSON with a content type only when there is a body', async () => {
    const calls = mockFetch({ body: { id: 1 } })
    await api.createContact({ name: 'Dana' })

    expect(calls[0].init?.method).toBe('POST')
    expect(calls[0].init?.body).toBe('{"name":"Dana"}')
    expect((calls[0].init?.headers as Record<string, string>)['Content-Type']).toBe('application/json')
  })

  it('omits the body and content type on a GET', async () => {
    const calls = mockFetch({ body: [] })
    await api.listContacts()
    expect(calls[0].init?.body).toBeUndefined()
    expect(calls[0].init?.headers).toBeUndefined()
  })

  // DELETE answers 204 with no body; parsing it as JSON would throw.
  it('handles a 204 with no body', async () => {
    mockFetch({ status: 204 })
    await expect(api.deleteContact(1)).resolves.toBeUndefined()
  })
})

describe('error handling', () => {
  it('turns the error envelope into an ApiError', async () => {
    mockFetch({
      status: 422,
      body: {
        error: {
          code: 'validation_failed',
          message: 'one or more fields are invalid',
          fields: [{ field: 'phone', message: 'must be an international number' }],
        },
      },
    })

    await expect(api.createContact({})).rejects.toThrowError(ApiError)
    try {
      await api.createContact({})
    } catch (err) {
      const apiErr = err as ApiError
      expect(apiErr.status).toBe(422)
      expect(apiErr.code).toBe('validation_failed')
      // This is what puts a message under the right form input.
      expect(apiErr.fieldMessage('phone')).toBe('must be an international number')
      expect(apiErr.fieldMessage('name')).toBeUndefined()
    }
  })

  it('survives a non-JSON error body rather than masking it with a parse error', async () => {
    mockFetch({ status: 502, text: '' })
    try {
      await api.listContacts()
      expect.unreachable('should have thrown')
    } catch (err) {
      const apiErr = err as ApiError
      expect(apiErr.status).toBe(502)
      expect(apiErr.code).toBe('unknown')
    }
  })

  it('exposes an empty field list when the server names none', async () => {
    mockFetch({ status: 503, body: { error: { code: 'provider_unavailable', message: 'none' } } })
    try {
      await api.providerStatus()
      expect.unreachable('should have thrown')
    } catch (err) {
      expect((err as ApiError).fields).toEqual([])
      expect((err as ApiError).fieldMessage('anything')).toBeUndefined()
    }
  })
})

describe('endpoint shapes', () => {
  it('builds the send-now URL, with and without force', async () => {
    let calls = mockFetch({ body: {} })
    await api.sendNow(7)
    expect(calls[0].url).toBe('/api/v1/contacts/7/send-now')

    calls = mockFetch({ body: {} })
    await api.sendNow(7, true)
    expect(calls[0].url).toBe('/api/v1/contacts/7/send-now?force=true')
  })

  it('builds the history URL with kind and limit', async () => {
    let calls = mockFetch({ body: [] })
    await api.history('group_echo', 5)
    expect(calls[0].url).toBe('/api/v1/history?limit=5&kind=group_echo')

    calls = mockFetch({ body: [] })
    await api.history()
    expect(calls[0].url).toBe('/api/v1/history?limit=25')
  })

  it('escapes a kind that would otherwise break the query string', async () => {
    const calls = mockFetch({ body: [] })
    await api.history('a&b=c')
    expect(calls[0].url).toContain('kind=a%26b%3Dc')
  })

  it('asks for dismissed notices only when told to', async () => {
    let calls = mockFetch({ body: [] })
    await api.notices()
    expect(calls[0].url).toBe('/api/v1/notices')

    calls = mockFetch({ body: [] })
    await api.notices(true)
    expect(calls[0].url).toBe('/api/v1/notices?all=true')
  })

  it('sends an empty string when no test message is given', async () => {
    const calls = mockFetch({ body: {} })
    await api.providerTest('972501234567')
    expect(JSON.parse(calls[0].init?.body as string)).toEqual({
      phone: '972501234567',
      message: '',
    })
  })
})

describe('SECRET_MASK', () => {
  // The API reads this exact value back as "leave the stored secret alone", so
  // the constant must not drift from the server's.
  it('is the four-bullet mask the server sends', () => {
    expect(SECRET_MASK).toBe('••••')
    expect(SECRET_MASK).toHaveLength(4)
  })
})

// Every wrapper's method and path, checked against docs/api.md. A typo here is
// invisible until the UI 404s at runtime, and the table is cheaper than finding
// that out from a screenshot.
describe('endpoint table', () => {
  const cases: Array<[string, () => Promise<unknown>, string, string]> = [
    ['health', () => api.health(), 'GET', '/api/v1/healthz'],

    ['listContacts', () => api.listContacts(), 'GET', '/api/v1/contacts'],
    ['createContact', () => api.createContact({}), 'POST', '/api/v1/contacts'],
    ['updateContact', () => api.updateContact(7, {}), 'PUT', '/api/v1/contacts/7'],
    ['deleteContact', () => api.deleteContact(7), 'DELETE', '/api/v1/contacts/7'],

    ['listBlessings', () => api.listBlessings(), 'GET', '/api/v1/blessings'],
    ['createBlessing', () => api.createBlessing({}), 'POST', '/api/v1/blessings'],
    ['updateBlessing', () => api.updateBlessing(7, {}), 'PUT', '/api/v1/blessings/7'],
    ['deleteBlessing', () => api.deleteBlessing(7), 'DELETE', '/api/v1/blessings/7'],

    ['listGroups', () => api.listGroups(), 'GET', '/api/v1/groups'],
    ['availableGroups', () => api.availableGroups(), 'GET', '/api/v1/groups/available'],
    ['createGroup', () => api.createGroup({}), 'POST', '/api/v1/groups'],
    ['updateGroup', () => api.updateGroup(7, {}), 'PUT', '/api/v1/groups/7'],
    ['deleteGroup', () => api.deleteGroup(7), 'DELETE', '/api/v1/groups/7'],

    ['listWishPatterns', () => api.listWishPatterns(), 'GET', '/api/v1/wish-patterns'],
    ['createWishPattern', () => api.createWishPattern({}), 'POST', '/api/v1/wish-patterns'],
    ['updateWishPattern', () => api.updateWishPattern(7, {}), 'PUT', '/api/v1/wish-patterns/7'],
    ['deleteWishPattern', () => api.deleteWishPattern(7), 'DELETE', '/api/v1/wish-patterns/7'],

    ['getSettings', () => api.getSettings(), 'GET', '/api/v1/settings'],
    ['saveSettings', () => api.saveSettings({}), 'PUT', '/api/v1/settings'],

    ['providerStatus', () => api.providerStatus(), 'GET', '/api/v1/provider/status'],
    ['providerTest', () => api.providerTest('972500000001'), 'POST', '/api/v1/provider/test'],

    ['notices', () => api.notices(), 'GET', '/api/v1/notices'],
    ['dismissNotice', () => api.dismissNotice(7), 'DELETE', '/api/v1/notices/7'],
  ]

  it.each(cases)('%s issues %s %s', async (_name, call, method, path) => {
    const calls = mockFetch({ body: {} })
    await call()
    expect(calls[0].init?.method).toBe(method)
    expect(calls[0].url).toBe(path)
  })
})
