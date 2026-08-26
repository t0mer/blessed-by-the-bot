# REST API

`blessedbot` serves a JSON API under **`/api/v1`** and two provider webhook
endpoints under **`/webhooks`**. The embedded SPA is a client of this API — every
action available in the UI has an endpoint here.

- **Base URL:** `http://<host>:<port>/api/v1` (default port `8080`)
- **Content type:** `application/json` for requests and responses
- **Authentication:** none in v1. The application is meant for a LAN or
  home-lab deployment. The middleware chain is structured so basic auth can be
  added to the API router later without touching the webhook router.

All examples below are real responses captured from a running instance.

---

## Error envelope

Every error — validation, not-found, provider failure — uses one shape:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "one or more fields are invalid",
    "fields": [
      {"field": "name",  "message": "is required"},
      {"field": "phone", "message": "must be an international number with 8-15 digits, no + or spaces"}
    ]
  }
}
```

`fields` is present only for errors that can name specific request fields.

| Code | Status | Meaning |
|---|---|---|
| `not_found` | 404 | No such resource. |
| `method_not_allowed` | 405 | Wrong verb for the path. |
| `invalid_json` | 400 / 413 | Malformed body, empty or `null` body, unknown field, wrong field type, or oversized body (limit 1 MiB). |
| `invalid_id` | 400 | The `{id}` path segment is not a positive integer. |
| `invalid_query` | 400 | A query parameter is malformed. |
| `validation_failed` | 422 | One or more fields were rejected; see `fields`. |
| `conflict` | 409 | A uniqueness constraint was violated. |
| `already_sent` | 409 | This contact already got their blessing this event year; use `force=true`. |
| `contact_disabled` | 409 | The contact is disabled, so nothing was sent. |
| `no_blessing` | 422 | No template matches this contact — add one under Blessings. |
| `unauthorized` | 401 | Webhook authentication failed. |
| `provider_unavailable` | 503 | No WhatsApp provider is configured. |
| `provider_failed` | 502 | The provider was reached but returned an error. |
| `not_implemented` | 501 | The feature's engine is not running yet. |
| `internal_error` | 500 | Unexpected server fault. Details are logged, never returned. |

Unknown fields are rejected rather than ignored, so a typo surfaces immediately
instead of being silently dropped:

```console
$ curl -X POST -d '{"name":"X","nickname":"Y"}' .../api/v1/contacts
{"error":{"code":"invalid_json","message":"unknown field nickname","fields":[{"field":"nickname","message":"unknown field"}]}}
```

---

## Health

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/healthz` | Process and database liveness. |
| GET | `/healthz` | Same handler, outside `/api/v1`, for the container healthcheck. |

```console
$ curl .../healthz
{"database":"ok","status":"ok","version":"dev"}
```

Returns `503` with `"status":"error"` when the database is unreachable.

---

## Contacts

