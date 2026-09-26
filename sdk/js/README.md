# svpc-ai-sdk

A client for the [SVPC AI](https://github.com/sovereignempirex-ux/svcp-ai) HTTP API,
for the browser, Deno, Bun and Node.

SVPC AI is an agentic coding assistant. It runs on your machine; this talks to it.
Nothing is sent to a third party, and no model runs in the page.

No dependencies, and nothing Node-specific: it uses `fetch` and `ReadableStream`,
which a browser, a WebView, Deno, Bun and Node 18 all have. That is deliberate —
the Android client is a WebView, so the same file runs there.

## Install

```bash
npm install svpc-ai-sdk
```

## Use

Start the agent on the machine you want to drive:

```bash
svpc --serve 0.0.0.0
```

It prints a port and a token. The token is generated on first use and shown once.

```js
import { Client, ALLOW, DENY } from 'svpc-ai-sdk';

const agent = new Client({
  host: '192.168.1.20',
  port: 8080,
  token: process.env.SVPC_TOKEN,
});

// Check the bridge is there and speaks this version of the API. Worth doing once
// on connect: a bridge that does not answer this is one this client cannot use.
await agent.version();

const { ready, setupError } = await agent.models();
if (!ready) {
  // Not an error. A fresh install has no provider, and the agent is running.
  throw new Error(setupError);
}

const { text } = await agent.askOnce('what changed in this repository?');
console.log(text);
```

### Watching a turn happen

`ask` is an async generator, so the answer reads as it is written:

```js
for await (const event of agent.ask('fix the failing test', {
  onPermission: async (request) => {
    // request.description is the model's own sentence about what it is about to
    // do, and it is the text to show a person.
    const ok = await askTheUser(`${request.tool} wants to ${request.action}:\n${request.description}`);
    return ok ? ALLOW : DENY;
  },
})) {
  if (event.text) process.stdout.write(event.text);
  if (event.tool?.running) console.error(`running ${event.tool.name}`);
  if (event.toolError) console.error(`${event.toolError.id} failed: ${event.toolError.error}`);
}
```

`break` out of the loop to stop watching. It does not cancel the turn; pass an
`AbortSignal` for that.

### Continuing a conversation

The session id arrives on the first event, and it is the one value the turn
discovers rather than the one you gave it:

```js
let session = '';
for await (const event of agent.ask('what does this do?')) {
  if (event.sessionId) session = event.sessionId;
  if (event.text) process.stdout.write(event.text);
}
// Ask again in the same conversation:
await agent.askOnce('and how is it built?', { sessionId: session });
```

## Approval

The agent asks before it runs a command or writes outside the project, and the turn
is suspended until it is answered. `onPermission` is where that answer comes from,
and a request with no handler is **denied** — the safe outcome for a program nobody
is watching.

```js
const agent = new Client({
  /* ... */
  onPermission: (r) =>
    r.tool === 'read' || r.tool === 'grep' ? ALLOW : DENY,
});
```

## Errors

Every failure is an `ApiError` carrying the status, because "it failed" is not
actionable and the one thing a person can do about a 401 is fix the token.

```js
import { ApiError } from 'svpc-ai-sdk';

try {
  await agent.askOnce('hello');
} catch (err) {
  if (err instanceof ApiError && err.unauthorized) {
    console.error('the token is wrong; read serve.toml in your config directory');
  } else {
    throw err;
  }
}
```

## The contract

[`docs/api.md`](https://github.com/sovereignempirex-ux/svcp-ai/blob/main/docs/api.md)
in the SVPC AI repository. It is versioned; this client speaks `v1` and refuses a
bridge that speaks something else, rather than failing later on a reply shape it
misreads.

## Also

- [`pkg/svpc`](https://github.com/sovereignempirex-ux/svcp-ai/tree/main/pkg/svpc) —
  embed the agent in a Go program, with no server in between.
- [`sdk/python`](https://github.com/sovereignempirex-ux/svcp-ai/tree/main/sdk/python) —
  the same client for Python, standard library only.

## Licence

MIT
