import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { useApi } from './useApi'

function Probe({ fn, refreshMs }: { fn: () => Promise<string>; refreshMs?: number }) {
  const { data, error, loading, reload } = useApi(fn, [], refreshMs)
  return (
    <div>
      <span data-testid="state">
        {loading ? 'loading' : error ? `error:${error.message}` : (data ?? 'empty')}
      </span>
      <button onClick={reload}>reload</button>
    </div>
  )
}

describe('useApi', () => {
  it('reports loading, then the data', async () => {
    render(<Probe fn={async () => 'value'} />)
    expect(screen.getByTestId('state').textContent).toBe('loading')
    await waitFor(() => expect(screen.getByTestId('state').textContent).toBe('value'))
  })

  it('surfaces a failure as an Error', async () => {
    render(<Probe fn={async () => { throw new Error('boom') }} />)
    await waitFor(() => expect(screen.getByTestId('state').textContent).toBe('error:boom'))
  })

  // A rejected non-Error (a string, say) must not crash the consumer.
  it('wraps a non-Error rejection', async () => {
    // eslint-disable-next-line @typescript-eslint/only-throw-error
    render(<Probe fn={async () => { throw 'plain string' }} />)
    await waitFor(() => expect(screen.getByTestId('state').textContent).toBe('error:plain string'))
  })

  it('refetches on reload', async () => {
    let n = 0
    render(<Probe fn={async () => `call ${++n}`} />)
    await waitFor(() => expect(screen.getByTestId('state').textContent).toBe('call 1'))

    await userEvent.click(screen.getByRole('button', { name: 'reload' }))
    await waitFor(() => expect(screen.getByTestId('state').textContent).toBe('call 2'))
  })

  it('polls on an interval when one is given', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      let n = 0
      render(<Probe fn={async () => `call ${++n}`} refreshMs={50} />)
      await waitFor(() => expect(screen.getByTestId('state').textContent).toBe('call 1'))

      await vi.advanceTimersByTimeAsync(60)
      await waitFor(() => expect(screen.getByTestId('state').textContent).toBe('call 2'))
    } finally {
      vi.useRealTimers()
    }
  })

  // A refresh must not blank the page every minute; the previous value stays
  // on screen while the new one is in flight.
  it('does not flip back to loading on a refresh', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      let n = 0
      render(<Probe fn={async () => `call ${++n}`} refreshMs={50} />)
      await waitFor(() => expect(screen.getByTestId('state').textContent).toBe('call 1'))

      const states: string[] = []
      const observer = setInterval(() => {
        states.push(screen.getByTestId('state').textContent ?? '')
      }, 5)
      await vi.advanceTimersByTimeAsync(120)
      clearInterval(observer)

      expect(states).not.toContain('loading')
    } finally {
      vi.useRealTimers()
    }
  })

  it('stops polling once unmounted', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const fn = vi.fn(async () => 'value')
      const { unmount } = render(<Probe fn={fn} refreshMs={50} />)
      await waitFor(() => expect(fn).toHaveBeenCalledTimes(1))

      unmount()
      await vi.advanceTimersByTimeAsync(200)
      expect(fn).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })
})
