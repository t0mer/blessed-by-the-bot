import { describe, expect, it } from 'vitest'

import { cn, daysUntil, dirOf, formatDate, formatDateTime, isRTLText, nextOccurrence } from './utils'

describe('formatDate', () => {
  it('renders DD/MM/YYYY, the format the spec fixes', () => {
    expect(formatDate('1990-05-17')).toBe('17/05/1990')
    expect(formatDate('2026-12-31')).toBe('31/12/2026')
  })

  it('leaves an unparseable value alone rather than showing NaN', () => {
    expect(formatDate('not-a-date')).toBe('not-a-date')
    expect(formatDate('')).toBe('')
  })
})

describe('formatDateTime', () => {
  it('renders DD/MM/YYYY HH:MM in 24-hour form', () => {
    // Built from local components so the assertion holds in any timezone.
    const local = new Date(2026, 4, 17, 21, 5)
    expect(formatDateTime(local.toISOString())).toBe('17/05/2026 21:05')
  })

  it('zero-pads single digits', () => {
    const local = new Date(2026, 0, 2, 3, 4)
    expect(formatDateTime(local.toISOString())).toBe('02/01/2026 03:04')
  })

  it('passes a malformed timestamp straight through', () => {
    expect(formatDateTime('nonsense')).toBe('nonsense')
  })
})

describe('isRTLText / dirOf', () => {
  it('detects Hebrew', () => {
    expect(isRTLText('מזל טוב')).toBe(true)
    expect(dirOf('יום הולדת שמח')).toBe('rtl')
  })

  it('detects Arabic', () => {
    expect(isRTLText('مبروك')).toBe(true)
  })

  it('treats Latin text as left-to-right', () => {
    expect(isRTLText('Happy birthday')).toBe(false)
    expect(dirOf('Happy birthday')).toBe('ltr')
  })

  // A template is usually mixed: the placeholder and emoji are Latin/neutral.
  it('detects a mixed template by its Hebrew content', () => {
    expect(dirOf('מזל טוב {{name}}! 🎂')).toBe('rtl')
  })

  it('does not call an emoji-only or empty string RTL', () => {
    expect(isRTLText('🎂🎉')).toBe(false)
    expect(isRTLText('')).toBe(false)
  })
})

describe('nextOccurrence', () => {
  const from = (y: number, m: number, d: number) => new Date(y, m - 1, d)

  it('returns this year when the date is still ahead', () => {
    expect(nextOccurrence('1990-05-17', from(2026, 5, 1))).toEqual(from(2026, 5, 17))
  })

  it('returns today when the event is today', () => {
    expect(nextOccurrence('1990-05-17', from(2026, 5, 17))).toEqual(from(2026, 5, 17))
  })

  it('rolls to next year once the date has passed', () => {
    expect(nextOccurrence('1990-05-17', from(2026, 5, 18))).toEqual(from(2027, 5, 17))
  })

  it('crosses the year boundary', () => {
    expect(nextOccurrence('1990-01-01', from(2026, 12, 31))).toEqual(from(2027, 1, 1))
  })

  // This rule is duplicated from the Go scheduler (DueOn). If the two ever
  // disagree the UI shows a date the bot will not actually send on.
  it('observes a leap-day event on Feb 28 in a common year', () => {
    expect(nextOccurrence('1992-02-29', from(2026, 1, 1))).toEqual(from(2026, 2, 28))
  })

  it('observes it on Feb 29 in a leap year', () => {
    expect(nextOccurrence('1992-02-29', from(2024, 1, 1))).toEqual(from(2024, 2, 29))
  })

  it('applies the full Gregorian rule at century boundaries', () => {
    // 2100 divides by 4 but is not a leap year; 2000 divides by 100 but is.
    expect(nextOccurrence('1992-02-29', from(2100, 1, 1))).toEqual(from(2100, 2, 28))
    expect(nextOccurrence('1992-02-29', from(2000, 1, 1))).toEqual(from(2000, 2, 29))
  })

  it('does not shift a Feb 28 event onto Feb 29', () => {
    expect(nextOccurrence('1990-02-28', from(2024, 1, 1))).toEqual(from(2024, 2, 28))
  })
})

describe('daysUntil', () => {
  const day = (y: number, m: number, d: number) => new Date(y, m - 1, d)

  it('counts whole days', () => {
    expect(daysUntil(day(2026, 5, 20), day(2026, 5, 17))).toBe(3)
    expect(daysUntil(day(2026, 5, 17), day(2026, 5, 17))).toBe(0)
  })

  // Times of day must not round a boundary the wrong way: "tomorrow" at 23:00
  // is still 1, not 0.
  it('ignores the time of day', () => {
    const late = new Date(2026, 4, 17, 23, 59)
    const early = new Date(2026, 4, 18, 0, 1)
    expect(daysUntil(early, late)).toBe(1)
  })

  it('survives a daylight-saving transition', () => {
    // Europe/most zones shift in late March; the count must stay whole.
    expect(daysUntil(day(2026, 3, 30), day(2026, 3, 28))).toBe(2)
  })
})

describe('cn', () => {
  it('lets a caller override a component default', () => {
    expect(cn('p-2', 'p-4')).toBe('p-4')
  })

  it('drops falsy values', () => {
    expect(cn('a', false && 'b', undefined, 'c')).toBe('a c')
  })
})
