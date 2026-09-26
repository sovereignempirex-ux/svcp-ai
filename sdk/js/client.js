/**
 * A client for the SVPC AI HTTP API.
 *
 * The agent runs on a machine; this talks to it. Start it with
 *
 *     svpc --serve 0.0.0.0
 *
 * and point this at the address it prints, with the token beside it.
 *
 * The contract this implements is docs/api.md in the SVPC AI repository. It is
 * versioned: the client checks `/api/v1/version` on connect and refuses a bridge
 * that speaks something else, rather than failing later on a reply shape it
 * misreads.
 *
 * No dependencies, and nothing Node-specific: it uses fetch and ReadableStream,
 * which a browser, Deno, Bun and Node 18 all have. That is deliberate — the
 * Android app is a WebView, so the same code runs there.
 */

/** A decision on an approval request. Mirrors the API's three actions. */
export const ALLOW = 'allow';
export const ALLOW_SESSION = 'allow_session';
export const DENY = 'deny';

/** A tool the agent is using. */
export class Tool {
  constructor({ id = '', name = '', input = '', state = '' } = {}) {
    this.id = id;
    this.name = name;
    this.input = input;
    /** 'running' or 'done'. */
    this.state = state;
  }
  get running() {
    return this.state === 'running';
  }
  get done() {
    return this.state === 'done';
  }
}

/** A tool that failed. The turn carries on. */
export class ToolError {
  constructor({ id = '', error = '' } = {}) {
    this.id = id;
    this.error = error;
  }
}

/** An approval request. The turn is waiting for an answer. */
export class Permission {
  constructor({ id = '', tool_name = '', action = '', description = '', path = '', diff = '' } = {}) {
    this.id = id;
    this.tool = tool_name;
    this.action = action;
    this.description = description;
    this.path = path;
    this.diff = diff;
  }
}

/** A model, as the agent reports it. */
export class Model {
  constructor({ id = '', name = '', provider = '', context_window = 0 } = {}) {
    this.id = id;
    this.name = name;
    this.provider = provider;
    this.contextWindow = context_window;
  }
}

/** A stored conversation. */
export class Session {
  constructor({ id = '', title = '', created_at = 0 } = {}) {
    this.id = id;
    this.title = title;
    this.createdAt = created_at;
  }
}

/** Raised when the bridge answers with a status that is not a success. */
export class ApiError extends Error {
  constructor(message, { status = 0, path = '' } = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.path = path;
  }

  /** True when the credential was refused, which is a configuration problem. */
  get unauthorized() {
    return this.status === 401 || this.status === 403;
  }
}

/** The API version this client speaks. */
export const API_VERSION = 'v1';

/** The options a client is built with. Everything is optional. */
export class ClientOptions {
  constructor({
    host = '127.0.0.1',
    port = 0,
    token = '',
    path = '',
    timeout = 0,
    fetch: fetchImpl = undefined,
  } = {}) {
    this.host = host;
    this.port = port;
    this.token = token;
    /** A path prefix, for a bridge behind a reverse proxy. */
    this.path = path.replace(/\/+$/, '');
    /** Milliseconds before a request is abandoned. 0 means never. */
    this.timeout = timeout;
    /** Injected for testing. */
    this.fetchImpl = fetchImpl;
  }
}

/**
 * A connection to one SVPC AI agent.
 *
 *     const agent = new Client({ host: '192.168.1.20', port: 8080, token });
 *     const { ready } = await agent.models();
 *     for await (const event of agent.ask('what changed here?')) {
 *       if (event.text) process.stdout.write(event.text);
 *     }
 */
export class Client {
  /** @param {ClientOptions|object} options */
  constructor(options = {}) {
    this.options = options instanceof ClientOptions ? options : new ClientOptions(options);
    /** Set once the version handshake has succeeded. */
    this.serverVersion = '';
  }

  /** The base URL every request is made against. */
  get baseUrl() {
    const { host, port, path } = this.options;
    const authority = port ? `${host}:${port}` : host;
    return `http://${authority}${path}`;
  }

