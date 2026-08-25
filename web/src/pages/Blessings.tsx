import { useMemo, useState } from 'react'
import { Pencil, Trash2 } from 'lucide-react'

import {
  Badge, Button, Card, Dialog, EmptyState, ErrorNote, Field, Input, Select, Spinner, Switch, Textarea,
} from '@/components/ui'
import { ApiError, api } from '@/lib/api'
import type { Blessing, EventType, Gender, Relation } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useApi } from '@/lib/useApi'
import { cn, dirOf } from '@/lib/utils'

const eventTypes: EventType[] = ['birthday', 'wedding', 'anniversary', 'custom']
const genders: Gender[] = ['male', 'female', 'other']
const relations: Relation[] = ['friend', 'close_friend', 'family', 'coworker']

const PLACEHOLDER = '{{name}}'
const SAMPLE_NAME = 'Dana'

type Draft = {
  event_type: EventType
  language: string
  gender: '' | Gender
  relation: '' | Relation
  text: string
  enabled: boolean
}

const emptyDraft: Draft = {
  event_type: 'birthday', language: 'he', gender: '', relation: '', text: '', enabled: true,
}

export default function Blessings() {
  const { t } = useI18n()
  const { data, error, loading, reload } = useApi(() => api.listBlessings(), [])

  const [tab, setTab] = useState<EventType>('birthday')
  const [editing, setEditing] = useState<Blessing | null>(null)
  const [creating, setCreating] = useState(false)

  const grouped = useMemo(() => (data ?? []).filter((b) => b.event_type === tab), [data, tab])

  const remove = async (b: Blessing) => {
    if (!confirm(t('blessings.confirmDelete'))) return
    await api.deleteBlessing(b.id)
    reload()
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-xl font-semibold">{t('blessings.title')}</h1>
        <Button className="ms-auto" onClick={() => setCreating(true)}>{t('blessings.add')}</Button>
      </div>

      {/* Event type is the primary axis: a user edits "birthday templates", not
          a flat list of everything. */}
      <div className="flex gap-1 overflow-x-auto rounded-lg bg-muted p-1">
        {eventTypes.map((v) => (
          <button
            key={v}
            onClick={() => setTab(v)}
            className={cn(
              'shrink-0 rounded-md px-3 py-1.5 text-sm font-medium transition',
              tab === v ? 'bg-card shadow-sm' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t(`event.${v}`)}
          </button>
        ))}
      </div>

      {loading ? (
        <Spinner label={t('common.loading')} />
      ) : error ? (
        <ErrorNote message={error.message} onRetry={reload} retryLabel={t('common.retry')} />
      ) : grouped.length === 0 ? (
        <EmptyState
          title={t('blessings.empty')}
          action={<Button onClick={() => setCreating(true)}>{t('blessings.add')}</Button>}
        />
      ) : (
        <ul className="space-y-2">
          {grouped.map((b) => (
            <Card key={b.id} className="p-3 sm:p-4">
              <div className="flex items-start gap-2">
                <div className="min-w-0 grow">
                  <p className="whitespace-pre-wrap break-words text-sm" dir={dirOf(b.text)}>
                    {b.text}
                  </p>
                  <div className="mt-2 flex flex-wrap items-center gap-1.5">
                    <Badge>{b.language}</Badge>
                    {b.gender ? <Badge>{t(`gender.${b.gender}`)}</Badge> : null}
                    {b.relation ? <Badge>{t(`relation.${b.relation}`)}</Badge> : null}
                    {!b.text.includes(PLACEHOLDER) ? (
                      <Badge tone="success">{t('blessings.groupUsable')}</Badge>
                    ) : null}
                    {!b.enabled ? <Badge tone="danger">{t('common.disabled')}</Badge> : null}
                  </div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button variant="ghost" size="icon" aria-label={t('common.edit')} onClick={() => setEditing(b)}>
                    <Pencil size={16} />
                  </Button>
                  <Button variant="ghost" size="icon" aria-label={t('common.delete')} onClick={() => void remove(b)}>
                    <Trash2 size={16} />
                  </Button>
                </div>
              </div>
            </Card>
          ))}
        </ul>
      )}

      <BlessingDialog
        open={creating || editing !== null}
        blessing={editing}
        defaultType={tab}
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

function BlessingDialog({
  open, blessing, defaultType, onClose, onSaved,
}: {
  open: boolean
  blessing: Blessing | null
  defaultType: EventType
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [saving, setSaving] = useState(false)
  const [failure, setFailure] = useState<Error | null>(null)
  const [key, setKey] = useState(0)

  const wanted = blessing ? blessing.id : 0
  if (open && key !== wanted + 1) {
    setKey(wanted + 1)
    setDraft(
      blessing
        ? {
            event_type: blessing.event_type, language: blessing.language,
            gender: blessing.gender ?? '', relation: blessing.relation ?? '',
            text: blessing.text, enabled: blessing.enabled,
          }
        : { ...emptyDraft, event_type: defaultType },
    )
    setFailure(null)
  }

  const set = <K extends keyof Draft>(field: K, value: Draft[K]) =>
    setDraft((d) => ({ ...d, [field]: value }))

  const fieldError = (name: string) =>
    failure instanceof ApiError ? failure.fieldMessage(name) : undefined

  const preview = draft.text.replaceAll(PLACEHOLDER, SAMPLE_NAME)
  const nameFree = !draft.text.includes(PLACEHOLDER)

  const submit = async () => {
    setSaving(true)
    setFailure(null)
    try {
      const payload = {
        event_type: draft.event_type,
        language: draft.language,
        gender: draft.gender === '' ? null : draft.gender,
        relation: draft.relation === '' ? null : draft.relation,
        text: draft.text,
        enabled: draft.enabled,
      }
      if (blessing) await api.updateBlessing(blessing.id, payload)
      else await api.createBlessing(payload)
      onSaved()
    } catch (err) {
      setFailure(err instanceof Error ? err : new Error(String(err)))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onClose={onClose} title={blessing ? t('common.edit') : t('blessings.add')}>
      <div className="space-y-3">
        {failure && !(failure instanceof ApiError && failure.fields.length > 0) ? (
          <ErrorNote message={failure.message} />
        ) : null}

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('contacts.eventType')} error={fieldError('event_type')}>
            <Select value={draft.event_type} onChange={(e) => set('event_type', e.target.value as EventType)}>
              {eventTypes.map((v) => (
                <option key={v} value={v}>{t(`event.${v}`)}</option>
              ))}
            </Select>
          </Field>
          <Field label={t('common.language')} error={fieldError('language')}>
            <Input dir="ltr" value={draft.language} onChange={(e) => set('language', e.target.value)} />
          </Field>
        </div>

        <Field label={t('blessings.text')} hint={t('blessings.placeholderHint')} error={fieldError('text')}>
          {/* Direction follows the value, not the UI language: a Hebrew template
              must read correctly even with the interface in English. */}
          <Textarea
            rows={4}
            dir={dirOf(draft.text)}
            value={draft.text}
            onChange={(e) => set('text', e.target.value)}
          />
        </Field>

        {draft.text ? (
          <div className="rounded-lg bg-muted p-3">
            <div className="mb-1 text-xs font-medium text-muted-foreground">{t('blessings.preview')}</div>
            <p className="whitespace-pre-wrap break-words text-sm" dir={dirOf(preview)}>{preview}</p>
            {nameFree ? (
              <p className="mt-2 text-xs text-success">{t('blessings.groupUsableHint')}</p>
            ) : null}
          </div>
        ) : null}

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('contacts.gender')} error={fieldError('gender')}>
            <Select value={draft.gender} onChange={(e) => set('gender', e.target.value as '' | Gender)}>
              <option value="">{t('blessings.anyGender')}</option>
              {genders.map((v) => (
                <option key={v} value={v}>{t(`gender.${v}`)}</option>
              ))}
            </Select>
          </Field>
          <Field label={t('contacts.relation')} error={fieldError('relation')}>
            <Select value={draft.relation} onChange={(e) => set('relation', e.target.value as '' | Relation)}>
              <option value="">{t('blessings.anyRelation')}</option>
              {relations.map((v) => (
                <option key={v} value={v}>{t(`relation.${v}`)}</option>
              ))}
            </Select>
          </Field>
        </div>

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
