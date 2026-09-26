# svpc-ai

A Python client for the [SVPC AI](https://github.com/sovereignempirex-ux/svcp-ai) HTTP API.

SVPC AI is an agentic coding assistant. It runs on your machine; this talks to it.
Nothing is sent to a third party, and no model runs in your process.

The standard library only. A client for a tool that runs commands ought not to drag
a dependency tree into the program that uses it, and everything here — `urllib`,
`json`, generators — is in every Python that has ever run.

## Install

```bash
pip install svpc-ai
```

## Use

Start the agent on the machine you want to drive:

```bash
svpc --serve 0.0.0.0
```

It prints a port and a token. The token is generated on first use and shown once.

```python
from svpc import Client, ALLOW

with Client("192.168.1.20", 8080, token) as agent:
    agent.connect()          # check the bridge is there and speaks this API version
    print(agent.ask_once("what changed in this repository?")["text"])
```

### Watching a turn happen

`ask` is a generator, so the answer prints as it is written:

```python
with Client(host, port, token) as agent:
    for event in agent.ask("fix the failing test", on_permission=decide):
        if event.text:
            print(event.text, end="", flush=True)
        elif event.tool and event.tool.running:
            print(f"\n[running {event.tool.name}]", file=sys.stderr)
        elif event.tool_error:
            print(f"\n[{event.tool_error.id} failed: {event.tool_error.error}]", file=sys.stderr)
```

Breaking out of the loop closes the connection, which is how the bridge learns the
turn is over.

### Continuing a conversation

The session id arrives on the first event, and it is the one value the turn
discovers rather than the one you gave it:

```python
session = ""
with Client(host, port, token) as agent:
    for event in agent.ask("what does this do?"):
        if event.session_id:
            session = event.session_id
        if event.text:
            print(event.text, end="", flush=True)

    # Ask again in the same conversation.
    print(agent.ask_once("and how is it built?", session_id=session)["text"])
```

## Approval

The agent asks before it runs a command or writes outside the project, and the turn
is suspended until it is answered. `on_permission` is where that answer comes from,
and a request with no handler is **denied** — the safe outcome for a program nobody
is watching.

```python
from svpc import ALLOW, ALLOW_SESSION, DENY

def decide(request):
    # request.description is the model's own sentence about what it is about to
    # do, and it is the text to show a person: the tool and action alone do not
    # say what happens to their files.
    if request.tool in ("read", "grep", "glob"):
        return ALLOW
    if request.path.startswith("/home/me/project"):
        return ALLOW_SESSION
    return DENY
```

## Not configured is not broken

A fresh install has no provider, and that is a state the client reports rather than
raises — a program that raised here would make the agent look broken when it is not.

```python
status = agent.models()
if not status["ready"]:
    print(status["setup_error"])   # "no provider is configured"
```

## Errors

Every failure is an `ApiError` carrying the status, because "it failed" is not
actionable and the one thing a person can do about a 401 is fix the token.

```python
from svpc import ApiError

try:
    agent.ask_once("hello")
except ApiError as err:
    if err.unauthorized:
        print("the token is wrong; read serve.toml in your config directory")
    else:
        raise
```

## Also

- [`pkg/svpc`](https://github.com/sovereignempirex-ux/svcp-ai/tree/main/pkg/svpc) —
  embed the agent in a Go program, with no server in between.
- [`sdk/js`](https://github.com/sovereignempirex-ux/svcp-ai/tree/main/sdk/js) — the
  same client for JavaScript, with no dependencies, in the browser and on Node.
- [`docs/api.md`](https://github.com/sovereignempirex-ux/svcp-ai/blob/main/docs/api.md) —
  the contract itself, for a language nobody has written a client for yet.

## Licence

MIT
