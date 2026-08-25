import { useEffect, useState } from 'react'

import {
  Badge, Button, Card, CardTitle, ErrorNote, Field, Input, Select, Spinner,
} from '@/components/ui'
import { ApiError, SECRET_MASK, api } from '@/lib/api'
import type { Settings as SettingsPayload } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import type { Language } from '@/lib/i18n'
import { useTheme } from '@/lib/theme'
import type { Theme } from '@/lib/theme'

export default function Settings() {
  const { t } = useI18n()
  const [settings, setSettings] = useState<SettingsPayload | null>(null)
  const [loadError, setLoadError] = useState<Error | null>(null)
  const [saving, setSaving] = useState(false)
  const [failure, setFailure] = useState<Error | null>(null)
  const [notice, setNotice] = useState('')

  const load = () => {
    setLoadError(null)
    api.getSettings().then(setSettings).catch((e: unknown) =>
      setLoadError(e instanceof Error ? e : new Error(String(e))),
    )
  }
  useEffect(load, [])

  if (loadError) {
    return <ErrorNote message={loadError.message} onRetry={load} retryLabel={t('common.retry')} />
  }
  if (!settings) return <Spinner label={t('common.loading')} />

  const patch = (next: Partial<SettingsPayload>) => setSettings({ ...settings, ...next })

  const save = async () => {
    setSaving(true)
    setFailure(null)
    setNotice('')
    try {
      // The whole document goes back, masks included. The API treats a mask as
      // "unchanged", so an untouched secret survives the round-trip.
      const { provider_error: _ignored, ...payload } = settings
      const saved = await api.saveSettings(payload)
      setSettings(saved)
      setNotice(saved.provider_error ? saved.provider_error : t('settings.saved'))
    } catch (err) {
      setFailure(err instanceof Error ? err : new Error(String(err)))
    } finally {
      setSaving(false)
    }
  }

  const fieldError = (name: string) =>
    failure instanceof ApiError ? failure.fieldMessage(name) : undefined

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">{t('settings.title')}</h1>

      {failure ? <ErrorNote message={failure.message} /> : null}
      {notice ? (
        <div className="rounded-lg border border-success/40 bg-success/10 p-3 text-sm text-success">{notice}</div>
      ) : null}

      <Appearance />

      <Card>
        <CardTitle>{t('settings.provider')}</CardTitle>
        <Field label={t('settings.provider')} error={fieldError('provider')}>
          <Select
            value={settings.provider}
            onChange={(e) => patch({ provider: e.target.value as SettingsPayload['provider'] })}
          >
            <option value="greenapi">{t('settings.greenapi')}</option>
            <option value="gowa">{t('settings.gowa')}</option>
          </Select>
        </Field>

        <div className="mt-4 space-y-3">
          {settings.provider === 'greenapi' ? (
            <GreenAPIFields settings={settings} patch={patch} />
          ) : (
            <GOWAFields settings={settings} patch={patch} />
          )}
        </div>
      </Card>

      <ProviderTools />

      <Card>
        <CardTitle>{t('settings.scheduler')}</CardTitle>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t('settings.timezone')}>
            <Input
              dir="ltr" value={settings.scheduler.timezone}
              onChange={(e) => patch({ scheduler: { ...settings.scheduler, timezone: e.target.value } })}
            />
          </Field>
          <Field label={t('settings.generalSendTime')}>
            <Input
              type="time" value={settings.scheduler.send_time}
              onChange={(e) => patch({ scheduler: { ...settings.scheduler, send_time: e.target.value } })}
            />
          </Field>
        </div>
      </Card>

      <Card>
        <CardTitle>{t('settings.groupEcho')}</CardTitle>
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label={t('settings.threshold')}>
            <Input
              type="number" min={1} value={settings.group_echo.threshold}
              onChange={(e) =>
                patch({ group_echo: { ...settings.group_echo, threshold: Number(e.target.value) } })
              }
            />
          </Field>
          <Field label={t('settings.window')}>
            <Input
              type="number" min={1} value={settings.group_echo.window_hours}
              onChange={(e) =>
                patch({ group_echo: { ...settings.group_echo, window_hours: Number(e.target.value) } })
              }
            />
          </Field>
          <Field label={t('settings.cooldown')}>
            <Input
              type="number" min={1} value={settings.group_echo.cooldown_hours}
              onChange={(e) =>
                patch({ group_echo: { ...settings.group_echo, cooldown_hours: Number(e.target.value) } })
              }
            />
          </Field>
        </div>
      </Card>

      <div className="sticky bottom-20 flex justify-end sm:bottom-4">
        <Button disabled={saving} onClick={() => void save()}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </div>
    </div>
  )
}

function GreenAPIFields({
  settings, patch,
}: {
  settings: SettingsPayload
  patch: (n: Partial<SettingsPayload>) => void
}) {
  const { t } = useI18n()
  const set = (k: keyof SettingsPayload['greenapi'], v: string) =>
    patch({ greenapi: { ...settings.greenapi, [k]: v } })

  return (
    <>
      <Field label={t('settings.apiUrl')}>
        <Input dir="ltr" value={settings.greenapi.api_url} onChange={(e) => set('api_url', e.target.value)} />
      </Field>
      <Field label={t('settings.idInstance')}>
        <Input dir="ltr" value={settings.greenapi.id_instance} onChange={(e) => set('id_instance', e.target.value)} />
      </Field>
      <SecretField
        label={t('settings.apiToken')}
        value={settings.greenapi.api_token}
        onChange={(v) => set('api_token', v)}
      />
      <Field label={t('settings.mode')}>
        <Select value={settings.greenapi.mode} onChange={(e) => set('mode', e.target.value)}>
          <option value="polling">{t('settings.modePolling')}</option>
          <option value="webhook">{t('settings.modeWebhook')}</option>
        </Select>
      </Field>
      {settings.greenapi.mode === 'webhook' ? (
        <SecretField
          label={t('settings.webhookAuth')}
          value={settings.greenapi.webhook_auth_header}
          onChange={(v) => set('webhook_auth_header', v)}
        />
      ) : null}
    </>
  )
}

