import io
import json
import os
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from mywant import Client, MyWantError  # noqa: E402
from mywant import plugin  # noqa: E402


class _Recorder(BaseHTTPRequestHandler):
    calls = []
    reply = {}

    def _handle(self):
        n = int(self.headers.get("Content-Length") or 0)
        body = json.loads(self.rfile.read(n)) if n else None
        _Recorder.calls.append((self.command, self.path, body, self.headers.get("Authorization")))
        status, payload = _Recorder.reply.get((self.command, self.path.split("?")[0]), (200, {"ok": True}))
        data = json.dumps(payload).encode() if payload is not None else b""
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    do_GET = do_POST = do_PUT = do_PATCH = do_DELETE = _handle

    def log_message(self, *a):
        pass


class ClientTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.srv = HTTPServer(("127.0.0.1", 0), _Recorder)
        threading.Thread(target=cls.srv.serve_forever, daemon=True).start()
        cls.url = f"http://127.0.0.1:{cls.srv.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.srv.shutdown()

    def setUp(self):
        _Recorder.calls = []
        _Recorder.reply = {}

    def test_wants_lists_and_filters(self):
        _Recorder.reply[("GET", "/api/v1/wants")] = (200, {"wants": [{"metadata": {"name": "a"}}]})
        got = Client(self.url).wants(type="enemy", labels={"mywant.io/enemy-of": "keep"})
        self.assertEqual(got, [{"metadata": {"name": "a"}}])
        method, path, _, _ = _Recorder.calls[0]
        self.assertEqual(method, "GET")
        self.assertIn("type=enemy", path)
        self.assertIn("label=mywant.io%2Fenemy-of%3Dkeep", path)

    def test_patch_sends_only_what_changes(self):
        Client(self.url).patch_want("w-1", params={"target": "帳簿"}, labels={"x": None})
        method, path, body, _ = _Recorder.calls[0]
        self.assertEqual((method, path), ("PATCH", "/api/v1/wants/w-1"))
        self.assertEqual(body, {"metadata": {"labels": {"x": None}}, "spec": {"params": {"target": "帳簿"}}})

    def test_create_sends_a_bare_list(self):
        Client(self.url).create_want({"metadata": {"name": "e", "type": "enemy"}})
        self.assertEqual(_Recorder.calls[0][2], [{"metadata": {"name": "e", "type": "enemy"}}])

    def test_batch_controls(self):
        Client(self.url).suspend("a", "b")
        self.assertEqual(_Recorder.calls[0][:3], ("POST", "/api/v1/wants/suspend", {"ids": ["a", "b"]}))

    def test_set_imports_keeps_the_rest_of_the_spec(self):
        _Recorder.reply[("GET", "/api/v1/wants/w-1")] = (200, {
            "metadata": {"id": "w-1", "name": "帳簿"}, "spec": {"params": {"max_hp": 10}}})
        self.assertTrue(Client(self.url).set_imports("w-1", {"e.attack": "action:e.attack"}))
        method, path, body, _ = _Recorder.calls[1]
        self.assertEqual((method, path), ("PUT", "/api/v1/wants/w-1"))
        self.assertEqual(body["spec"], {"params": {"max_hp": 10}, "imports": {"e.attack": "action:e.attack"}})

    def test_set_imports_leaves_what_is_already_so(self):
        _Recorder.reply[("GET", "/api/v1/wants/w-1")] = (200, {
            "metadata": {"id": "w-1"}, "spec": {"imports": {"e.attack": "action:e.attack"}}})
        self.assertFalse(Client(self.url).set_imports("w-1", {"e.attack": "action:e.attack"}))
        self.assertEqual([c[0] for c in _Recorder.calls], ["GET"])

    def test_expose_only_once(self):
        _Recorder.reply[("GET", "/api/v1/wants/w-1")] = (200, {
            "metadata": {"id": "w-1"}, "spec": {"exposes": [{"currentState": "strike", "as": "a.attack"}]}})
        self.assertFalse(Client(self.url).expose("w-1", "strike", "a.attack"))
        self.assertTrue(Client(self.url).expose("w-1", "pulse", "a.heal"))
        self.assertEqual(_Recorder.calls[-1][:3], ("POST", "/api/v1/wants/w-1/relations", {"currentState": "pulse", "as": "a.heal"}))

    def test_auth(self):
        Client(self.url, token="t0k").want("x")
        self.assertEqual(_Recorder.calls[-1][3], "Bearer t0k")
        Client(self.url, username="mywant", password="pw").want("x")
        self.assertEqual(_Recorder.calls[-1][3], "Basic bXl3YW50OnB3")

    def test_errors_carry_the_status(self):
        _Recorder.reply[("GET", "/api/v1/wants/missing")] = (404, {"error": "Want not found"})
        with self.assertRaises(MyWantError) as cm:
            Client(self.url).want("missing")
        self.assertEqual(cm.exception.status, 404)

    def test_unreachable(self):
        with self.assertRaises(MyWantError):
            Client("http://127.0.0.1:1", timeout=0.5).wants()


class PluginTest(unittest.TestCase):
    def tearDown(self):
        plugin._leave()
        for k in ("MYWANT_WANT_ID", "MYWANT_WANT_STATE", "MYWANT_URL"):
            os.environ.pop(k, None)

    def test_the_call_s_want(self):
        out = io.StringIO()
        w = plugin._enter({"want_id": "w-1", "want_name": "帳簿", "state": {"hp": 4},
                           "params": {"max_hp": 10}, "server": {"url": "http://example:9", "token": "t"}}, out=out)
        self.assertIs(plugin.current(), w)
        self.assertEqual((w.state["hp"], w.params["max_hp"], w.server.url, w.server.token), (4, 10, "http://example:9", "t"))
        w.set("hp", 3)
        w.suspend("worn down")
        w.move_to(2, 5)
        w.finish()
        lines = [json.loads(l) for l in out.getvalue().splitlines()]
        self.assertEqual(lines, [
            {"_control": "suspend", "_reason": "worn down"},
            {"_tile": {"x": 2, "y": 5}},
            {"hp": 3},
        ])

    def test_from_the_environment(self):
        os.environ["MYWANT_WANT_ID"] = "w-2"
        os.environ["MYWANT_WANT_STATE"] = json.dumps({"hp": 7})
        os.environ["MYWANT_URL"] = "http://elsewhere:1"
        plugin._leave()
        w = plugin.current()
        self.assertEqual((w.id, w.state["hp"], w.server.url), ("w-2", 7, "http://elsewhere:1"))

    def test_one_want_per_thread(self):
        seen = {}

        def call(i):
            plugin._enter({"want_id": f"w-{i}"}, out=io.StringIO())
            seen[i] = plugin.current().id
            plugin._leave()

        ts = [threading.Thread(target=call, args=(i,)) for i in range(5)]
        for t in ts:
            t.start()
        for t in ts:
            t.join()
        self.assertEqual(seen, {i: f"w-{i}" for i in range(5)})


if __name__ == "__main__":
    unittest.main()