  /**
   * Check that the bridge is there and speaks this version of the API.
   *
   * Worth calling before anything else: a bridge that does not answer this is a
   * bridge this client cannot use, and finding that out from a misread field
   * during a stream is much harder to act on.
   *
   * @returns {Promise<{version: string, api: string}>}
   */
  async version() {
    const body = await this.#request('GET', `/api/${API_VERSION}/version`);
    if (body.api !== API_VERSION) {
      throw new ApiError(
        `the bridge speaks API ${body.api}, and this client speaks ${API_VERSION}`,
        { path: '/api/version' }
      );
    }
    this.serverVersion = body.version || '';
    return body;
  }

  /**
   * What the agent can do, and whether it is configured.
   *
   * `ready` is false on a fresh install, which is not an error: the agent is
   * running and simply has no provider yet.
   */
  async models() {
    const body = await this.#request('GET', `/api/${API_VERSION}/models`);
    return {
      ready: Boolean(body.ready),
      current: body.current ? new Model(body.current) : null,
      setupError: body.setup_error || '',
    };
  }

  /** Stored conversations, newest first. */
  async sessions() {
    const body = await this.#request('GET', `/api/${API_VERSION}/sessions`);
    return (body.sessions || []).map((s) => new Session(s));
  }

  /**
   * Answer an approval request.
   *
   * `action` is ALLOW, ALLOW_SESSION or DENY. A second answer to the same request
   * is ignored by the bridge, so a client may retry after a timeout without
   * reversing a decision a person already saw.
   */
  async answer(id, action = ALLOW) {
    if (!id) throw new ApiError('an approval request needs an id', { path: '/api/permission' });
    await this.#request('POST', `/api/${API_VERSION}/permission`, { id, action });
  }

  /**
   * Run one turn, yielding events as they happen.
   *
   * An async generator, so `for await` reads the answer as it is written. Closing
   * the loop early — a `break` — disconnects, and the bridge treats that as
   * cancelling the turn.
   *
   * @param {string} prompt
   * @param {{sessionId?: string, config?: object, signal?: AbortSignal}} [options]
   */
  async *ask(prompt, { sessionId = '', config = {}, signal = undefined } = {}) {
    if (!prompt || !prompt.trim()) {
      throw new ApiError('the prompt is empty', { path: '/api/chat' });
    }

    // A permission request arrives inside the stream and the turn is waiting, so
    // answering it has to be possible from inside the loop that is reading the
    // stream. A caller supplies onPermission; without one, a request is denied,
    // which is the safe outcome for a program nobody is watching.
    const onPermission = config.onPermission;
    const body = { session_id: sessionId, content: prompt, config: stripCallbacks(config) };
    delete body.config.onPermission;

    const response = await this.#fetch(`/api/${API_VERSION}/chat`, {
      method: 'POST',
      headers: this.#headers(),
      body: JSON.stringify(body),
      signal,
    });

    if (!response.ok) {
      throw await this.#errorFor(response, '/api/chat');
    }
    if (!response.body) {
      throw new ApiError('the bridge sent no event stream', { path: '/api/chat' });
    }

    for await (const frame of readEvents(response.body)) {
      const event = decode(frame);
      if (!event) continue;

      if (event.permission) {
        const request = new Permission(event.permission);
        if (onPermission) {
          const decision = await onPermission(request);
          // Answered here rather than handed to the caller, because the turn is
          // suspended until it is: a caller that collected the request and carried
          // on would watch a turn that never finishes.
          await this.answer(request.id, decision || DENY);
        } else {
          await this.answer(request.id, DENY);
        }
        continue;
      }
      if (event.tool) yield { tool: new Tool(event.tool) };
      else if (event.tool_error) yield { toolError: new ToolError(event.tool_error) };
      else if (event.error) throw new ApiError(event.error, { path: '/api/chat' });
      else if (event.session_id) yield { sessionId: event.session_id };
      else if (event.text) yield { text: event.text };
      else if (event.done) return;
    }
  }

  /**
   * Run one turn and return the whole reply as a string, for a caller that does
   * not need to watch it happen.
   *
   * @returns {Promise<{text: string, sessionId: string, tools: Tool[], toolErrors: ToolError[]}>}
   */
  async askOnce(prompt, options = {}) {
    let text = '';
    let sessionId = options.sessionId || '';
    const tools = [];
    const toolErrors = [];
    for await (const event of this.ask(prompt, options)) {
      if (event.text) text += event.text;
      if (event.sessionId) sessionId = event.sessionId;
      if (event.tool) tools.push(event.tool);
      if (event.toolError) toolErrors.push(event.toolError);
    }
    return { text, sessionId, tools, toolErrors };
  }

  /** Headers for a request, including the credential when there is one. */
  #headers() {
    const headers = { 'Content-Type': 'application/json', Accept: 'application/json' };
    if (this.options.token) headers.Authorization = `Bearer ${this.options.token}`;
    return headers;
  }

  #url(path) {
    return `${this.baseUrl}${path}`;
  }

  async #fetch(path, init) {
    const { fetchImpl, timeout } = this.options;
    const doFetch = fetchImpl || globalThis.fetch;
    if (typeof doFetch !== 'function') {
      throw new ApiError('no fetch is available; pass one in the options', { path });
    }
    if (!timeout) return doFetch(this.#url(path), init);

    // A bridge that stops answering would otherwise leave the caller waiting
    // forever, and a turn with no answer is indistinguishable from a slow one.
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeout);
    if (init.signal) {
      init.signal.addEventListener('abort', () => controller.abort(), { once: true });
    }
    try {
      return await doFetch(this.#url(path), { ...init, signal: controller.signal });
    } finally {
      clearTimeout(timer);
    }
  }

  async #request(method, path, body) {
    const init = { method, headers: this.#headers() };
    if (body !== undefined) init.body = JSON.stringify(body);

    const response = await this.#fetch(path, init);
    if (!response.ok) {
      throw await this.#errorFor(response, path);
    }
    if (response.status === 204) return {};
    const text = await response.text();
    if (!text) return {};
    try {
      return JSON.parse(text);
    } catch (cause) {
      throw new ApiError(`the reply was not JSON: ${text.slice(0, 120)}`, { path });
    }
  }

  /** An error carrying the status, because "it failed" is not actionable. */
  async #errorFor(response, path) {
    let detail = '';
    try {
      detail = (await response.text()).trim();
    } catch {
      // A body that cannot be read is not worth reporting over the status.
    }
    const message = detail || `${response.status} ${response.statusText || ''}`.trim();
    return new ApiError(message, { status: response.status, path });
  }
}

