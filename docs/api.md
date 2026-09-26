# The SVPC AI HTTP API

SVPC AI serves a small HTTP API on a loopback port. It is what the desktop
window talks to, what the Android app talks to, and what every SDK in `sdk/`
talks to. This document is the contract; the code in `internal/gui` is the
implementation, and the tests in `internal/gui` are the check that the two agree.

Anything not written down here is not part of the contract and may change.

## Talking to it

Start the agent with the bridge exposed:

    svpc --serve 0.0.0.0

The command prints the port and a token. The token is generated on first use,
192 bits from the system random source, stored in `serve.toml` in the
configuration directory, and shown once. It is the only credential.

    Authorization: Bearer <token>

A token in the query string is accepted once and exchanged for a cookie, which is
what a browser or a WebView needs:

    GET /?token=<token>

The cookie is `HttpOnly` and `SameSite=Strict`. A request without a valid
credential gets `401`, except on loopback, where the only possible client is the
window on the same machine and a credential would be a token in a URL for no gain.

Closing the connection cancels the turn in progress: the agent's context is the
request's context.

## Endpoints

Four, and a fifth for compatibility.

| | |
|---|---|
| `POST /api/v1/chat` | Run one turn. Streams. |
| `GET  /api/v1/models` | The model in use, and whether the agent is configured. |
| `GET  /api/v1/sessions` | Stored conversations. |
| `POST /api/v1/permission` | Answer an approval request. |
| `GET  /api/v1/version` | The agent's version, for a compatibility check. |

`/api/chat`, `/api/models`, `/api/sessions` and `/api/permission` are the same
handlers without the version, kept because the shipped window and the shipped
Android app call them. New clients should use the versioned paths: they are what
stays fixed, so a future `/api/v2/chat` can be added beside this one rather than
through it.

## `POST /api/v1/chat`

One turn. The body:

```json
{
  "session_id": "optional; omit to start or continue the newest",
  "content":    "what to say to the agent",
  "config": {
    "provider": "optional",
    "api_key":  "optional",
    "model":    "optional",
    "base_url": "optional; a gateway that speaks the provider's API"
  }
}
```

Anything left empty in `config` keeps whatever the agent is already using, which
is what lets a window or a phone reuse the terminal's configuration instead of
demanding its own.

The reply is a stream of server-sent events. Each event is one JSON object on one
`data:` line, and there is no event type in the framing, so a client dispatches on
the keys that are present:

```json
data: {"session_id":"ses_..."}

data: {"tool":{"id":"call_1","name":"Bash","input":"ls -la","state":"running"}}

data: {"text":"Looking at the directory."}

data: {"tool":{"id":"call_1","name":"Bash","input":"ls -la","state":"done"}}

data: {"done":true}
```

| Key | Meaning |
|---|---|
| `session_id` | The session the turn is in. Sent once, first. |
| `text` | Part of the answer. Several may arrive. |
| `tool` | A tool started (`state` is `running`) or finished (`state` is `done`). |
| `tool_error` | A tool failed. `error` says why; the turn carries on. |
| `permission` | The agent needs an answer. See below. |
| `error` | The turn failed. Nothing else will arrive. |
| `done` | The turn is over. Always last on a successful turn. |

A tool that is reported `running` and never `done` is one the agent is still
waiting on. A `permission` event suspends the turn until `POST /api/v1/permission`
answers it; the turn does not time out on its own.

Only one turn runs at a time. A second concurrent turn gets
`{"error":"a turn is already running"}` rather than interleaving, because the
approval requests of two turns would have nowhere to go.

## `POST /api/v1/permission`

```json
{ "id": "the id from the permission event", "action": "allow" }
```

`action` is `allow`, `allow_session` — do not ask again for this session — or
`deny`. Answering an id that is not outstanding is ignored, and so is a second
answer to the same one, because a client that retries after a timeout must not be
able to reverse a decision the user already saw.

## `GET /api/v1/models`

```json
{ "ready": true, "current": { "id": "gpt-4.1", "name": "GPT-4.1",
                             "provider": "openai", "context_window": 1000000 } }
```

`ready` is false before the agent has been started, in which case `current` is
absent and `setup_error` may explain why — a client should show a setup screen
rather than an error, because this is the normal state of a fresh install.

## `GET /api/v1/sessions`

```json
{ "sessions": [ { "id": "ses_...", "title": "...", "created_at": 1758000000 } ] }
```

Newest first. An unconfigured agent returns an empty list rather than an error,
for the same reason `models` does.

## `GET /api/v1/version`

```json
{ "version": "1.0.0", "api": "v1" }
```

An SDK should call this once on connect and refuse to proceed if `api` is not one
it speaks, rather than failing later on a shape it does not recognise.

## What the API deliberately does not have

- **Fetching a session's messages.** The window keeps its own transcript and the
  agent keeps its own; neither is served. An SDK that needs history has to keep
  what it streamed.
- **Listing or cancelling turns by id.** One turn at a time, cancelled by
  disconnecting. A queue would be a feature, and a feature that can be wrong about
  whose turn is running.
- **Creating a session explicitly.** A session is a consequence of sending a
  message, and an id the client did not choose is one less thing to get wrong.
