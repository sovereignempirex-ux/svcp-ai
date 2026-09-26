/**
 * The client is tested against a real HTTP server that speaks the protocol the way
 * the bridge does, rather than against a mock of the client. A mock would agree
 * with the client by construction, which is the one thing a test of a parser must
 * not do.
 *
 * Run with:  node --test sdk/js
 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';

import { Client, ApiError, ALLOW, DENY, readEvents, decode } from './client.js';

/**
 * A bridge that answers with whatever the test tells it to.
 *
 * `frames` is written as raw text so a test can send two events in one write,
 * split one event across two writes, and put a stray blank line in the stream,
 * which is what a network does and what a parser has to survive.
 */
async function serve(handler) {
  const server = http.createServer(handler);
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const { port } = server.address();
  return {
    port,
    close: () => new Promise((resolve) => server.close(resolve)),
  };
}

/** The exact byte sequences the bridge writes, per docs/api.md. */
const SSE = (obj) => `data: ${JSON.stringify(obj)}\n\n`;

test('readEvents reassembles frames split across writes', async () => {
  // One frame delivered in two pieces, which is what a slow network does to a
  // parser that reads "lines" instead of frames.
  const parts = ['data: {"text":"hel', 'lo"}\n\ndata: {"done":true}\n\n'];
  const stream = new ReadableStream({
    start(controller) {
      for (const p of parts) controller.enqueue(new TextEncoder().encode(p));
      controller.close();
    },
  });

  const seen = [];
  for await (const frame of readEvents(stream)) seen.push(frame);
  assert.equal(seen.length, 2);
  assert.equal(decode(seen[0]).text, 'hello');
  assert.equal(decode(seen[1]).done, true);
});

test('readEvents keeps several frames that arrived in one write', async () => {
  // The opposite failure: a parser that assumed one frame per read would see one
  // event and silently drop the rest of the turn.
  const all = SSE({ text: 'one' }) + SSE({ text: 'two' }) + SSE({ done: true });
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(all));
      controller.close();
    },
  });

  const seen = [];
  for await (const frame of readEvents(stream)) seen.push(decode(frame));
  assert.equal(seen.length, 3);
  assert.deepEqual(seen.map((e) => e.text || e.done), ['one', 'two', true]);
});

test('readEvents emits a trailing frame that never got its blank line', async () => {
  // A bridge that closes straight after the last event is behaving correctly, and
  // a parser that waits for a terminator would lose the answer.
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode('data: {"done":true}'));
      controller.close();
    },
  });
  const seen = [];
  for await (const frame of readEvents(stream)) seen.push(decode(frame));
  assert.equal(seen.length, 1);
  assert.equal(seen[0].done, true);
});

test('decode skips a frame that is not JSON and keeps the rest', async () => {
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(
        new TextEncoder().encode('data: not json at all\n\n' + SSE({ text: 'after' }))
      );
      controller.close();
    },
  });
  const seen = [];
  for await (const frame of readEvents(stream)) {
    const d = decode(frame);
    if (d) seen.push(d);
  }
  assert.equal(seen.length, 1);
  assert.equal(seen[0].text, 'after');
});

test('the base URL is built from host, port and prefix', () => {
  const plain = new Client({ host: '127.0.0.1', port: 8080 });
  assert.equal(plain.baseUrl, 'http://127.0.0.1:8080');

  const behindProxy = new Client({ host: 'agent.internal', port: 443, path: '/svpc/' });
  assert.equal(behindProxy.baseUrl, 'http://agent.internal:443/svpc');

  // A trailing slash on the prefix would otherwise double up at every path.
  const noPort = new Client({ host: 'agent.internal' });
  assert.equal(noPort.baseUrl, 'http://agent.internal');
});

test('version refuses a bridge that speaks another version of the API', async () => {
  const bridge = await serve((req, res) => {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ version: '2.0.0', api: 'v2' }));
  });
  try {
    const client = new Client({ port: bridge.port });
    await assert.rejects(() => client.version(), (err) => {
      assert.ok(err instanceof ApiError);
      assert.match(err.message, /speaks API v2/);
      return true;
    });
  } finally {
    await bridge.close();
  }
});

