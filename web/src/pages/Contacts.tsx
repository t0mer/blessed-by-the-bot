import { useMemo, useState } from 'react'
import { Pencil, Send, Trash2 } from 'lucide-react'

import {
  Badge, Button, Card, Dialog, EmptyState, ErrorNote, Field, Input, Select, Spinner, Switch,
} from '@/components/ui'
import { ApiError, api } from '@/lib/api'
import type { Contact, EventType, Gender, Relation } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useApi } from '@/lib/useApi'
import { formatDate } from '@/lib/utils'

const eventTypes: EventType[] = ['birthday', 'wedding', 'anniversary', 'custom']
const genders: Gender[] = ['male', 'female', 'other']
const relations: Relation[] = ['friend', 'close_friend', 'family', 'coworker']

type Draft = {
  name: string
  phone: string
  event_date: string
  event_type: EventType
  language: string
  relation: Relation
  gender: Gender
  importance: number
  send_time: string
  useSendTime: boolean
  enabled: boolean
}

const emptyDraft: Draft = {
  name: '', phone: '', event_date: '', event_type: 'birthday', language: 'he',
  relation: 'friend', gender: 'female', importance: 3, send_time: '09:00',
  useSendTime: false, enabled: true,
}

function toDraft(c: Contact): Draft {
  return {
    name: c.name, phone: c.phone, event_date: c.event_date, event_type: c.event_type,
    language: c.language, relation: c.relation, gender: c.gender, importance: c.importance,
    send_time: c.send_time ?? '09:00', useSendTime: c.send_time !== null, enabled: c.enabled,
  }
}

export default function Contacts() {
  const { t } = useI18n()
  const { data, error, loading, reload } = useApi(() => api.listContacts(), [])

  const [search, setSearch] = useState('')
  const [typeFilter, setTypeFilter] = useState<'' | EventType>('')
  const [editing, setEditing] = useState<Contact | null>(null)
  const [creating, setCreating] = useState(false)
  const [notice, setNotice] = useState('')

  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase()
    return (data ?? []).filter((c) => {
      if (typeFilter && c.event_type !== typeFilter) return false
      if (!needle) return true
      return c.name.toLowerCase().includes(needle) || c.phone.includes(needle)
    })
  }, [data, search, typeFilter])

  const remove = async (c: Contact) => {
    if (!confirm(t('contacts.confirmDelete'))) return
    await api.deleteContact(c.id)
    reload()
  }

  const sendNow = async (c: Contact) => {
    try {
      await api.sendNow(c.id)
      setNotice(t('contacts.sendNowDone'))
      reload()
    } catch (err) {
      setNotice(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-xl font-semibold">{t('contacts.title')}</h1>
        <Button className="ms-auto" onClick={() => setCreating(true)}>
          {t('contacts.add')}
        </Button>
      </div>

      {notice ? <ErrorNote message={notice} onRetry={() => setNotice('')} retryLabel={t('common.close')} /> : null}

      <div className="flex flex-wrap gap-2">
        <Input
          className="sm:max-w-xs"
          placeholder={t('common.search')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Select
          className="sm:max-w-[12rem]"
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value as '' | EventType)}
        >
          <option value="">{t('common.all')}</option>
          {eventTypes.map((v) => (
            <option key={v} value={v}>{t(`event.${v}`)}</option>
          ))}
        </Select>
      </div>

      {loading ? (
        <Spinner label={t('common.loading')} />
      ) : error ? (
        <ErrorNote message={error.message} onRetry={reload} retryLabel={t('common.retry')} />
      ) : (data ?? []).length === 0 ? (
        <EmptyState
          title={t('contacts.empty')}
          action={<Button onClick={() => setCreating(true)}>{t('contacts.add')}</Button>}
        />
      ) : filtered.length === 0 ? (
        <EmptyState title={t('contacts.noMatch')} />
      ) : (
        <ul className="space-y-2">
          {filtered.map((c) => (
            <Card key={c.id} className="p-3 sm:p-4">
              <div className="flex flex-wrap items-center gap-2">
                <div className="min-w-0 grow">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium">{c.name}</span>
                    {!c.enabled ? <Badge>{t('common.disabled')}</Badge> : null}
                  </div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <span dir="ltr">{c.phone}</span>
                    <span>{formatDate(c.event_date)}</span>
                    <span>{t(`event.${c.event_type}`)}</span>
                    <span>{t(`relation.${c.relation}`)}</span>
                    {c.send_time ? <span dir="ltr">{c.send_time}</span> : null}
                  </div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button variant="ghost" size="icon" aria-label={t('contacts.sendNow')} onClick={() => void sendNow(c)}>
                    <Send size={16} />
                  </Button>
                  <Button variant="ghost" size="icon" aria-label={t('common.edit')} onClick={() => setEditing(c)}>
                    <Pencil size={16} />
                  </Button>
                  <Button variant="ghost" size="icon" aria-label={t('common.delete')} onClick={() => void remove(c)}>
                    <Trash2 size={16} />
                  </Button>
                </div>
              </div>
            </Card>
          ))}
        </ul>
      )}

      <ContactDialog
        open={creating || editing !== null}
        contact={editing}
        onClose={() => {
          setCreating(false)
          setEditing(null)
        }}
        onSaved={() => {
          setCreating(false)
          setEditing(null)
          reload()
        }}
      />
    </div>
  )
}

