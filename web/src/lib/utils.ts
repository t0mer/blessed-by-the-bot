import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/** cn merges Tailwind classes, letting a caller override a component's defaults. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** formatDate renders DD/MM/YYYY, the format the spec fixes for this UI. */
export function formatDate(value: string): string {
  const [year, month, day] = value.split('-')
  if (!year || !month || !day) return value
  return `${day}/${month}/${year}`
}

/** formatDateTime renders DD/MM/YYYY HH:MM in 24-hour form, in local time. */
export function formatDateTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** isRTLText detects a right-to-left value so a blessing textarea can flip
 *  direction per-value — a Hebrew template must not render left-aligned just
 *  because the UI language is English. */
export function isRTLText(value: string): boolean {
  return /[֐-ࣿיִ-﷿ﹰ-﻿]/.test(value)
}

/** dirOf returns the `dir` attribute a free-text value should carry. */
export function dirOf(value: string): 'rtl' | 'ltr' {
  return isRTLText(value) ? 'rtl' : 'ltr'
}

/** nextOccurrence returns the next date this annual event falls on, applying the
 *  same Feb-29 → Feb-28 rule the scheduler uses so the UI cannot disagree with
 *  what actually gets sent. */
export function nextOccurrence(eventDate: string, from = new Date()): Date {
  const [, month, day] = eventDate.split('-').map(Number)
  if (!month || !day) return from

  const today = new Date(from.getFullYear(), from.getMonth(), from.getDate())
  for (const year of [from.getFullYear(), from.getFullYear() + 1]) {
    const d = observedDay(year, month, day)
    if (d >= today) return d
  }
  return observedDay(from.getFullYear() + 1, month, day)
}

function observedDay(year: number, month: number, day: number): Date {
  const isLeap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0)
  const observed = month === 2 && day === 29 && !isLeap ? 28 : day
  return new Date(year, month - 1, observed)
}

/** daysUntil counts whole days from today to the given date. */
export function daysUntil(target: Date, from = new Date()): number {
  const a = new Date(from.getFullYear(), from.getMonth(), from.getDate()).getTime()
  const b = new Date(target.getFullYear(), target.getMonth(), target.getDate()).getTime()
  return Math.round((b - a) / 86_400_000)
}