function GOWAFields({
  settings, patch,
}: {
  settings: SettingsPayload
  patch: (n: Partial<SettingsPayload>) => void
}) {
  const { t } = useI18n()
  const set = (k: keyof SettingsPayload['gowa'], v: string) =>
    patch({ gowa: { ...settings.gowa, [k]: v } })

  return (
    <>
      <Field label={t('settings.baseUrl')}>
        <Input dir="ltr" value={settings.gowa.base_url} onChange={(e) => set('base_url', e.target.value)} />
      </Field>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={`${t('settings.username')} (${t('common.optional')})`}>
          <Input dir="ltr" value={settings.gowa.username} onChange={(e) => set('username', e.target.value)} />
        </Field>
        <SecretField
          label={`${t('settings.password')} (${t('common.optional')})`}
          value={settings.gowa.password}
          onChange={(v) => set('password', v)}
        />
      </div>
      <Field label={t('settings.deviceId')}>
        <Input dir="ltr" value={settings.gowa.device_id} onChange={(e) => set('device_id', e.target.value)} />
      </Field>
      <SecretField
        label={t('settings.webhookSecret')}
        hint={t('settings.webhookSecretHint')}
        value={settings.gowa.webhook_secret}
        onChange={(v) => set('webhook_secret', v)}
      />
    </>
  )
}

/**
 * SecretField renders a stored credential without ever revealing it.
 *
 * The API returns the mask for a secret that is set. Editing replaces it;
 * leaving it alone sends the mask back, which the API reads as "unchanged".
 */
function SecretField({
  label, hint, value, onChange,
}: {
  label: string
  hint?: string
  value: string
  onChange: (v: string) => void
}) {
  const { t } = useI18n()
  const isStored = value === SECRET_MASK

  return (
    <Field label={label} hint={isStored ? t('settings.secretKept') : hint}>
      <div className="flex items-center gap-2">
        <Input
          dir="ltr"
          type={isStored ? 'text' : 'password'}
          value={value}
          onFocus={(e) => {
            // Clear the mask on focus so typing replaces rather than appends.
            if (isStored) {
              onChange('')
              e.currentTarget.type = 'password'
            }
          }}
          onChange={(e) => onChange(e.target.value)}
        />
        {isStored ? <Badge tone="success">{t('common.enabled')}</Badge> : null}
      </div>
    </Field>
  )
}

function ProviderTools() {
  const { t } = useI18n()
  const [phone, setPhone] = useState('')
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState('')
  const [failed, setFailed] = useState(false)

  const check = async () => {
    setBusy(true)
    setFailed(false)
    try {
      const status = await api.providerStatus()
      setResult(`${status.provider}: ${status.state}${status.connected ? ' ✓' : ''}`)
    } catch (err) {
      setFailed(true)
      setResult(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const sendTest = async () => {
    setBusy(true)
    setFailed(false)
    try {
      const res = await api.providerTest(phone)
      setResult(`${res.chat_id} → ${res.message_id}`)
    } catch (err) {
      setFailed(true)
      setResult(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardTitle>{t('settings.test')}</CardTitle>
      <p className="mb-3 text-xs text-muted-foreground">
        {/* Save first: these act on the stored configuration, not the form. */}
        {t('settings.secretKept')}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" disabled={busy} onClick={() => void check()}>
          {t('settings.test')}
        </Button>
        <Input
          className="min-w-40 grow sm:max-w-xs" dir="ltr" placeholder={t('settings.testPhone')}
          value={phone} onChange={(e) => setPhone(e.target.value)}
        />
        <Button variant="secondary" disabled={busy || !phone.trim()} onClick={() => void sendTest()}>
          {t('settings.sendTest')}
        </Button>
      </div>
      {result ? (
        <p className={`mt-3 break-words text-sm ${failed ? 'text-danger' : 'text-success'}`} dir="ltr">
          {result}
        </p>
      ) : null}
    </Card>
  )
}

function Appearance() {
  const { t, language, setLanguage } = useI18n()
  const { theme, setTheme } = useTheme()

  return (
    <Card>
      <CardTitle>{t('settings.uiLanguage')}</CardTitle>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t('settings.uiLanguage')}>
          <Select value={language} onChange={(e) => setLanguage(e.target.value as Language)}>
            <option value="en">English</option>
            <option value="he">עברית</option>
          </Select>
        </Field>
        <Field label={t('settings.theme')}>
          <Select value={theme} onChange={(e) => setTheme(e.target.value as Theme)}>
            <option value="system">{t('settings.themeSystem')}</option>
            <option value="light">{t('settings.themeLight')}</option>
            <option value="dark">{t('settings.themeDark')}</option>
          </Select>
        </Field>
      </div>
    </Card>
  )
}
