# SVPC AI — Go library

Embed the agent in a Go program. The command line tool, the desktop window and
the HTTP bridge are three faces of one agent; this is the agent on its own.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/svpc-ai/svpc/pkg/svpc"
)

func main() {
	ctx := context.Background()

	agent, err := svpc.New(ctx, svpc.Options{
		WorkingDir: ".",
		OnPermission: func(r svpc.Request) svpc.Decision {
			// r.Description is the model's own sentence about what it is about to
			// do, and it is the thing to show a person: the tool and action alone
			// do not say what happens to their files.
			fmt.Printf("%s wants to %s: %s\n", r.Tool, r.Action, r.Description)
			return svpc.Allow
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer agent.Close()

	// Check before asking. A fresh install has no provider, which is not an error.
	if status, _ := agent.Status(ctx); !status.Ready {
		log.Fatal(status.Error)
	}

	answer, session, err := agent.Answer(ctx, "", "what does this repository do?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(answer)

	// Keep the session id to continue the same conversation.
	_, _, err = agent.Answer(ctx, session, "and how is it built?")
}
```

## Install

```
go get github.com/svpc-ai/svpc/pkg/svpc
```

## What it does

`New` reads the same configuration file the command line tool reads, so an
embedded agent and a terminal share credentials, model and history rather than
each keeping their own.

| | |
|---|---|
| `New` | Starts an agent. A configuration that cannot be read is not fatal. |
| `Status` | Whether a provider is configured, and which. Call this first. |
| `Ask` | One turn, with a callback per event. |
| `Answer` | One turn, returning the whole reply and the session id. |
| `Close` | Releases everything, and answers anything waiting on approval. |

## Approval

The agent asks before it runs a command or writes outside the project, and the
turn is suspended until it is answered. `Options.OnPermission` is where that
answer comes from.

```go
agent, _ := svpc.New(ctx, svpc.Options{
	OnPermission: func(r svpc.Request) svpc.Decision {
		switch {
		case r.Tool == "read" || r.Tool == "grep" || r.Tool == "glob":
			return svpc.Allow // reading is not dangerous
		case strings.HasPrefix(r.Path, "/home/me/project"):
			return svpc.AllowSession
		default:
			return svpc.Deny
		}
	},
})
```

`Deny` is the zero value, so a `Request` nobody answered is refused rather than
allowed. A `nil` function denies everything, which is the right default for a
program nobody is watching.

The wait is bounded: after five minutes the action is refused and the model is
told so, which it can react to. A callback that hangs therefore fails safe rather
than forever.

## A turn in detail

`Ask` is a callback, not a promise, because a turn produces many things in order
and a caller usually wants the answer as it is written:

```go
err := agent.Ask(ctx, session, "fix the failing test", func(ev svpc.Event) bool {
	switch {
	case ev.Text != "":
		fmt.Print(ev.Text)
	case ev.Tool.Running:
		log.Printf("running %s", ev.Tool.Name)
	case ev.ToolError.Error != "":
		log.Printf("%s failed: %s", ev.ToolError.ID, ev.ToolError.Error)
	case ev.SessionID != "":
		session = ev.SessionID
	case ev.Error != nil:
		log.Printf("turn failed: %v", ev.Error)
	}
	return true // false stops watching, without cancelling the turn
})
```

`Tool.Input` is shortened to 200 characters, because a tool's real input is a JSON
document that can run to pages and a caller displaying it wants its shape.

## What it will not do

- **Run two turns at once.** The agent refuses a second turn in a conversation
  while the first is running, because the tools of the first are still going. A
  turn is cancelled by cancelling its context.
- **Invent a session id.** One is created when you send a message, and the id
  arrives on the first event.
- **Reconfigure anything on its own.** The configuration is the user's. `Provider`
  changes a key, because a program that could rewrite a credential file should say
  so out loud first.

## Also in this repository

| | |
|---|---|
| `docs/api.md` | The HTTP contract, for clients in other languages. |
| `sdk/js` | A JavaScript client, for the browser, Deno, Bun and Node. |
| `sdk/python` | A Python client, standard library only. |

## Licence

MIT.
