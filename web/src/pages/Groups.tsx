import { useState } from 'react'
import { Pencil, Trash2 } from 'lucide-react'

import {
  Badge, Button, Card, CardTitle, Dialog, EmptyState, ErrorNote, Field, Input, Select, Spinner, Switch,
} from '@/components/ui'
import { ApiError, api } from '@/lib/api'
import type { AvailableGroup, Group, WishPattern } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { useApi } from '@/lib/useApi'
import { dirOf } from '@/lib/utils'

export default function Groups() {
  const { t } = useI18n()
  const groups = useApi(() => api.listGroups(), [])
  const [editing, setEditing] = useState<Group | null>(null)
  const [creating, setCreating] = useState(false)

  const remove = async (g: Group) => {
    if (!confirm(t('groups.confirmDelete'))) return
    await api.deleteGroup(g.id)
    groups.reload()
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-xl font-semibold">{t('groups.title')}</h1>
        <Button className="ms-auto" onClick={() => setCreating(true)}>{t('groups.add')}</Button>
      </div>

      {groups.loading ? (
        <Spinner label={t('common.loading')} />
      ) : groups.error ? (
        <ErrorNote message={groups.error.message} onRetry={groups.reload} retryLabel={t('common.retry')} />
      ) : (groups.data ?? []).length === 0 ? (
        <EmptyState
          title={t('groups.empty')}
          action={<Button onClick={() => setCreating(true)}>{t('groups.add')}</Button>}
        />
      ) : (
        <ul className="space-y-2">
          {(groups.data ?? []).map((g) => (
            <Card key={g.id} className="p-3 sm:p-4">
              <div className="flex flex-wrap items-center gap-2">
                <div className="min-w-0 grow">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium">{g.name}</span>
                    {!g.enabled ? <Badge>{t('common.disabled')}</Badge> : null}
                  </div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <span className="truncate font-mono" dir="ltr">{g.chat_id}</span>
                    <span>{g.language}</span>
                    <span>
                      {t('groups.threshold')}: {g.threshold ?? t('groups.useDefault')}
                    </span>
                  </div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button variant="ghost" size="icon" aria-label={t('common.edit')} onClick={() => setEditing(g)}>
                    <Pencil size={16} />
                  </Button>
                  <Button variant="ghost" size="icon" aria-label={t('common.delete')} onClick={() => void remove(g)}>
                    <Trash2 size={16} />
                  </Button>
                </div>
              </div>
            </Card>
          ))}
        </ul>
      )}

      <WishPatterns />

      <GroupDialog
        open={creating || editing !== null}
        group={editing}
        onClose={() => {
          setCreating(false)
          setEditing(null)
        }}
        onSaved={() => {
          setCreating(false)
          setEditing(null)
          groups.reload()
        }}
      />
    </div>
  )
}

