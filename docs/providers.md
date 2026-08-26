# WhatsApp provider setup

blessed-by-the-bot talks to WhatsApp through one of two providers. You pick and
configure the active one in **Settings** in the web UI — switching providers does
not require restarting the app.

Credentials you enter are encrypted at rest with AES-256-GCM under your
`BBTB_ENCRYPTION_KEY` and are never returned in plaintext by the API.

| | GreenAPI | GOWA |
|---|---|---|
| Hosting | Cloud service (paid tiers, free tier available) | Self-hosted container you run |
| Needs a public URL | No (polling mode) | Yes, for webhooks |
| Incoming messages | Polling **or** webhook | Webhook only |
| Best for | Home-lab behind NAT | You already run your own stack |

---

## GreenAPI

### 1. Create an instance

1. Sign up at <https://green-api.com> and create an instance.
2. Scan the QR code in the GreenAPI console with the WhatsApp account the bot
   should send from. The instance state must reach **`authorized`**.
3. Note the **Instance ID** (`idInstance`) and **API Token** (`apiToken`).

### 2. Fill in Settings → GreenAPI

| Field | Notes |
|---|---|
| **API URL** | Leave blank for `https://api.green-api.com`. GreenAPI assigns instances to clusters — if your console shows a cluster host such as `https://7103.api.greenapi.com`, put that here. |
| **Instance ID** | Digits only, copied exactly. |
| **API token** | Copied exactly. |
| **Mode** | `polling` (default) or `webhook` — see below. |
| **Webhook auth header** | Optional. Only used in webhook mode. |

**Copy these fields with no leading or trailing whitespace.** Both values become
path segments in every request URL, so a stray space produces an opaque `400`
rather than a useful error. The app trims them defensively, but the console's
copy button is safer than selecting by hand.

### 3. Choose an incoming mode

**Polling (default, recommended).** The app long-polls GreenAPI's
`receiveNotification` endpoint and deletes each receipt after handling it. This
needs **no inbound connectivity**, so it works behind NAT with no port forwarding
and no public hostname — the right choice for a home-lab deployment.

**Webhook.** Point GreenAPI at `https://<your-public-host>/webhooks/greenapi`.
Set an authorization token in the GreenAPI console and put the same value in the
**Webhook auth header** field; the app compares it in constant time and rejects
mismatches. Leaving it blank accepts any caller, which matches GreenAPI's own
default but is only safe on a trusted network.

### Chat ID format

Phone numbers are stored as E.164 digits with no `+` and no spaces — for example
`972501234567`, not `+972 50 1234567`. The app appends `@c.us` for private chats
itself; group chat IDs (`…@g.us`) are used as-is.

---

## GOWA (go-whatsapp-web-multidevice)

The app does **not** run GOWA for you. You run it yourself and point this app at
it. Target **v8.x**.

### 1. Run GOWA

Add it to your own compose stack, for example:

```yaml
services:
  gowa:
    image: aldinokemal2104/go-whatsapp-web-multidevice:latest
    ports:
      - "3000:3000"
    volumes:
      - gowa-data:/app/storages
    command:
      - rest
      - --basic-auth=admin:choose-a-strong-password
      - --webhook=http://blessedbot:8080/webhooks/gowa
      - --webhook-secret=choose-a-strong-webhook-secret
      - --webhook-events=message
    restart: unless-stopped

volumes:
  gowa-data:
```

`--webhook-events=message` matters: the app only consumes message events, and
subscribing to everything else just adds noise it discards.

### 2. Log the device in

Open GOWA's web UI and scan the QR code. **This app deliberately does not proxy
the QR flow in v1** — when the device is logged out, the Settings page reports
"QR needed" and links you to GOWA's own UI.

### 3. Fill in Settings → GOWA

| Field | Notes |
|---|---|
| **Base URL** | Where the app reaches GOWA, e.g. `http://gowa:3000`. A trailing slash is trimmed. |
| **Username** / **Password** | Must match `--basic-auth`. Leave both blank if you run GOWA without auth. |
| **Device ID** | See below. |
| **Webhook secret** | Must match `--webhook-secret`. **Required** — see below. |