A contact is a person with a recurring event.

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/contacts` | List all contacts, ordered by name. |
| POST | `/api/v1/contacts` | Create a contact. `201` on success. |
| GET | `/api/v1/contacts/{id}` | Fetch one contact. |
| PUT | `/api/v1/contacts/{id}` | Replace a contact. Full body required. |
| DELETE | `/api/v1/contacts/{id}` | Delete a contact. `204` on success. |
| POST | `/api/v1/contacts/{id}/send-now` | Send immediately — see [Send now](#send-now). |

### Request body

| Field | Type | Required | Default | Constraint |
|---|---|---|---|---|
| `name` | string | yes | — | Non-blank. |
| `phone` | string | yes | — | International number. Punctuation, spaces and a leading `+` are stripped; the result must be 8–15 digits. |
| `event_date` | string | yes | — | `YYYY-MM-DD`, zero-padded, a real calendar date with a year between 1900 and 2200. The year is kept for age arithmetic. |
| `event_type` | string | yes | — | `birthday`, `wedding`, `anniversary`, `custom`. |
| `language` | string | yes | — | Language tag: `he`, `en`, `pt-BR`. |
| `relation` | string | yes | — | `friend`, `close_friend`, `family`, `coworker`. |
| `gender` | string | yes | — | `male`, `female`, `other`. Hebrew blessings are gendered, so this drives template choice. |
| `importance` | integer | no | `3` | 1–5. |
| `send_time` | string \| null | no | `null` | `HH:MM`, 24-hour. `null` means "use the general send time from settings". |
| `enabled` | boolean | no | `true` | |

`id`, `created_at` and `updated_at` are server-owned and rejected as unknown
fields if sent.

### Example

```console
$ curl -X POST -H 'Content-Type: application/json' \
    -d '{"name":"Dana","phone":"+972 50-123 4567","event_date":"1990-05-17",
         "event_type":"birthday","language":"he","relation":"friend","gender":"female"}' \
    .../api/v1/contacts
{"id":1,"name":"Dana","phone":"972501234567","event_date":"1990-05-17","event_type":"birthday",
 "language":"he","relation":"friend","importance":3,"gender":"female","send_time":null,
 "enabled":true,"created_at":"2026-08-25T17:32:17.779314155Z","updated_at":"2026-08-25T17:32:17.779314155Z"}
```

Note the phone was normalised to `972501234567` and the omitted optionals took
their defaults.

---

## Blessings

A blessing is a message template. `{{name}}` is substituted with the contact's
name at send time.

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/blessings` | List all templates. |
| POST | `/api/v1/blessings` | Create one. `201` on success. |
| GET | `/api/v1/blessings/{id}` | Fetch one. |
| PUT | `/api/v1/blessings/{id}` | Replace one. |
| DELETE | `/api/v1/blessings/{id}` | Delete one. `204` on success. |

### Request body

| Field | Type | Required | Default | Constraint |
|---|---|---|---|---|
| `event_type` | string | yes | — | `birthday`, `wedding`, `anniversary`, `custom`. |
| `language` | string | yes | — | Language tag. |
| `text` | string | yes | — | Non-blank. May contain `{{name}}`. |
| `gender` | string \| null | no | `null` | `male`, `female`, `other`. `null` or `""` means **any gender**. |
| `relation` | string \| null | no | `null` | `friend`, `close_friend`, `family`, `coworker`. `null` or `""` means **any relation**. |
| `enabled` | boolean | no | `true` | |

`gender` and `relation` are targeting filters. A blessing matching a contact's
exact gender or relation is preferred over one where the field is `null`.

Templates **without** `{{name}}` are the only ones eligible for group echo,
because the bot does not know whose birthday a group is celebrating.

### Example

```console
$ curl -X POST -H 'Content-Type: application/json' \
    -d '{"event_type":"birthday","language":"he","gender":"female","text":"יום הולדת שמח {{name}}!"}' \
    .../api/v1/blessings
{"id":1,"event_type":"birthday","language":"he","gender":"female","relation":null,
 "text":"יום הולדת שמח {{name}}!","enabled":true,
 "created_at":"2026-08-25T17:32:46.68660386Z","updated_at":"2026-08-25T17:32:46.68660386Z"}
```

> **Fresh installs ship starter templates.** Migrations `0003` and `0005` seed
> Hebrew and English blessings for every event type, so the bot can send before
> you write anything. They are ordinary rows: edit or delete them freely. A
> language you use that has no template falls back to English and raises a
> notice.

---

## Groups

