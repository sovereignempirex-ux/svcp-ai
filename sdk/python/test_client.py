"""The Python client is tested against a real HTTP server speaking the protocol the
bridge does, not a mock of the client. A mock would agree with the client by
construction, which is the one thing a test of a parser must not do.

Run with:  python -m unittest discover -s sdk/python
"""

import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from svpc import (
    ALLOW,
    API_VERSION,
    DENY,
    ApiError,
    Client,
    _decode,
    _frames,
)


def sse(obj) -> str:
    """The exact bytes the bridge writes for one event."""
    return "data: " + json.dumps(obj) + "\n\n"


class Chunked:
    """A response body that hands out a fixed number of bytes at a time.

    The size is a parameter because where a read lands is the whole question: a
    client that only works when a read happens to align with a frame boundary is a
    client that works in a test and fails over a network.
    """

    def __init__(self, data: bytes, size: int):
        self.data = data
        self.size = size
        self.at = 0

    def read(self, n: int) -> bytes:
        out = self.data[self.at : self.at + self.size]
        self.at += len(out)
        return out

    def close(self):
        pass


class Bridge:
    """A stand-in bridge.

    ``responder`` is a function of (method, path, body) returning
    (status, headers, payload). The last request is recorded on ``self.last`` so a
    test can check what the client actually sent, which is the only way to be sure a
    credential was presented rather than merely built.
    """

    def __init__(self, responder):
        self.responder = responder
        self.last = None
        self.requests = []
        owner = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *args):  # keep the test output readable
                pass

            def do_GET(self):
                self._respond("GET", self.path, b"")

            def do_POST(self):
                length = int(self.headers.get("Content-Length", "0"))
                self._respond("POST", self.path, self.rfile.read(length))

            def _respond(self, method, path, body):
                owner.last = {
                    "method": method,
                    "path": path,
                    "headers": dict(self.headers),
                    "body": body,
                }
                owner.requests.append(owner.last)
                status, headers, payload = owner.responder(method, path, body)
                self.send_response(status)
                for key, value in headers.items():
                    self.send_header(key, value)
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = self.server.server_address[1]
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def stop(self):
        self.server.shutdown()
        self.server.server_close()

    def client(self, **kwargs) -> Client:
        return Client(port=self.port, **kwargs)


def json_responder(payload, status=200):
    """A bridge that answers everything with one JSON body."""

    def responder(method, path, body):
        return status, {"Content-Type": "application/json"}, json.dumps(payload).encode()

    return responder


def stream_responder(*frames: str):
    """A bridge whose chat endpoint writes the given frames, byte for byte."""

    def responder(method, path, body):
        if path.endswith("/permission"):
            return 204, {}, b""
        return (
            200,
            {"Content-Type": "text/event-stream", "Cache-Control": "no-cache"},
            "".join(frames).encode(),
        )

    return responder


class Framing(unittest.TestCase):
    """The stream is where a client usually breaks, so it is tested hardest."""

    def test_a_frame_split_across_reads_is_reassembled(self):
        # One byte per read, which is the worst case a network can produce and the
        # case a client that assumed one event per read gets wrong.
        frames = list(_frames(Chunked(b'data: {"text":"hello"}\n\ndata: {"done":true}\n\n', 1)))
        self.assertEqual(len(frames), 2)
        self.assertEqual(_decode(frames[0])["text"], "hello")
        self.assertTrue(_decode(frames[1])["done"])

    def test_a_frame_split_between_the_bytes_of_a_character_is_reassembled(self):
        # A multi-byte character straddling two reads is the case that loses a
        # character and produces a reply that will not parse.
        raw = "data: " + json.dumps({"text": "café ☕"}) + "\n\n"
        frames = list(_frames(Chunked(raw.encode("utf-8"), 1)))
        self.assertEqual(len(frames), 1)
        self.assertEqual(_decode(frames[0])["text"], "café ☕")

    def test_several_frames_in_one_read_are_all_delivered(self):
        # The opposite failure: a parser that treats a read as a frame loses the
        # rest of the turn.
        payload = (sse({"text": "one"}) + sse({"text": "two"}) + sse({"done": True})).encode()
        frames = list(_frames(Chunked(payload, 4096)))
        self.assertEqual(len(frames), 3)
        self.assertEqual([_decode(f).get("text") for f in frames[:2]], ["one", "two"])

    def test_a_trailing_frame_without_a_blank_line_is_still_delivered(self):
        frames = list(_frames(Chunked(b'data: {"done":true}', 4096)))
        self.assertEqual(len(frames), 1)
        self.assertTrue(_decode(frames[0])["done"])

    def test_a_frame_that_is_not_json_is_skipped_not_raised(self):
        self.assertIsNone(_decode("data: not json"))
        self.assertIsNone(_decode("this frame has no data line"))
        # A bridge that emits a comment line, as the SSE format allows.
        self.assertIsNone(_decode(": keep-alive"))


