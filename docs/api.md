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
| `invalid_json` | 400 / 413 | Malformed body, unknown field, wrong field type, or oversized body (limit 1 MiB). |
| `invalid_id` | 400 | The `{id}` path segment is not a positive integer. |
| `invalid_query` | 400 | A query parameter is malformed. |
| `validation_failed` | 422 | One or more fields were rejected; see `fields`. |
| `conflict` | 409 | A uniqueness constraint was violated. |
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
| `event_date` | string | yes | — | `YYYY-MM-DD`, zero-padded, a real calendar date. The year is kept for age arithmetic. |
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

> **Fresh installs ship no blessings.** Migration `0002` seeds wish *patterns*
> only. Add at least one blessing per event type and language you use, or
> scheduled sends will find no template.

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
| PUT | `/api/v1/settings` | Replace the configuration and re-apply it to the live provider. |

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
| Sending `""` for a set secret | — | **clears it** |

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

An empty log returns `[]`, never `null`.

```console
$ curl '.../api/v1/history?kind=telepathy'
{"error":{"code":"invalid_query","message":"kind must be scheduled or group_echo"}}
```

---

## Send now

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/contacts/{id}/send-now` | Send this contact's blessing immediately. |

| Parameter | Type | Default | Notes |
|---|---|---|---|
| `force` | boolean | `false` | `true` also bypasses the once-per-year dedupe. |

Bypasses the scheduled send time. With `force=false` the once-per-year dedupe
still applies, so a contact who already received this year's blessing gets
nothing. On success the created send-log entry is returned.

**Not yet implemented.** The blessing engine arrives in Phase 5. Until then the
route exists and answers honestly:

```console
$ curl -X POST .../api/v1/contacts/1/send-now
{"error":{"code":"not_implemented","message":"the blessing engine is not running yet"}}
```

The UI should render the button as disabled when this returns `501`.

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

Consumption of these messages by the group-echo engine arrives in Phase 6. Until
then the endpoints verify, parse and log, which is enough to confirm a
provider's webhook configuration end to end.