function GroupDialog({
  open, group, onClose, onSaved,
}: {
  open: boolean
  group: Group | null
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useI18n()
  const [name, setName] = useState('')
  const [chatID, setChatID] = useState('')
  const [language, setLanguage] = useState('he')
  const [threshold, setThreshold] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [saving, setSaving] = useState(false)
  const [failure, setFailure] = useState<Error | null>(null)
  const [key, setKey] = useState(0)

  // Offered only when the provider can actually list groups; a 503 on a fresh
  // install is expected and simply leaves manual entry as the path.
  const available = useApi(() => api.availableGroups().catch(() => null), [key])

  const wanted = group ? group.id : 0
  if (open && key !== wanted + 1) {
    setKey(wanted + 1)
    setName(group?.name ?? '')
    setChatID(group?.chat_id ?? '')
    setLanguage(group?.language ?? 'he')
    setThreshold(group?.threshold != null ? String(group.threshold) : '')
    setEnabled(group?.enabled ?? true)
    setFailure(null)
  }

  const fieldError = (field: string) =>
    failure instanceof ApiError ? failure.fieldMessage(field) : undefined

  const pick = (g: AvailableGroup) => {
    setChatID(g.chat_id)
    if (!name) setName(g.name)
  }

  const submit = async () => {
    setSaving(true)
    setFailure(null)
    try {
      const payload = {
        name,
        chat_id: chatID,
        language,
        threshold: threshold.trim() === '' ? null : Number(threshold),
        enabled,
      }
      if (group) await api.updateGroup(group.id, payload)
      else await api.createGroup(payload)
      onSaved()
    } catch (err) {
      setFailure(err instanceof Error ? err : new Error(String(err)))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onClose={onClose} title={group ? t('common.edit') : t('groups.add')}>
      <div className="space-y-3">
        {failure && !(failure instanceof ApiError && failure.fields.length > 0) ? (
          <ErrorNote message={failure.message} />
        ) : null}

        {available.data && available.data.length > 0 ? (
          <Field label={t('groups.pick')}>
            <Select
              value=""
              onChange={(e) => {
                const found = available.data?.find((g) => g.chat_id === e.target.value)
                if (found) pick(found)
              }}
            >
              <option value="">{t('groups.manual')}</option>
              {available.data.map((g) => (
                <option key={g.chat_id} value={g.chat_id}>{g.name}</option>
              ))}
            </Select>
          </Field>
        ) : (
          <p className="text-xs text-muted-foreground">{t('groups.pickUnavailable')}</p>
        )}

        <Field label={t('common.name')} error={fieldError('name')}>
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>

        <Field label={t('groups.chatId')} error={fieldError('chat_id')}>
          <Input dir="ltr" className="font-mono" value={chatID} onChange={(e) => setChatID(e.target.value)} />
        </Field>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('common.language')} error={fieldError('language')}>
            <Input dir="ltr" value={language} onChange={(e) => setLanguage(e.target.value)} />
          </Field>
          <Field label={t('groups.threshold')} hint={t('groups.thresholdHint')} error={fieldError('threshold')}>
            <Input
              type="number" min={1} placeholder={t('groups.useDefault')}
              value={threshold} onChange={(e) => setThreshold(e.target.value)}
            />
          </Field>
        </div>

        <div className="flex items-center gap-3">
          <Switch checked={enabled} onChange={setEnabled} label={t('common.enabled')} />
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

function WishPatterns() {
  const { t } = useI18n()
  const patterns = useApi(() => api.listWishPatterns(), [])
  const [language, setLanguage] = useState('he')
  const [value, setValue] = useState('')
  const [failure, setFailure] = useState<Error | null>(null)

  const add = async () => {
    setFailure(null)
    try {
      await api.createWishPattern({ language, pattern: value })
      setValue('')
      patterns.reload()
    } catch (err) {
      setFailure(err instanceof Error ? err : new Error(String(err)))
    }
  }

  const toggle = async (p: WishPattern) => {
    await api.updateWishPattern(p.id, { language: p.language, pattern: p.pattern, enabled: !p.enabled })
    patterns.reload()
  }

  const remove = async (p: WishPattern) => {
    await api.deleteWishPattern(p.id)
    patterns.reload()
  }

  return (
    <Card>
      <CardTitle>{t('groups.patterns')}</CardTitle>
      <p className="mb-3 text-xs text-muted-foreground">{t('groups.patternsHint')}</p>

      {failure ? <div className="mb-3"><ErrorNote message={failure.message} /></div> : null}

      <div className="mb-3 flex flex-wrap gap-2">
        <Input className="w-20" dir="ltr" value={language} onChange={(e) => setLanguage(e.target.value)} />
        <Input
          className="min-w-40 grow" dir={dirOf(value)} value={value}
          placeholder={t('groups.pattern')} onChange={(e) => setValue(e.target.value)}
        />
        <Button onClick={() => void add()} disabled={!value.trim()}>{t('common.add')}</Button>
      </div>

      {patterns.loading ? (
        <Spinner label={t('common.loading')} />
      ) : (
        <ul className="divide-y">
          {(patterns.data ?? []).map((p) => (
            <li key={p.id} className="flex items-center gap-2 py-2">
              <Badge>{p.language}</Badge>
              <span className="grow truncate text-sm" dir={dirOf(p.pattern)}>{p.pattern}</span>
              <Switch checked={p.enabled} onChange={() => void toggle(p)} label={t('common.enabled')} />
              <Button variant="ghost" size="icon" aria-label={t('common.delete')} onClick={() => void remove(p)}>
                <Trash2 size={16} />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}
