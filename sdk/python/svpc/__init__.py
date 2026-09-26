"""A client for the SVPC AI HTTP API.

The agent runs on a machine; this talks to it. Start it with

    svpc --serve 0.0.0.0

and point this at the address it prints, with the token beside it.

The contract this implements is docs/api.md in the SVPC AI repository. It is
versioned: :meth:`Client.version` reports what the bridge speaks, and
:meth:`Client.connect` refuses one that speaks something else rather than failing
later on a reply shape it misreads.

The standard library only. A client for a tool that runs commands ought not to
drag a dependency tree into the program that uses it, and everything here —
urllib, threads, json — is in every Python that has ever run.

    from svpc import Client, ALLOW

    with Client("192.168.1.20", 8080, token) as agent:
        for event in agent.ask("what changed here?"):
            if event.text:
                print(event.text, end="", flush=True)

The stream is a generator, and a turn is cancelled by closing it or by closing the
client, which is how the bridge learns to stop.
"""

from __future__ import annotations

import json
import threading
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Callable, Iterator, Optional

__all__ = [
    "ALLOW",
    "ALLOW_SESSION",
    "DENY",
    "API_VERSION",
    "ApiError",
    "Client",
    "Event",
    "Model",
    "Permission",
    "Session",
    "Tool",
    "ToolError",
]

#: The API version this client speaks.
API_VERSION = "v1"

#: Answers to an approval request. Mirrors the three the bridge accepts.
ALLOW = "allow"
ALLOW_SESSION = "allow_session"
DENY = "deny"


class ApiError(RuntimeError):
    """The bridge answered with a status that is not a success.

    Carries the status because "it failed" is not actionable, and the one thing a
    person can do about a 401 is fix the token.
    """

    def __init__(self, message: str, status: int = 0, path: str = ""):
        super().__init__(message)
        self.status = status
        self.path = path

    @property
    def unauthorized(self) -> bool:
        """True when the credential was refused, which is a configuration problem."""
        return self.status in (401, 403)


@dataclass(frozen=True)
class Tool:
    """A tool the agent is using."""

    id: str = ""
    name: str = ""
    input: str = ""
    state: str = ""

    @property
    def running(self) -> bool:
        return self.state == "running"

    @property
    def done(self) -> bool:
        return self.state == "done"


@dataclass(frozen=True)
class ToolError:
    """A tool that failed. The turn carries on."""

    id: str = ""
    error: str = ""


@dataclass(frozen=True)
class Permission:
    """An approval request. The turn is waiting for an answer."""

    id: str = ""
    tool: str = ""
    action: str = ""
    description: str = ""
    path: str = ""
    diff: str = ""


@dataclass(frozen=True)
class Model:
    """A model, as the agent reports it."""

    id: str = ""
    name: str = ""
    provider: str = ""
    context_window: int = 0


@dataclass(frozen=True)
class Session:
    """A stored conversation."""

    id: str = ""
    title: str = ""
    created_at: int = 0


@dataclass
class Event:
    """One thing that happened during a turn.

    Exactly one of ``text``, ``tool``, ``tool_error``, ``permission`` and
    ``error`` carries a value. ``session_id`` arrives once, on the first event.
    """

    session_id: str = ""
    text: str = ""
    tool: Optional[Tool] = None
    tool_error: Optional[ToolError] = None
    permission: Optional[Permission] = None
    error: Optional[str] = None

    @property
    def done(self) -> bool:
        return self.session_id == "" and not any(
            (self.text, self.tool, self.tool_error, self.permission, self.error)
        )