class BaseUrl(unittest.TestCase):
    def test_the_url_is_built_from_host_port_and_prefix(self):
        self.assertEqual(Client(host="127.0.0.1", port=8080).base_url, "http://127.0.0.1:8080")
        self.assertEqual(
            Client(host="agent.internal", port=443, path="/svpc/").base_url,
            "http://agent.internal:443/svpc",
        )
        # A trailing slash on the prefix would otherwise double at every path.
        self.assertEqual(Client(host="agent.internal").base_url, "http://agent.internal")


class Version(unittest.TestCase):
    def test_a_bridge_speaking_another_version_is_refused(self):
        bridge = Bridge(json_responder({"version": "2.0.0", "api": "v2"}))
        try:
            with self.assertRaises(ApiError) as caught:
                bridge.client().version()
            self.assertIn("speaks API v2", str(caught.exception))
        finally:
            bridge.stop()

    def test_a_bridge_speaking_this_version_is_accepted(self):
        bridge = Bridge(json_responder({"version": "1.0.0", "api": "v1"}))
        try:
            client = bridge.client()
            body = client.connect()
            self.assertEqual(body["api"], "v1")
            self.assertEqual(client.server_version, "1.0.0")
        finally:
            bridge.stop()


class Status(unittest.TestCase):
    def test_an_unconfigured_agent_is_not_an_error(self):
        # A fresh install has no provider. That is the normal state of a new user,
        # so a client that raised here would make the agent look broken.
        bridge = Bridge(json_responder({"ready": False, "setup_error": "no provider is configured"}))
        try:
            status = bridge.client().models()
            self.assertFalse(status["ready"])
            self.assertIsNone(status["current"])
            self.assertIn("no provider", status["setup_error"])
        finally:
            bridge.stop()

    def test_sessions_come_back_as_values(self):
        bridge = Bridge(
            json_responder(
                {"sessions": [{"id": "ses_1", "title": "first", "created_at": 1758000000}]}
            )
        )
        try:
            sessions = bridge.client().sessions()
            self.assertEqual(len(sessions), 1)
            self.assertEqual(sessions[0].id, "ses_1")
            self.assertEqual(sessions[0].created_at, 1758000000)
        finally:
            bridge.stop()

    def test_a_401_is_reported_as_a_credential_problem(self):
        bridge = Bridge(json_responder({"error": "unauthorized"}, status=401))
        try:
            with self.assertRaises(ApiError) as caught:
                bridge.client().models()
            self.assertEqual(caught.exception.status, 401)
            self.assertTrue(caught.exception.unauthorized)
        finally:
            bridge.stop()

    def test_an_agent_that_is_not_there_says_so(self):
        # Nothing is listening on this port, which is the common case by far and
        # deserves a sentence rather than a stack trace from inside urllib.
        client = Client(port=1)
        with self.assertRaises(ApiError) as caught:
            client.models()
        self.assertIn("could not reach the agent", str(caught.exception))


class Credentials(unittest.TestCase):
    def test_the_token_is_sent_as_a_bearer_credential(self):
        bridge = Bridge(json_responder({"ready": True}))
        try:
            bridge.client(token="s3cret").models()
            self.assertEqual(bridge.last["headers"].get("Authorization"), "Bearer s3cret")
        finally:
            bridge.stop()

    def test_the_token_never_appears_in_a_url(self):
        # A URL ends up in logs, in shell history and in a referrer header, and
        # this one grants the power to run commands on the machine.
        bridge = Bridge(json_responder({"ready": True}))
        try:
            bridge.client(token="s3cret").models()
            self.assertNotIn("s3cret", bridge.last["path"])
        finally:
            bridge.stop()

    def test_no_authorization_header_without_a_token(self):
        # A loopback bridge needs no credential, and sending an empty one would
        # make it look like a failed attempt to authenticate.
        bridge = Bridge(json_responder({"ready": True}))
        try:
            bridge.client().models()
            self.assertNotIn("Authorization", bridge.last["headers"])
        finally:
            bridge.stop()