A group is a watched WhatsApp group the bot echoes into.

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/groups` | List configured groups. |
| POST | `/api/v1/groups` | Add one. `201` on success. |
| GET | `/api/v1/groups/available` | Ask the live provider which groups the account belongs to. |
| GET | `/api/v1/groups/{id}` | Fetch one. |
| PUT | `/api/v1/groups/{id}` | Replace one. |
| DELETE | `/api/v1/groups/{id}` | Delete one. `204`. Cascades to that group's `wish_events`. |

### Request body

| Field | Type | Required | Default | Constraint |
|---|---|---|---|---|
| `name` | string | yes | — | Non-blank. |
| `chat_id` | string | yes | — | Must end in `@g.us`. Unique. A private `@c.us` id is rejected: it would never match. |
| `language` | string | yes | — | Language of the blessing the bot posts here. |
| `threshold` | integer \| null | no | `null` | At least 1. `null` means "use the global group-echo threshold". |
| `enabled` | boolean | no | `true` | |

### Example

```console
$ curl -X POST -H 'Content-Type: application/json' \
    -d '{"name":"Family","chat_id":"120363001234567890@g.us","language":"he","threshold":4}' \
    .../api/v1/groups
{"id":1,"name":"Family","chat_id":"120363001234567890@g.us","language":"he","threshold":4,
 "enabled":true,"created_at":"2026-08-25T17:32:46.70509779Z","updated_at":"2026-08-25T17:32:46.70509779Z"}
```

A duplicate `chat_id` is a `409`, not a server error:

```console
{"error":{"code":"conflict","message":"a group with this chat id already exists","fields":[{"field":"chat_id","message":"must be unique"}]}}
```

### `GET /groups/available`

Returns `[{"chat_id": "...", "name": "..."}]` from the active provider, so the UI
can offer a picker instead of demanding a hand-typed JID.

This endpoint depends on a working provider connection and degrades honestly:

```console
$ curl .../api/v1/groups/available          # no provider configured
{"error":{"code":"provider_unavailable","message":"no whatsapp provider is configured; set one up in Settings"}}
```

On `503` or `502` the UI should fall back to manual chat-id entry.

---

## Wish patterns

Wish patterns are the phrases that make the bot notice a group is congratulating
someone. Migration `0002` seeds Hebrew, English and emoji defaults on first run;
all of them are editable.

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/wish-patterns` | List all patterns. |
| POST | `/api/v1/wish-patterns` | Create one. `201` on success. |
| PUT | `/api/v1/wish-patterns/{id}` | Replace one. |
| DELETE | `/api/v1/wish-patterns/{id}` | Delete one. `204` on success. |

There is deliberately **no** `GET /wish-patterns/{id}`; the UI edits from the
list it already holds. Requesting it returns `405`.

### Request body

| Field | Type | Required | Default | Constraint |
|---|---|---|---|---|
| `language` | string | yes | — | Language tag, or `any` for language-neutral signals such as emoji. |
| `pattern` | string | yes | — | A plain substring, or a regular expression wrapped in slashes. |
| `enabled` | boolean | no | `true` | |

**Pattern syntax.** A value wrapped in `/.../` is compiled as a Go regular
expression and validated at save time, so a broken rule is rejected here rather
than silently never matching. Anything else is a case-insensitive substring. A
value that *starts* with `/` but does not close is rejected as a malformed
regex — an internal slash (`and/or`) is fine.

`(language, pattern)` is unique; re-adding a seeded pattern returns `409`.

### Example

```console
$ curl -X POST -H 'Content-Type: application/json' \
    -d '{"language":"en","pattern":"/happy\\s+b-?day/"}' .../api/v1/wish-patterns
{"id":18,"language":"en","pattern":"/happy\\s+b-?day/","enabled":true}
```

---