class Client:
    """A connection to one SVPC AI agent.

    :param host: the machine the agent runs on.
    :param port: the port it printed. 0 means the default 80.
    :param token: the credential, when the bridge is not on loopback.
    :param path: a prefix, for a bridge behind a reverse proxy.
    :param timeout: seconds before a request is abandoned. 0 means never, which
        is right for a turn, since a turn is long by nature.
    """

    def __init__(
        self,
        host: str = "127.0.0.1",
        port: int = 0,
        token: str = "",
        path: str = "",
        timeout: float = 0,
    ):
        self.host = host
        self.port = port
        self.token = token
        self.path = path.rstrip("/")
        self.timeout = timeout
        #: Set once the version handshake has succeeded.
        self.server_version = ""
        self._lock = threading.Lock()
        self._closed = False

    # ── Plumbing ───────────────────────────────────────────────────

    @property
    def base_url(self) -> str:
        authority = f"{self.host}:{self.port}" if self.port else self.host
        return f"http://{authority}{self.path}"

    def _headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json", "Accept": "application/json"}
        if self.token:
            # Never in the URL: a URL ends up in logs, in history and in a
            # referrer header, and this one grants the power to run commands.
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    def _open(self, path: str, data: Optional[bytes] = None, method: str = "GET"):
        request = urllib.request.Request(
            self.base_url + path,
            data=data,
            headers=self._headers(),
            method=method,
        )
        try:
            return urllib.request.urlopen(request, timeout=self.timeout or None)
        except urllib.error.HTTPError as err:
            detail = ""
            try:
                detail = err.read().decode("utf-8", "replace").strip()
            except Exception:  # pragma: no cover - a body that cannot be read
                pass
            raise ApiError(
                detail or f"{err.code} {err.reason}", status=err.code, path=path
            ) from err
        except urllib.error.URLError as err:
            # A bridge that is not there is the common case by far, and saying so
            # is more use than a stack trace from inside urllib.
            raise ApiError(
                f"could not reach the agent at {self.base_url}: {err.reason}",
                path=path,
            ) from err

    def _get_json(self, path: str) -> dict[str, Any]:
        with self._open(path) as response:
            raw = response.read().decode("utf-8", "replace")
        if not raw:
            return {}
        try:
            return json.loads(raw)
        except json.JSONDecodeError as err:
            raise ApiError(f"the reply was not JSON: {raw[:120]}", path=path) from err

    def _post_json(self, path: str, body: dict[str, Any]) -> None:
        payload = json.dumps(body).encode("utf-8")
        with self._open(path, data=payload, method="POST"):
            pass

    # ── Endpoints ──────────────────────────────────────────────────

    def version(self) -> dict[str, Any]:
        """What this bridge is, and which version of the API it speaks.

        Raises :class:`ApiError` when the versions differ, because a client that
        carried on would misread every reply that followed.
        """
        body = self._get_json(f"/api/{API_VERSION}/version")
        if body.get("api") != API_VERSION:
            raise ApiError(
                f"the bridge speaks API {body.get('api')}, and this client speaks {API_VERSION}",
                path="/api/version",
            )
        self.server_version = body.get("version", "")
        return body

    def connect(self) -> dict[str, Any]:
        """Check the bridge is there and usable.

        Worth calling before anything else: a bridge that does not answer this is
        one this client cannot use, and finding that out from a misread field during
        a stream is much harder to act on.
        """
        return self.version()

    def models(self) -> dict[str, Any]:
        """What the agent can do.

        ``ready`` is false on a fresh install, which is not an error: the agent is
        running and simply has no provider yet.
        """
        body = self._get_json(f"/api/{API_VERSION}/models")
        current = body.get("current")
        return {
            "ready": bool(body.get("ready")),
            "current": _model(current) if current else None,
            "setup_error": body.get("setup_error", ""),
        }

    def sessions(self) -> list[Session]:
        """Stored conversations, newest first."""
        body = self._get_json(f"/api/{API_VERSION}/sessions")
        return [
            Session(id=s.get("id", ""), title=s.get("title", ""), created_at=s.get("created_at", 0))
            for s in body.get("sessions", [])
        ]

    def answer(self, request_id: str, action: str = ALLOW) -> None:
        """Answer an approval request.

        A second answer to the same request is ignored by the bridge, so a client
        may retry after a timeout without reversing a decision a person already saw.
        """
        if not request_id:
            raise ApiError("an approval request needs an id", path="/api/permission")
        self._post_json(f"/api/{API_VERSION}/permission", {"id": request_id, "action": action})

    def ask(
        self,
        prompt: str,
        session_id: str = "",
        config: Optional[dict[str, Any]] = None,
        on_permission: Optional[Callable[[Permission], str]] = None,
    ) -> Iterator[Event]:
        """Run one turn, yielding events as they happen.

        ``on_permission`` is called when the agent needs approval, and the turn is
        suspended until it answers: returning :data:`ALLOW`, :data:`ALLOW_SESSION`
        or :data:`DENY`. Without one, a request is denied, which is the safe outcome
        for a program nobody is watching.

        It is a generator, so nothing is sent until it is iterated, and abandoning
        the loop closes the connection — which is how a turn is cancelled.
        """
        if not prompt or not prompt.strip():
            raise ApiError("the prompt is empty", path="/api/chat")

        body: dict[str, Any] = {"session_id": session_id, "content": prompt, "config": dict(config or {})}
        # The handler is this client's, not the bridge's, so it is not sent. It is
        # a function and json would drop it anyway, but relying on that would make
        # the request's shape depend on an accident.
        body["config"].pop("on_permission", None)

        payload = json.dumps(body).encode("utf-8")
        response = self._open(f"/api/{API_VERSION}/chat", data=payload, method="POST")
        try:
            for event in self._events(response, on_permission):
                yield event
        finally:
            # Closed on the way out, including when the caller breaks out of the
            # loop, so the bridge learns the turn is over rather than waiting for a
            # grace period to expire.
            response.close()

    def ask_once(
        self,
        prompt: str,
        session_id: str = "",
        config: Optional[dict[str, Any]] = None,
        on_permission: Optional[Callable[[Permission], str]] = None,
    ) -> dict[str, Any]:
        """Run one turn and return the whole reply, for a caller that does not need
        to watch it happen.

        Returns the text, the session id, and the tools that ran. The session id
        comes back because a caller that wants a second turn needs it and there is
        nowhere else to get it: it is the one value the turn discovers.
        """
        text: list[str] = []
        tools: list[Tool] = []
        tool_errors: list[ToolError] = []
        found_session = session_id

        for event in self.ask(prompt, session_id, config, on_permission):
            if event.session_id:
                found_session = event.session_id
            if event.text:
                text.append(event.text)
            if event.tool:
                tools.append(event.tool)
            if event.tool_error:
                tool_errors.append(event.tool_error)

        return {
            "text": "".join(text),
            "session_id": found_session,
            "tools": tools,
            "tool_errors": tool_errors,
        }

    # ── The stream ─────────────────────────────────────────────────

    def _events(
        self, response, on_permission: Optional[Callable[[Permission], str]]
    ) -> Iterator[Event]:
        for frame in _frames(response):
            event = _decode(frame)
            if event is None:
                # A frame that is not JSON is skipped rather than raised: a stream
                # is a sequence, and one bad frame is not a reason to lose the rest
                # of the turn.
                continue

            permission = event.get("permission")
            if permission is not None:
                request = _permission(permission)
                decision = on_permission(request) if on_permission else DENY
                # Answered here rather than handed to the caller, because the turn
                # is suspended until it is. A caller that collected the request and
                # carried on would be watching a turn that can never finish.
                self.answer(request.id, decision or DENY)
                continue

            if "tool" in event:
                yield Event(tool=_tool(event["tool"]))
            elif "tool_error" in event:
                yield Event(tool_error=_tool_error(event["tool_error"]))
            elif "error" in event:
                raise ApiError(event["error"], path="/api/chat")
            elif "session_id" in event:
                yield Event(session_id=event["session_id"])
            elif "text" in event:
                yield Event(text=event["text"])
            elif event.get("done"):
                return

    def close(self) -> None:
        """Release the client.

        Safe to call more than once, so a ``with`` block and an explicit close can
        both happen.
        """
        with self._lock:
            self._closed = True

    def __enter__(self) -> "Client":
        return self

    def __exit__(self, *exc) -> None:
        self.close()