class Turn(unittest.TestCase):
    def test_events_arrive_in_order(self):
        bridge = Bridge(
            stream_responder(
                sse({"session_id": "ses_abc"}),
                sse({"tool": {"id": "c1", "name": "Bash", "input": "ls", "state": "running"}}),
                sse({"text": "Looking"}),
                sse({"tool": {"id": "c1", "name": "Bash", "input": "ls", "state": "done"}}),
                sse({"text": " here."}),
                sse({"done": True}),
            )
        )
        try:
            events = list(bridge.client().ask("look around"))
            self.assertEqual(events[0].session_id, "ses_abc")
            self.assertTrue(events[1].tool.running)
            self.assertEqual(events[2].text, "Looking")
            self.assertTrue(events[3].tool.done)
            self.assertEqual(events[4].text, " here.")
        finally:
            bridge.stop()

    def test_ask_once_assembles_the_answer_and_keeps_the_session(self):
        bridge = Bridge(
            stream_responder(
                sse({"session_id": "ses_xyz"}),
                sse({"text": "Hello, "}),
                sse({"text": "world."}),
                sse({"done": True}),
            )
        )
        try:
            out = bridge.client().ask_once("greet me")
            self.assertEqual(out["text"], "Hello, world.")
            self.assertEqual(out["session_id"], "ses_xyz")
        finally:
            bridge.stop()

    def test_a_second_turn_is_reported_rather_than_interleaved(self):
        bridge = Bridge(stream_responder(sse({"error": "a turn is already running"})))
        try:
            with self.assertRaises(ApiError) as caught:
                bridge.client().ask_once("second turn")
            self.assertIn("already running", str(caught.exception))
        finally:
            bridge.stop()

    def test_an_empty_prompt_never_reaches_the_bridge(self):
        reached = []

        def responder(method, path, body):
            reached.append(path)
            return 200, {"Content-Type": "text/event-stream"}, sse({"done": True}).encode()

        bridge = Bridge(responder)
        try:
            with self.assertRaises(ApiError):
                list(bridge.client().ask("   "))
            # A turn with nothing in it would open a session and close it again.
            self.assertEqual(reached, [])
        finally:
            bridge.stop()

    def test_a_permission_request_is_answered_inside_the_turn(self):
        # This is the whole reason ask is a generator: the turn is suspended until
        # the request is answered, so a client that collected the request and
        # returned would wait for a turn that can never finish.
        answered = []

        def responder(method, path, body):
            if path.endswith("/permission"):
                answered.append(json.loads(body))
                return 204, {}, b""
            return (
                200,
                {"Content-Type": "text/event-stream"},
                (
                    sse({"session_id": "ses_p"})
                    + sse(
                        {
                            "permission": {
                                "id": "perm_1",
                                "tool_name": "bash",
                                "action": "run",
                                "description": "run the tests",
                                "path": "/repo",
                            }
                        }
                    )
                    + sse({"text": "The tests pass."})
                    + sse({"done": True})
                ).encode(),
            )

        bridge = Bridge(responder)
        try:
            seen = []

            def handler(request):
                seen.append(request)
                return ALLOW

            out = bridge.client().ask_once("run the tests", on_permission=handler)
            self.assertEqual(len(seen), 1)
            self.assertEqual(seen[0].id, "perm_1")
            self.assertEqual(seen[0].tool, "bash")
            self.assertIn("run the tests", seen[0].description)
            self.assertEqual(answered, [{"id": "perm_1", "action": ALLOW}])
            self.assertEqual(out["text"], "The tests pass.")
        finally:
            bridge.stop()

    def test_a_permission_request_with_no_handler_is_denied(self):
        # Denying is safe for a program nobody is watching; leaving the turn
        # suspended would be neither safe nor an answer.
        answered = []

        def responder(method, path, body):
            if path.endswith("/permission"):
                answered.append(json.loads(body))
                return 204, {}, b""
            return (
                200,
                {"Content-Type": "text/event-stream"},
                (sse({"permission": {"id": "perm_2", "tool_name": "write"}}) + sse({"done": True})).encode(),
            )

        bridge = Bridge(responder)
        try:
            bridge.client().ask_once("write something")
            self.assertEqual(answered, [{"id": "perm_2", "action": DENY}])
        finally:
            bridge.stop()

    def test_the_handler_is_not_sent_as_configuration(self):
        # It is a function, and json would drop it, but relying on that would make
        # the request's shape depend on an accident.
        captured = {}

        def responder(method, path, body):
            if path.endswith("/chat"):
                captured.update(json.loads(body))
            return 200, {"Content-Type": "text/event-stream"}, sse({"done": True}).encode()

        bridge = Bridge(responder)
        try:
            bridge.client().ask_once(
                "hello",
                session_id="ses_1",
                config={"model": "gpt-4.1", "on_permission": lambda r: ALLOW},
            )
            self.assertEqual(captured["session_id"], "ses_1")
            self.assertEqual(captured["content"], "hello")
            self.assertEqual(captured["config"]["model"], "gpt-4.1")
            self.assertNotIn("on_permission", captured["config"])
        finally:
            bridge.stop()


if __name__ == "__main__":
    unittest.main()