## Settings

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/settings` | Current configuration, secrets masked. |
| PUT | `/api/v1/settings` | Update the configuration and re-apply it to the live provider. |

A `PUT` is a **section-wise merge, not a whole-document replace**. Each of
`provider`, `greenapi`, `gowa`, `scheduler` and `group_echo` is optional: a
section you omit (or send as `null`) keeps its stored value. That makes it safe
to change one thing without resending everything —

```json
{"provider": "gowa"}
```

— switches the active provider and leaves both providers' credentials intact.
Within a section you *do* send, every field is replaced, so an omitted field
inside a present section is cleared. The response body from `GET` (including
`provider_error`, when present) is always a valid `PUT` body.

Runtime settings live in the database, not in the YAML config file — the config
file covers infrastructure only (port, data directory, log level).

### Payload

```json
{
  "provider": "greenapi",
  "greenapi": {
    "api_url": "https://api.green-api.com",
    "id_instance": "",
    "api_token": "",
    "mode": "polling",
    "webhook_auth_header": ""
  },
  "gowa": {
    "base_url": "",
    "username": "",
    "password": "",
    "device_id": "",
    "webhook_secret": ""
  },
  "scheduler": {"timezone": "Asia/Jerusalem", "send_time": "09:00"},
  "group_echo": {"threshold": 3, "window_hours": 6, "cooldown_hours": 20}
}
```

That is also the exact response on a fresh install — the defaults.

| Field | Constraint |
|---|---|
| `provider` | `greenapi` or `gowa`. |
| `greenapi.mode` | `polling` (default) or `webhook`. Polling works behind NAT with no public URL. |
| `scheduler.timezone` | An IANA zone name. The binary embeds `time/tzdata`, so named zones resolve even in the `scratch` image. |
| `scheduler.send_time` | `HH:MM`, 24-hour. |
| `group_echo.threshold` | ≥ 1. Distinct senders needed to trigger an echo. |
| `group_echo.window_hours` | ≥ 1. Rolling window the senders must fall inside. |
| `group_echo.cooldown_hours` | ≥ 1. Quiet period after an echo. |

Invalid values return `422 validation_failed` with the offending setting named
in the message.

### How secrets work

Four fields are secrets, encrypted at rest with AES-256-GCM:

- `greenapi.api_token`
- `greenapi.webhook_auth_header`
- `gowa.password`
- `gowa.webhook_secret`

The rules, in both directions:

| Situation | `GET` returns | `PUT` with that value does |
|---|---|---|
| Secret is set | `"••••"` | **leaves it unchanged** |
| Secret is not set | `""` | nothing (stays unset) |
| Sending a new value | — | replaces it |
| Sending `""` for a set secret | — | **clears it** (the whole section must be present) |

Two consequences worth stating plainly:

1. A stored secret is **never** returned in plaintext by any endpoint.
2. The SPA can `GET`, edit one unrelated field, and `PUT` the whole payload back
   verbatim — masks included — without destroying the stored credentials.

```console
$ curl .../api/v1/settings | jq .greenapi.api_token
"••••"
```

### Live reconfiguration

A successful `PUT` re-applies the configuration to the running provider, so
switching between GreenAPI and GOWA, or fixing a typo'd token, needs **no
restart**.

If the settings are valid but the provider will not build with them, the write
still succeeds and the failure is reported alongside:

```json
{
  "provider": "greenapi",
  "greenapi": {"...": "..."},
  "provider_error": "greenapi: instance id is required"
}
```

The status stays `200` — refusing the write would strand the user with settings
they cannot save. `provider_error` is absent when the rebuild succeeded.

---

## Provider

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/provider/status` | Connection state of the active provider. |
| POST | `/api/v1/provider/test` | Send a real test message. |

### `GET /provider/status`

```json
{"provider": "gowa", "connected": true, "state": "authorized", "needs_qr": false}
```

| Field | Meaning |
|---|---|
| `provider` | `greenapi` or `gowa`. |
| `connected` | Whether the backend is usable right now. |
| `state` | Backend-specific state string. |
| `needs_qr` | The backend needs a QR scan. Render as a call to action linking to the GOWA UI — v1 does not proxy the QR flow. |
| `detail` | Optional extra context. |

`needs_qr` is a normal reportable state and arrives as `200`, not an error.
`503` means nothing is configured; `502` means the backend was unreachable.

### `POST /provider/test`

```json
{"phone": "972501234567", "message": "optional custom text"}
```

`phone` accepts a bare international number (normalised to `<digits>@c.us`) or a
full chat id, so the same button can verify a group. Omitting `message` sends a
default. On success:

```json
{"provider": "greenapi", "chat_id": "972501234567@c.us", "message_id": "BAE5F4"}
```