# ── Wire format ───────────────────────────────────────────────────


def _frames(response, chunk_size: int = 4096) -> Iterator[str]:
    """Yield complete server-sent event frames from a response.

    Split on the blank line that ends a frame rather than on newlines: a frame's
    JSON never contains a raw newline, and anything else would cut a frame in half.
    A read can land anywhere, including between the two bytes of a multi-byte
    character, so the buffer is carried across reads and decoded as it grows rather
    than per chunk.
    """
    buffer = ""
    while True:
        chunk = response.read(chunk_size)
        if not chunk:
            break
        buffer += chunk.decode("utf-8", "replace")
        while "\n\n" in buffer:
            frame, buffer = buffer.split("\n\n", 1)
            if frame.strip():
                yield frame
    # Whatever is left did not end with a blank line. Emitted anyway, because a
    # bridge that closes straight after the last event is behaving correctly.
    if buffer.strip():
        yield buffer


def _decode(frame: str) -> Optional[dict[str, Any]]:
    for line in frame.split("\n"):
        if not line.startswith("data:"):
            continue
        payload = line[5:].strip()
        if not payload:
            return None
        try:
            return json.loads(payload)
        except json.JSONDecodeError:
            return None
    return None


def _tool(raw: dict[str, Any]) -> Tool:
    return Tool(
        id=raw.get("id", ""),
        name=raw.get("name", ""),
        input=raw.get("input", ""),
        state=raw.get("state", ""),
    )


def _tool_error(raw: dict[str, Any]) -> ToolError:
    return ToolError(id=raw.get("id", ""), error=raw.get("error", ""))


def _permission(raw: dict[str, Any]) -> Permission:
    return Permission(
        id=raw.get("id", ""),
        tool=raw.get("tool_name", ""),
        action=raw.get("action", ""),
        description=raw.get("description", ""),
        path=raw.get("path", ""),
        diff=raw.get("diff", ""),
    )


def _model(raw: dict[str, Any]) -> Model:
    return Model(
        id=raw.get("id", ""),
        name=raw.get("name", ""),
        provider=raw.get("provider", ""),
        context_window=raw.get("context_window", 0),
    )