/** The callbacks are the client's own, not the bridge's, so they are not sent. */
function stripCallbacks(config) {
  const { onPermission, ...rest } = config || {};
  return rest;
}

/**
 * Turn one SSE frame into an object, or null if it is not one.
 *
 * Exported because it is the part most worth testing on its own: a frame is
 * separated by a blank line and carries its payload on a data line, and a client
 * that got either detail wrong would work in a test and fail over a network.
 */
export function decode(frame) {
  const line = frame.split('\n').find((l) => l.startsWith('data:'));
  if (!line) return null;
  const payload = line.slice(5).trim();
  if (!payload) return null;
  try {
    return JSON.parse(payload);
  } catch {
    // A frame that is not JSON is skipped rather than thrown: a stream is a
    // sequence, and one bad frame is not a reason to lose the ones after it.
    return null;
  }
}

/**
 * Read server-sent events out of a response body, one complete frame at a time.
 *
 * Split on the blank line that ends a frame rather than on newlines, because a
 * frame's JSON can contain a newline escaped as \\n but never a raw one, and
 * anything else would cut a frame in half.
 */
export async function* readEvents(body) {
  const decoder = new TextDecoder();
  const reader = body.getReader();
  let buffer = '';
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      let split;
      while ((split = buffer.indexOf('\n\n')) !== -1) {
        const frame = buffer.slice(0, split);
        buffer = buffer.slice(split + 2);
        if (frame.trim()) yield frame;
      }
    }
    // Whatever is left did not end with a blank line. Emitted anyway, because a
    // bridge that closes straight after the last event is behaving correctly.
    if (buffer.trim()) yield buffer;
  } finally {
    // Releasing the lock lets the request be cancelled by dropping the loop, which
    // is how a caller stops a turn.
    try {
      reader.releaseLock();
    } catch {
      // Already released, which is fine.
    }
  }
}