**Device ID.** GOWA v8 scopes device calls by an `X-Device-Id` header. Set this
to your registered device's id. If you leave it blank the app omits the header
entirely, and GOWA falls back to the single registered device — which only works
if you have exactly one.

**Webhook secret is mandatory here.** The app refuses to accept a GOWA webhook
unless a secret is configured and the `X-Hub-Signature-256` HMAC-SHA256 digest
over the raw body verifies. Without that, anyone who can reach the endpoint could
forge group traffic and make the bot post on demand. There is no "accept
unsigned" option.

---

## Verifying setup

Settings has **Test connection** and **Send test message** buttons. Behind them:

- Connection test → GreenAPI `getStateInstance` / GOWA `/app/status`
- Group picker → GreenAPI `getContacts` (filtered to `type: "group"`) / GOWA `/user/my/groups`
- Sending → GreenAPI `sendMessage` / GOWA `/send/message`

Outbound messages are paced at **one message every three seconds** regardless of
provider. WhatsApp bans accounts that send in bursts, and this limit applies to
scheduled blessings and group replies alike.

Failed requests retry three times with jittered exponential backoff, but only for
network errors, `5xx` and `429`. A `4xx` — a rejected token, a malformed chat id
— fails immediately, because retrying a rejected credential only burns your rate
limit.

---

## Implementation assumptions worth verifying

The GreenAPI integration follows that service's published request and response
shapes. The **GOWA** integration was written against the endpoints named in the
build spec, but the exact JSON field names could not be checked against a live
v8 instance during development. Response parsing is therefore deliberately
tolerant:

| Call | Endpoint | Fields read |
|---|---|---|
| Send | `POST /send/message` | `results.message_id`, falling back to `results.messageId`, then `results.id` |
| Groups | `GET /user/my/groups` | `results.data[].JID`/`jid` and `.Name`/`name` |
| Status | `GET /app/status` | `results.is_connected`, `results.is_logged_in` |
| Webhook | — | `event`, `payload.id`, `payload.chat_id`, `payload.from`, `payload.pushname`, `payload.message.text` (or `.conversation`), `payload.timestamp` |

A send whose response omits every known message-id field is still treated as
**successful** with an empty id — a message that actually went out must not be
recorded as failed just because a field was renamed. If your GOWA instance
returns different shapes, these are the places to adjust.


## Self-sent messages

The echo engine must not count the bot's own blessing as a wish — the text it
posts matches the wish patterns, so it would otherwise help trigger itself.

- **GreenAPI**: nothing to do. Its `incomingMessageReceived` webhook fires only
  for messages from other people; the bot's own sends arrive as
  `outgoingMessageReceived`/`outgoingAPIMessageReceived`, which the parser
  ignores outright.
- **GOWA**: the parser reads a `from_me` flag from the payload, also accepting
  the `fromMe` and `is_from_me` spellings this field has carried across
  releases. If your GOWA build emits none of them, self-sent messages will be
  counted as wishes — worth verifying against your version, since a bot echo
  would then contribute one sender toward the next trigger.


## Polling vs webhooks: which loop runs

GreenAPI's **polling** mode is the default, and it is what a deployment behind
NAT should use — it needs no public URL. The application runs the polling loop
itself, supervised so that it starts, stops and restarts to match your settings:

| Setting | What runs |
|---|---|
| GreenAPI + `polling` | The app polls `receiveNotification` continuously |
| GreenAPI + `webhook` | Nothing polls; GreenAPI must reach `/webhooks/greenapi` |
| GOWA | Nothing polls; GOWA pushes to `/webhooks/gowa` |

Switching provider or mode in Settings takes effect immediately — no restart.
Changing the instance ID, token or API URL restarts the loop with the new
credentials, because the running one holds the old ones.

Polling does **not** start when the instance ID or token is blank. A loop that
can only ever return 401 would bury a genuine outage in authentication errors,
so an unconfigured provider stays quiet until you fill the fields in.