test('version accepts a bridge that speaks this one, and records its version', async () => {
  const bridge = await serve((req, res) => {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ version: '1.0.0', api: 'v1' }));
  });
  try {
    const client = new Client({ port: bridge.port });
    const body = await client.version();
    assert.equal(body.api, 'v1');
    assert.equal(client.serverVersion, '1.0.0');
  } finally {
    await bridge.close();
  }
});

test('models reports an unconfigured agent as not ready, without treating it as an error', async () => {
  // A fresh install has no provider. That is the normal state of a new user, so a
  // client that raised here would make the agent look broken when it is not.
  const bridge = await serve((req, res) => {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ ready: false, setup_error: 'no provider is configured' }));
  });
  try {
    const status = await new Client({ port: bridge.port }).models();
    assert.equal(status.ready, false);
    assert.equal(status.current, null);
    assert.match(status.setupError, /no provider/);
  } finally {
    await bridge.close();
  }
});

test('sessions come back as values, not raw dictionaries', async () => {
  const bridge = await serve((req, res) => {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(
      JSON.stringify({
        sessions: [
          { id: 'ses_1', title: 'first', created_at: 1758000000 },
          { id: 'ses_2', title: 'second', created_at: 1758000100 },
        ],
      })
    );
  });
  try {
    const list = await new Client({ port: bridge.port }).sessions();
    assert.equal(list.length, 2);
    assert.equal(list[0].id, 'ses_1');
    assert.equal(list[0].createdAt, 1758000000);
  } finally {
    await bridge.close();
  }
});

test('a 401 is reported as a credential problem, not a generic failure', async () => {
  // "It failed" is not actionable; the one thing a user can do about a 401 is fix
  // the token.
  const bridge = await serve((req, res) => {
    res.writeHead(401, { 'Content-Type': 'application/json' });
    res.end('unauthorized');
  });
  try {
    await assert.rejects(() => new Client({ port: bridge.port }).models(), (err) => {
      assert.ok(err instanceof ApiError);
      assert.equal(err.status, 401);
      assert.equal(err.unauthorized, true);
      return true;
    });
  } finally {
    await bridge.close();
  }
});

test('the token is sent as a bearer credential and never in the URL', async () => {
  let seenAuth = '';
  let seenUrl = '';
  const bridge = await serve((req, res) => {
    seenAuth = req.headers.authorization || '';
    seenUrl = req.url;
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ ready: true }));
  });
  try {
    const client = new Client({ port: bridge.port, token: 's3cret' });
    await client.models();
    assert.equal(seenAuth, 'Bearer s3cret');
    // A token in a URL ends up in logs, in history, and in a referrer header.
    assert.ok(!seenUrl.includes('s3cret'), `the token leaked into the URL: ${seenUrl}`);
  } finally {
    await bridge.close();
  }
});

test('ask yields the events of a turn in order', async () => {
  const bridge = await serve((req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
    res.write(SSE({ session_id: 'ses_abc' }));
    res.write(SSE({ tool: { id: 'c1', name: 'Bash', input: 'ls', state: 'running' } }));
    res.write(SSE({ text: 'Looking' }));
    res.write(SSE({ tool: { id: 'c1', name: 'Bash', input: 'ls', state: 'done' } }));
    res.write(SSE({ text: ' here.' }));
    res.write(SSE({ done: true }));
    res.end();
  });
  try {
    const events = [];
    for await (const ev of new Client({ port: bridge.port }).ask('look around')) {
      events.push(ev);
    }
    assert.equal(events[0].sessionId, 'ses_abc');
    assert.equal(events[1].tool.running, true);
    assert.equal(events[2].text, 'Looking');
    assert.equal(events[3].tool.done, true);
    assert.equal(events[4].text, ' here.');
  } finally {
    await bridge.close();
  }
});

test('askOnce assembles the answer and keeps the session', async () => {
  const bridge = await serve((req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write(SSE({ session_id: 'ses_xyz' }));
    res.write(SSE({ text: 'Hello, ' }));
    res.write(SSE({ text: 'world.' }));
    res.write(SSE({ done: true }));
    res.end();
  });
  try {
    const out = await new Client({ port: bridge.port }).askOnce('greet me');
    assert.equal(out.text, 'Hello, world.');
    assert.equal(out.sessionId, 'ses_xyz');
  } finally {
    await bridge.close();
  }
});