> This sends a **real WhatsApp message**. It goes through the same rate limiter
> as every other send (default 1 message / 3 s), so it cannot be used to bypass
> the pacing that exists to avoid a WhatsApp ban.

```console
$ curl -X POST -d '{"phone":"972501234567"}' .../api/v1/provider/test
{"error":{"code":"provider_unavailable","message":"no whatsapp provider is configured; set one up in Settings"}}
```

---

## History

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/history` | Send log, newest first. |

| Parameter | Type | Default | Notes |
|---|---|---|---|
| `kind` | string | all kinds | `scheduled` or `group_echo`. |
| `limit` | integer | 100 | Positive. Clamped to a maximum of 500. |

Response entries:

| Field | Type | Notes |
|---|---|---|
| `id` | integer | |
| `kind` | string | `scheduled` or `group_echo`. |
| `contact_id` | integer \| null | Set for scheduled sends. |
| `group_id` | integer \| null | Set for group echoes. |
| `blessing_id` | integer \| null | Template used. |
| `provider` | string | Provider that sent it. |
| `chat_id` | string | Destination. |
| `status` | string | `sent` or `failed`. |
| `error` | string \| null | Failure reason. |
| `event_year` | integer \| null | Dedupe key for scheduled sends: one successful send per contact per year. |
| `sent_at` | string | RFC 3339, UTC. |

An empty log returns `[]`, never `null`. Rows are written by the scheduler on
every send attempt — **successes and failures both**. Only a successful row with
a non-null `event_year` counts toward the yearly dedupe, which is why a failed
send is retried on the next tick for the rest of that day.

```console
$ curl '.../api/v1/history?kind=telepathy'
{"error":{"code":"invalid_query","message":"kind must be scheduled or group_echo"}}
```

---

## Notices

Conditions the operator should act on, but which are not failures worth
refusing work over. The dashboard renders active ones as a banner.

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/notices` | Active notices, newest first. `?all=true` includes dismissed ones. |
| DELETE | `/api/v1/notices/{id}` | Dismiss one. `204` on success. |

| Field | Type | Notes |
|---|---|---|
| `id` | integer | |
| `key` | string | Stable identity. Raising the same key again increments `occurrences` rather than adding a row. |
| `level` | string | `info`, `warning` or `error`. |
| `code` | string | Machine-readable kind; the SPA switches on it for wording. |
| `message` | string | What happened. |
| `detail` | string \| null | What to do about it. |
| `occurrences` | integer | How many times the condition has been seen. |
| `first_seen_at`, `last_seen_at` | string | RFC 3339, UTC. |
| `dismissed_at` | string \| null | Set once dismissed. |

Dismissing **hides, it does not delete**. The row survives so a recurrence can
reopen it: the operator acknowledged the last occurrence, not the underlying
problem.

### Codes

| Code | Raised when |
|---|---|
| `language_fallback` | A contact's (or group's) language had no template, so the English one was used. Keyed by event type and language, so a whole address book missing one language produces a single actionable notice. |

```console
$ curl .../api/v1/notices
[{"id":1,"key":"language_fallback:birthday:ru","level":"warning","code":"language_fallback",
  "message":"No birthday blessing in \"ru\"; using the en template instead.",
  "detail":"Add a birthday template in \"ru\" under Blessings, or the English one keeps being used.",
  "occurrences":3,"first_seen_at":"2026-08-26T08:06:21.399Z",
  "last_seen_at":"2026-08-26T08:06:22.206Z","dismissed_at":null}]
```

---

