"""The MRS agent service: every plugin script, in one Python.

The engine used to start a python3 per plugin call, or keep one resident
interpreter per plugin (`serve: true`) — about 25MB each, most of it the
interpreter, which on a 256mb Machine was the difference between running and
thrashing. This is the engine's external-agent service (ExecuteRequest in,
ExecuteResponse out — engine/core/webhook.go) for those scripts: one process,
each script loaded once and run as a spawn would run it.

As a spawn would run it, because no plugin is changed for it:
  - sys.argv is [script, *args] — per thread, so two calls at once do not mix;
  - what the script prints is its output — per thread, read as the stream of
    JSON objects a spawn's stdout is: {"_progress": n, "_message": m} goes back
    at once, the last other object is the result;
  - stdin is empty, as the spawn's /dev/null;
  - sys.exit(n) ends the call, not the service, and n != 0 is a failure;
  - the script's own directory is on sys.path, for the modules beside it.
Each call runs the whole script, in fresh globals named __main__ — top level,
`if __name__ == "__main__":` and all, exactly as a spawn does: a plugin that
takes its start time at the top (nearest's STARTED) gets this call's. What is
kept is what a spawn pays for: the interpreter, and the modules the scripts
import (sys.modules), plus each script compiled once until it changes.

Started by the engine (mrs_shared.go), on 127.0.0.1 only. The first line on
stdout is {"port": n}; after that nothing is written there.

    POST /execute   ExecuteRequest {"operation": "mrs", "params": {"script", "args"}}
                    → application/x-ndjson: progress objects, then one
                      ExecuteResponse {"status", "state_updates" | "error"}
    GET  /health    → {"status": "ok"}
"""

import builtins
import http.server
import io
import json
import os
import sys
import threading
import traceback

_local = threading.local()
_real_stdout = sys.stdout
_real_stderr = sys.stderr


class _ThreadStream(io.TextIOBase):
    """sys.stdout / sys.stderr: the calling request's, else the service's stderr."""

    def __init__(self, attr):
        self._attr = attr

    def _target(self):
        return getattr(_local, self._attr, None) or _real_stderr

    def write(self, s):
        return self._target().write(s)

    def flush(self):
        self._target().flush()

    def writable(self):
        return True

    def isatty(self):
        return False

    @property
    def encoding(self):
        return "utf-8"


class _ThreadArgv(list):
    """sys.argv: the calling request's [script, *args]."""

    def _cur(self):
        return getattr(_local, "argv", None) or ["mrs_agent_service"]

    def __getitem__(self, i):
        return self._cur()[i]

    def __setitem__(self, i, v):  # a script may set argv[0] for itself
        self._cur()[i] = v

    def __delitem__(self, i):
        del self._cur()[i]

    def __len__(self):
        return len(self._cur())

    def __iter__(self):
        return iter(self._cur())

    def __contains__(self, x):
        return x in self._cur()

    def __repr__(self):
        return repr(self._cur())

    def __eq__(self, other):
        return self._cur() == other

    def index(self, *a):
        return self._cur().index(*a)

    def copy(self):
        return list(self._cur())


class _Output(io.TextIOBase):
    """One request's stdout, read the way the engine reads a spawn's: a stream of
    JSON objects, not necessarily one per line."""

    def __init__(self, send_progress):
        self._buf = ""
        self._decoder = json.JSONDecoder()
        self._send_progress = send_progress
        self.result = None

    def write(self, s):
        self._buf += s
        self._drain()
        return len(s)

    def writable(self):
        return True

    def flush(self):
        pass

    def _drain(self):
        while True:
            text = self._buf.lstrip()
            if not text:
                self._buf = ""
                return
            try:
                obj, end = self._decoder.raw_decode(text)
            except ValueError:
                # Half an object so far — or not JSON at all, which a spawn's
                # decoder would also stop at. Wait for more either way.
                self._buf = text
                return
            self._buf = text[end:]
            if isinstance(obj, dict) and "_progress" in obj:
                self._send_progress(obj)
            elif isinstance(obj, dict):
                self.result = obj


_compiled = {}  # script path → (mtime, code)
_compile_lock = threading.Lock()


def _code(path):
    mtime = os.stat(path).st_mtime
    with _compile_lock:
        hit = _compiled.get(path)
        if hit and hit[0] == mtime:
            return hit[1]
        with open(path, "rb") as f:
            code = compile(f.read(), path, "exec")
        here = os.path.dirname(path)
        if here not in sys.path:
            sys.path.insert(0, here)  # the modules beside it, as for a spawn
        _compiled[path] = (mtime, code)
        return code


def run(path, args, send_progress):
    """Run one script as a spawn would. Returns (exit code, result, stderr)."""
    out = _Output(send_progress)
    err = io.StringIO()
    _local.argv = [path] + [str(a) for a in args]
    _local.stdout = out
    _local.stderr = err
    code = 0
    try:
        # Not runpy: it swaps sys.modules["__main__"] and argv[0] for the
        # whole process, which calls running side by side would tangle.
        exec(_code(path), {"__name__": "__main__", "__file__": path, "__builtins__": builtins})
    except SystemExit as e:
        if e.code is None:
            code = 0
        elif isinstance(e.code, int):
            code = e.code
        else:
            err.write(str(e.code))
            code = 1
    except BaseException:  # one bad call must not take the service down
        traceback.print_exc(file=err)
        code = 1
    finally:
        _local.argv = _local.stdout = _local.stderr = None
    return code, out.result, err.getvalue()


class _Server(http.server.ThreadingHTTPServer):
    daemon_threads = True
    # The default backlog of 5 resets connections when wants tick together.
    request_queue_size = 128


class _Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.0"  # the body ends when the connection does

    def log_message(self, *a):
        pass

    def do_GET(self):
        if self.path != "/health":
            self.send_error(404)
            return
        self._reply({"status": "ok"})

    def do_POST(self):
        if self.path != "/execute":
            self.send_error(404)
            return
        try:
            req = json.loads(self.rfile.read(int(self.headers.get("Content-Length") or 0)))
            params = req.get("params") or {}
            script = params["script"]
            args = params.get("args") or []
        except (ValueError, KeyError, TypeError) as e:
            self.send_error(400, str(e))
            return

        self.send_response(200)
        self.send_header("Content-Type", "application/x-ndjson")
        self.end_headers()
        lock = threading.Lock()

        def send(obj):
            line = (json.dumps(obj, ensure_ascii=False) + "\n").encode()
            with lock:
                try:
                    self.wfile.write(line)
                    self.wfile.flush()
                except OSError:
                    pass  # the engine gave up on this call (its timeout)

        code, result, stderr = run(script, args, send)
        if code != 0:
            # The script's own error over the exit code, as for a spawn.
            message = (result or {}).get("error") if isinstance(result, dict) else None
            if not message:
                message = f"exit status {code}"
                if stderr.strip():
                    message += "\nstderr: " + stderr.strip()
            send({"status": "failed", "error": message})
        elif result is None:
            send({"status": "failed", "error": "skill produced no JSON output"})
        else:
            send({"status": "completed", "state_updates": result})

    def _reply(self, obj):
        body = json.dumps(obj).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main():
    server = _Server(("127.0.0.1", 0), _Handler)
    _real_stdout.write(json.dumps({"port": server.server_address[1]}) + "\n")
    _real_stdout.flush()
    sys.stdout = _ThreadStream("stdout")
    sys.stderr = _ThreadStream("stderr")
    sys.stdin = io.StringIO("")
    sys.argv = _ThreadArgv()
    server.serve_forever()


if __name__ == "__main__":
    main()