function ContactDialog({
  open, contact, onClose, onSaved,
}: {
  open: boolean
  contact: Contact | null
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [saving, setSaving] = useState(false)
  const [failure, setFailure] = useState<ApiError | Error | null>(null)
  const [key, setKey] = useState(0)

  // Reset the form whenever the dialog opens for a different row.
  const wanted = contact ? contact.id : 0
  if (open && key !== wanted + 1) {
    setKey(wanted + 1)
    setDraft(contact ? toDraft(contact) : emptyDraft)
    setFailure(null)
  }

  const set = <K extends keyof Draft>(field: K, value: Draft[K]) =>
    setDraft((d) => ({ ...d, [field]: value }))

  const fieldError = (name: string) =>
    failure instanceof ApiError ? failure.fieldMessage(name) : undefined

  const submit = async () => {
    setSaving(true)
    setFailure(null)
    try {
      const payload = {
        name: draft.name,
        phone: draft.phone,
        event_date: draft.event_date,
        event_type: draft.event_type,
        language: draft.language,
        relation: draft.relation,
        gender: draft.gender,
        importance: draft.importance,
        send_time: draft.useSendTime ? draft.send_time : null,
        enabled: draft.enabled,
      }
      if (contact) await api.updateContact(contact.id, payload)
      else await api.createContact(payload)
      onSaved()
    } catch (err) {
      setFailure(err instanceof Error ? err : new Error(String(err)))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onClose={onClose} title={contact ? t('common.edit') : t('contacts.add')}>
      <div className="space-y-3">
        {failure && !(failure instanceof ApiError && failure.fields.length > 0) ? (
          <ErrorNote message={failure.message} />
        ) : null}

        <Field label={t('common.name')} error={fieldError('name')}>
          <Input value={draft.name} onChange={(e) => set('name', e.target.value)} />
        </Field>

        <Field label={t('contacts.phone')} hint={t('contacts.phoneHint')} error={fieldError('phone')}>
          <Input dir="ltr" inputMode="tel" value={draft.phone} onChange={(e) => set('phone', e.target.value)} />
        </Field>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('contacts.eventDate')} error={fieldError('event_date')}>
            <Input type="date" value={draft.event_date} onChange={(e) => set('event_date', e.target.value)} />
          </Field>
          <Field label={t('contacts.eventType')} error={fieldError('event_type')}>
            <Select value={draft.event_type} onChange={(e) => set('event_type', e.target.value as EventType)}>
              {eventTypes.map((v) => (
                <option key={v} value={v}>{t(`event.${v}`)}</option>
              ))}
            </Select>
          </Field>
          <Field label={t('contacts.gender')} error={fieldError('gender')}>
            <Select value={draft.gender} onChange={(e) => set('gender', e.target.value as Gender)}>
              {genders.map((v) => (
                <option key={v} value={v}>{t(`gender.${v}`)}</option>
              ))}
            </Select>
          </Field>
          <Field label={t('contacts.relation')} error={fieldError('relation')}>
            <Select value={draft.relation} onChange={(e) => set('relation', e.target.value as Relation)}>
              {relations.map((v) => (
                <option key={v} value={v}>{t(`relation.${v}`)}</option>
              ))}
            </Select>
          </Field>
          <Field label={t('common.language')} error={fieldError('language')}>
            <Input dir="ltr" value={draft.language} onChange={(e) => set('language', e.target.value)} />
          </Field>
          <Field label={t('contacts.importance')} error={fieldError('importance')}>
            <Input
              type="number" min={1} max={5} value={draft.importance}
              onChange={(e) => set('importance', Number(e.target.value))}
            />
          </Field>
        </div>

        <div className="flex items-center gap-3">
          <Switch
            checked={draft.useSendTime}
            onChange={(v) => set('useSendTime', v)}
            label={t('contacts.customSendTime')}
          />
          <span className="text-sm">{t('contacts.customSendTime')}</span>
        </div>
        {draft.useSendTime ? (
          <Field label={t('contacts.sendTime')} error={fieldError('send_time')}>
            <Input type="time" value={draft.send_time} onChange={(e) => set('send_time', e.target.value)} />
          </Field>
        ) : null}

        <div className="flex items-center gap-3">
          <Switch checked={draft.enabled} onChange={(v) => set('enabled', v)} label={t('common.enabled')} />
          <span className="text-sm">{t('common.enabled')}</span>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <Button variant="secondary" onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={saving} onClick={() => void submit()}>
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </div>
      </div>
    </Dialog>
  )
}