## Send now

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/contacts/{id}/send-now` | Send this contact's blessing immediately. |

| Parameter | Type | Default | Notes |
|---|---|---|---|
| `force` | boolean | `false` | `true` also bypasses the once-per-year dedupe. |

Sends straight away, ignoring **both** the event date and the send time — you
asked for it, so neither is relevant. Blessing selection is identical to a
scheduled send: same targeting tiers, same language fallback, same `{{name}}`
substitution, and it goes through the same rate limiter.

The once-per-year dedupe still applies unless `force=true`, so the button cannot
spam someone:

```console
$ curl -X POST .../api/v1/contacts/1/send-now
{"error":{"code":"already_sent","message":"this contact already received a blessing this year; use force=true to send anyway"}}
```

A forced resend is recorded with a **null `event_year`**. It is history, not a
dedupe key: the original scheduled send still counts for the year, so the tick
loop stays quiet afterwards.

```console
$ curl -X POST '.../api/v1/contacts/1/send-now?force=true'
{"id":2,"kind":"scheduled","contact_id":1,"blessing_id":1,"provider":"gowa",
 "chat_id":"972501234567@c.us","status":"sent","error":null,"event_year":null,
 "sent_at":"2026-08-25T18:51:02.117Z"}
```

Other outcomes: `404` if the contact does not exist, `409 contact_disabled` if
it is muted, `422 no_blessing` if no template matches, and `503` if no provider
is configured.

---

## Webhooks

| Method | Path | Provider |
|---|---|---|
| POST | `/webhooks/greenapi` | GreenAPI |
| POST | `/webhooks/gowa` | go-whatsapp-web-multidevice |

These are **not** under `/api/v1`: they are provider callbacks, not part of the
versioned API, and they deliberately sit outside any future API authentication.

See [`providers.md`](providers.md) for the full setup walkthrough of both
backends, including the GOWA flags that enable webhook delivery.

### Authentication

**GOWA** requires a valid `X-Hub-Signature-256` HMAC-SHA256 of the raw request
body, keyed with `gowa.webhook_secret`. Verification **fails closed**: if no
secret is configured, every webhook is rejected, because accepting unverified
callbacks would let anything that can reach the port inject fake wishes.

```console
$ curl -X POST -d '{"event":"message"}' .../webhooks/gowa   # no secret configured
{"error":{"code":"unauthorized","message":"webhook signature verification failed"}}
```

**GreenAPI** checks the `Authorization` header against
`greenapi.webhook_auth_header`. An unset value accepts anything, matching
GreenAPI's own default of an unauthenticated webhook — which is why **polling
mode is recommended** for a deployment that is not internet-exposed.

### Response semantics

| Situation | Status |
|---|---|
| Message accepted and dispatched | `200` |
| Payload is authentic but of a type we ignore (status update, reaction) | `200` |
| Downstream handling failed | `200` |
| Body is not parsable | `400` |
| Authentication failed | `401` |

Unhandled payloads and downstream failures return `200` on purpose: providers
retry on non-2xx, and neither case is something the provider can fix by sending
the message again.

### What happens to an accepted message

An authentic group message is handed to the **group echo engine**:

1. It is ignored unless it arrives in a configured, enabled group.
2. Messages the bot itself sent are skipped — its own blessing matches the wish
   patterns, so counting it would let the bot trigger itself.
3. The text is normalized (decomposed, combining marks stripped, casefolded) and
   tested against every enabled wish pattern in **all** languages, since groups
   are multilingual. Stripping combining marks is what makes vocalised Hebrew
   (`מַזָּל טוֹב`) match the plain pattern `מזל טוב`.
4. A match is recorded in `wish_events`, keyed `UNIQUE(group_id, message_id)` so
   a provider redelivery cannot inflate the count.
5. If the number of **distinct senders** inside the rolling window reaches the
   group's threshold — one excited friend sending five messages is one vote —
   and the group is out of cooldown, the bot posts one blessing and logs it as
   `group_echo`.

The blessing chosen for a group is always **name-free**: the bot sees the burst
but does not know whose birthday it is, so templates containing `{{name}}` (and
templates targeted at a gender or relation) are excluded. The starter seed
(migrations 0003 and 0005) carries a name-free template for every event type
and language, so this works on a fresh install.

Window, threshold and cooldown come from `group_echo` in Settings; a group may
override the threshold. `wish_events` older than 7 days are swept nightly.
