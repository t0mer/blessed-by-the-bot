import { useCallback, useEffect, useRef, useState } from 'react'

interface Result<T> {
  data: T | undefined
  error: Error | undefined
  loading: boolean
  reload: () => void
}

/**
 * useApi loads data once and, when refreshMs is given, on an interval.
 *
 * Live surfaces (provider status, history, upcoming events) refresh on their own
 * rather than making the user reload — the spec asks for ~60s. A refresh never
 * clears what is already on screen, so the page does not flash back to a
 * skeleton every minute.
 */
export function useApi<T>(fn: () => Promise<T>, deps: unknown[] = [], refreshMs?: number): Result<T> {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<Error>()
  const [loading, setLoading] = useState(true)
  const [nonce, setNonce] = useState(0)

  // Keeping the latest fn in a ref lets the effect depend on `deps` alone, so an
  // inline arrow at the call site does not restart the interval every render.
  const fnRef = useRef(fn)
  fnRef.current = fn

  const reload = useCallback(() => setNonce((n) => n + 1), [])

  useEffect(() => {
    let cancelled = false

    const run = async (initial: boolean) => {
      if (initial) setLoading(true)
      try {
        const result = await fnRef.current()
        if (cancelled) return
        setData(result)
        setError(undefined)
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err : new Error(String(err)))
      } finally {
        if (!cancelled && initial) setLoading(false)
      }
    }

    void run(true)
    if (!refreshMs) return () => { cancelled = true }

    const timer = setInterval(() => void run(false), refreshMs)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, nonce, refreshMs])

  return { data, error, loading, reload }
}