test('a turn that is already running is reported, not interleaved', async () => {
  // Two turns at once would leave an approval request with nowhere to go, so the
  // bridge refuses the second and a client has to hear about it.
  const bridge = await serve((req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write(SSE({ error: 'a turn is already running' }));
    res.end();
  });
  try {
    await assert.rejects(
      () => new Client({ port: bridge.port }).askOnce('second turn'),
      (err) => {
        assert.ok(err instanceof ApiError);
        assert.match(err.message, /already running/);
        return true;
      }
    );
  } finally {
    await bridge.close();
  }
});

test('a permission request is answered inside the turn, and the turn carries on', async () => {
  // This is the whole reason ask is a stream and not a promise: the turn is
  // suspended until the request is answered, so a client that collected the
  // request and returned would wait for a turn that can never finish.
  const answered = [];
  const bridge = await serve(async (req, res) => {
    if (req.url.endsWith('/permission')) {
      let body = '';
      for await (const chunk of req) body += chunk;
      answered.push(JSON.parse(body));
      res.writeHead(204);
      res.end();
      return;
    }
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write(SSE({ session_id: 'ses_p' }));
    res.write(
      SSE({
        permission: {
          id: 'perm_1',
          tool_name: 'bash',
          action: 'run',
          description: 'run the tests',
          path: '/repo',
        },
      })
    );
    res.write(SSE({ text: 'The tests pass.' }));
    res.write(SSE({ done: true }));
    res.end();
  });
  try {
    const seen = [];
    const out = await new Client({ port: bridge.port }).askOnce('run the tests', {
      config: {
        onPermission: async (request) => {
          seen.push(request);
          return ALLOW;
        },
      },
    });

    assert.equal(seen.length, 1);
    assert.equal(seen[0].id, 'perm_1');
    assert.equal(seen[0].tool, 'bash');
    assert.match(seen[0].description, /run the tests/);
    assert.deepEqual(answered, [{ id: 'perm_1', action: ALLOW }]);
    assert.equal(out.text, 'The tests pass.');
  } finally {
    await bridge.close();
  }
});

test('a permission request with no handler is denied rather than left hanging', async () => {
  let answered = null;
  const bridge = await serve(async (req, res) => {
    if (req.url.endsWith('/permission')) {
      let body = '';
      for await (const chunk of req) body += chunk;
      answered = JSON.parse(body);
      res.writeHead(204);
      res.end();
      return;
    }
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write(SSE({ permission: { id: 'perm_2', tool_name: 'write', action: 'write' } }));
    res.write(SSE({ done: true }));
    res.end();
  });
  try {
    await new Client({ port: bridge.port }).askOnce('write something');
    // Denying is the safe outcome for a program nobody is watching; leaving the
    // turn suspended would be neither safe nor an answer.
    assert.deepEqual(answered, { id: 'perm_2', action: DENY });
  } finally {
    await bridge.close();
  }
});

test('the permission callback is not sent to the bridge as configuration', async () => {
  // It is a function. JSON.stringify drops it, but the client must not rely on
  // that: a function that became a field would be a leak of client internals into
  // the request, and would break the day someone made it serialisable.
  let body = null;
  const bridge = await serve(async (req, res) => {
    if (req.url.endsWith('/chat')) {
      let raw = '';
      for await (const chunk of req) raw += chunk;
      body = JSON.parse(raw);
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      res.write(SSE({ done: true }));
      res.end();
      return;
    }
    res.writeHead(404);
    res.end();
  });
  try {
    await new Client({ port: bridge.port }).askOnce('hello', {
      sessionId: 'ses_1',
      config: { model: 'gpt-4.1', onPermission: () => ALLOW },
    });
    assert.equal(body.session_id, 'ses_1');
    assert.equal(body.content, 'hello');
    assert.equal(body.config.model, 'gpt-4.1');
    assert.ok(!('onPermission' in body.config), 'the client callback reached the bridge');
  } finally {
    await bridge.close();
  }
});

test('an empty prompt is refused before anything is sent', async () => {
  let requested = false;
  const bridge = await serve((req, res) => {
    requested = true;
    res.writeHead(200);
    res.end();
  });
  try {
    const client = new Client({ port: bridge.port });
    await assert.rejects(() => client.askOnce('   '), ApiError);
    // A turn with nothing in it is a caller mistake, and it should not reach the
    // bridge: it would open a session and immediately close it.
    assert.equal(requested, false);
  } finally {
    await bridge.close();
  }
});
