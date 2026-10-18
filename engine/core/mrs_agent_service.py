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
import time
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


_modules_seen = {}  # module name → (file, mtime) when it was first seen loaded
_stdlib = tuple({os.path.realpath(p) + os.sep for p in (sys.prefix, sys.base_prefix, sys.exec_prefix)})


def _forget_changed_modules():
    """Drop the plugin modules whose file changed since they were imported, so
    the script imports them afresh — as a spawn would have. A custom updated in
    place (mrs_browser, which every browser plugin imports) used to stay the
    old one until the service restarted: browser_run() without the keyword
    the new plugins pass. The interpreter's own modules are never looked at."""
    with _compile_lock:
        for name, mod in list(sys.modules.items()):
            f = getattr(mod, "__file__", None)
            if not f or os.path.realpath(f).startswith(_stdlib):
                continue
            try:
                mtime = os.stat(f).st_mtime
            except OSError:
                mtime = None  # gone: a custom removed or moved
            seen = _modules_seen.get(name)
            if seen is None or seen[0] != f:
                _modules_seen[name] = (f, mtime)
            elif seen[1] != mtime:
                sys.modules.pop(name, None)
                _modules_seen.pop(name, None)


def run(path, args, send_progress):
    """Run one script as a spawn would. Returns (exit code, result, stderr)."""
    _forget_changed_modules()
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
        _forget_changed_modules()  # what it imported, as it is now
    return code, out.result, err.getvalue()


def _answer(ran, send):
    """The last line of a call: completed with its result, or failed and why."""
    code, result, stderr = ran
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
                    return True
                except OSError:
                    return False  # the engine gave up on this call (its timeout)

        if _runner is not None:
            _runner.execute(params, send, self.connection)
            return
        _answer(run(script, args, send), send)

    def _reply(self, obj):
        body = json.dumps(obj).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


# ─── the runner: each call alone, in a jail of its own ──────────────────────
#
# `mywant runner serve` starts this file with --runner, in a container of its
# own (see mywant-ideas/impl-sandbox.md). There a call is not run in this
# process: a zygote — forked before any thread exists, so a fork of it never
# inherits a lock some other thread held — forks one child per call, and the
# child shuts itself in before it reads the script:
#
#   - its own mount namespace: / read-only, $HOME a scratch tmpfs with only
#     what the profile may see bound back into it, /tmp a tmpfs of its own;
#   - for profile "voice", its own network namespace too: not even loopback;
#   - the uid that owns ~/.mywant (never root), no new privileges, a
#     scrubbed environment.
#
#   profile  sees                                         writes
#   main     custom-types, customs, the --expose paths    plugin-data/<plugin>
#   voice    the script's own directory                   nothing that is kept
#
# Forking a warm interpreter costs 1.5–3ms where starting python3 costs 33ms
# (fly, 2026-10-18). What a child imports dies with it; what the zygote imported
# first (_WARM) the children share.

_runner = None

_WARM = ("json", "re", "datetime", "pathlib", "urllib.request", "urllib.parse",
         "http.client", "ssl", "hashlib", "base64", "subprocess", "colorsys",
         "math", "random", "time", "traceback", "typing", "dataclasses")

CLONE_NEWNS = 0x00020000
CLONE_NEWNET = 0x40000000
MS_RDONLY, MS_NOSUID, MS_NODEV = 0x1, 0x2, 0x4
MS_REMOUNT, MS_BIND, MS_REC, MS_PRIVATE = 0x20, 0x1000, 0x4000, 0x40000
MNT_DETACH = 0x2
PR_SET_NO_NEW_PRIVS = 38  # PR_SET_PDEATHSIG is 1, see _die_with_parent


class _Libc:
    def __init__(self):
        import ctypes
        self._ctypes = ctypes
        self._libc = ctypes.CDLL(None, use_errno=True)

    def _check(self, rc, what):
        if rc != 0:
            err = self._ctypes.get_errno()
            raise OSError(err, f"{what}: {os.strerror(err)}")

    def unshare(self, flags):
        self._check(self._libc.unshare(flags), "unshare")

    def mount(self, src, target, fstype, flags, data=None):
        enc = lambda v: v.encode() if isinstance(v, str) else v
        self._check(self._libc.mount(enc(src), enc(target), enc(fstype), flags, enc(data)),
                    f"mount {target}")

    def umount(self, target, flags=0):
        self._check(self._libc.umount2(target.encode(), flags), f"umount {target}")

    def no_new_privs(self):
        self._check(self._libc.prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0), "prctl")


def _plugin_of(script, mywant):
    """The plugin a script belongs to: its directory's name under custom-types
    (or customs, skills…), or the directory's own name for anything else."""
    here = os.path.dirname(os.path.abspath(script))
    for root in ("custom-types", "customs", "skills"):
        base = os.path.join(mywant, root) + os.sep
        if here.startswith(base):
            return here[len(base):].split(os.sep)[0]
    return os.path.basename(here)


def _sees(profile, script, mywant, expose):
    """What a call may see, as [(path, writable)]. A path is kept as the engine
    names it; where it is a link, its target is bound and the link remade."""
    here = os.path.dirname(os.path.abspath(script))
    if profile == "voice":
        return [(here, False)]
    paths = [(here, False)]
    for root in ("custom-types", "customs"):
        top = os.path.join(mywant, root)
        if os.path.isdir(top):
            paths.append((top, False))
            for name in os.listdir(top):  # the targets of the links in it
                entry = os.path.join(top, name)
                if os.path.islink(entry) and os.path.isdir(entry):
                    paths.append((os.path.realpath(entry), False))
    paths += [(p, False) for p in expose if os.path.isdir(p)]
    return paths


class _Runner:
    def __init__(self, jail, expose, api_url):
        self.jail = jail
        self.expose = [os.path.abspath(os.path.expanduser(p)) for p in expose]
        self.api_url = api_url
        self.home = os.environ.get("HOME") or "/root"
        self.mywant = os.path.join(self.home, ".mywant")
        self._lock = threading.Lock()
        self._sock = None

    # ── the zygote ──

    def start_zygote(self):
        import socket
        import signal
        for name in _WARM:
            try:
                __import__(name)
            except ImportError:
                pass
        ours, theirs = socket.socketpair(socket.AF_UNIX, socket.SOCK_STREAM)
        if os.fork() == 0:
            ours.close()
            _die_with_parent()
            signal.signal(signal.SIGCHLD, signal.SIG_IGN)  # children reap themselves
            self._zygote(theirs)
            os._exit(0)
        theirs.close()
        self._sock = ours

    def _zygote(self, sock):
        import signal
        import socket
        while True:
            try:
                head, fds, _, _ = socket.recv_fds(sock, 8, 1)
                if not head:
                    return  # the server went away
                head += _recv_exact(sock, 8 - len(head))
                msg = _recv_exact(sock, int.from_bytes(head, "big"))
            except OSError:
                return
            pid = os.fork()
            if pid == 0:
                sock.close()
                signal.signal(signal.SIGCHLD, signal.SIG_DFL)  # its own subprocesses
                _die_with_parent()
                self._child(json.loads(msg), fds[0])
                os._exit(0)
            os.close(fds[0])
            sock.sendall(pid.to_bytes(8, "big"))

    # ── the server's side of a call ──

    def execute(self, params, send, conn):
        """Hand a call to the zygote and pass on what its child writes. The
        child is killed when its time is up or the engine hangs up — what it
        would still do, nobody is waiting for."""
        import select
        import signal
        import socket
        rfd, wfd = os.pipe()
        try:
            msg = json.dumps(params).encode()
            with self._lock:
                socket.send_fds(self._sock, [len(msg).to_bytes(8, "big")], [wfd])
                self._sock.sendall(msg)
                pid = int.from_bytes(_recv_exact(self._sock, 8), "big")
        finally:
            os.close(wfd)
        deadline = time.monotonic() + float(params.get("timeout") or 120) + 2
        buf, answered = b"", False
        with os.fdopen(rfd, "rb", buffering=0) as r:
            while True:
                left = deadline - time.monotonic()
                ready = select.select([r, conn], [], [], max(left, 0))[0] if left > 0 else []
                if not ready:
                    _kill(pid, signal.SIGKILL)
                    send({"status": "failed", "error": "skill timed out in the runner"})
                    return
                if conn in ready and _hung_up(conn):
                    _kill(pid, signal.SIGKILL)
                    return
                if r not in ready:
                    continue
                chunk = r.read(65536)
                if not chunk:
                    break
                buf += chunk
                while b"\n" in buf:
                    line, buf = buf.split(b"\n", 1)
                    if not line.strip():
                        continue
                    obj = json.loads(line)
                    answered = answered or "status" in obj
                    if not send(obj):
                        _kill(pid, signal.SIGKILL)
                        return
        if not answered:
            send({"status": "failed", "error": "skill ended without answering (killed by a limit?)"})

    # ── the child ──

    def _child(self, params, wfd):
        out = os.fdopen(wfd, "w", buffering=1, encoding="utf-8")
        # The zygote was forked before main() redirected these; a call's
        # prints go where run() points them, as on the shared service.
        sys.stdout = _ThreadStream("stdout")
        sys.stderr = _ThreadStream("stderr")
        sys.stdin = io.StringIO("")
        sys.argv = _ThreadArgv()

        def send(obj):
            try:
                out.write(json.dumps(obj, ensure_ascii=False) + "\n")
            except BrokenPipeError:
                os._exit(1)  # the call was given up on

        try:
            script = params["script"]
            args = params.get("args") or []
            env = self._shut_in(params)
            os.environ.clear()
            os.environ.update(env)
        except BaseException as e:  # never run what could not be shut in
            send({"status": "failed", "error": f"sandbox: {e}"})
            out.close()
            return
        _answer(run(script, args, send), send)
        out.close()

    def _shut_in(self, params):
        import resource
        profile = params.get("profile") or "main"
        if profile not in ("main", "voice"):
            raise ValueError(f"unknown profile {profile!r}")
        script = os.path.abspath(params["script"])
        uid, gid = self.run_as()
        timeout = int(float(params.get("timeout") or 120)) + 1

        data = None
        if profile == "main":
            data = os.path.join(self.mywant, "plugin-data", _plugin_of(script, self.mywant))
            for d in (os.path.dirname(data), data):
                if not os.path.isdir(d):
                    os.makedirs(d, exist_ok=True)
                    os.chown(d, uid, gid)

        if self.jail:
            sees = _sees(profile, script, self.mywant, self.expose)
            if data:
                sees.append((data, True))
            self._mount(profile, sees)

        resource.setrlimit(resource.RLIMIT_CPU, (timeout, timeout))
        resource.setrlimit(resource.RLIMIT_FSIZE, (64 << 20, 64 << 20))
        if self.jail:
            os.setgroups([])
            os.setgid(gid)
            os.setuid(uid)
            _Libc().no_new_privs()
        os.chdir(os.path.dirname(script))

        env = {
            "PATH": os.environ.get("PATH", "/usr/local/bin:/usr/bin:/bin"),
            "HOME": self.home,
            "LANG": os.environ.get("LANG", "C.UTF-8"),
            "TZ": os.environ.get("TZ", ""),
            "PYTHONDONTWRITEBYTECODE": "1",
        }
        if profile == "main":
            env.update({str(k): str(v) for k, v in (params.get("env") or {}).items()})
            env["MYWANT_PLUGIN_DATA"] = data
            if self.api_url:
                env["MYWANT_URL"] = env["MYWANT_SERVER"] = self.api_url
        return {k: v for k, v in env.items() if v != ""}

    def run_as(self):
        """Who a call runs as: whoever owns ~/.mywant — never root, which
        inside its own mount namespace could take the jail apart. Docker
        Desktop shows a Mac's files as root's; then nobody."""
        owner = os.stat(self.mywant)
        if owner.st_uid == 0:
            return 65534, 65534
        return owner.st_uid, owner.st_gid

    def _mount(self, profile, sees):
        import tempfile
        libc = _Libc()
        libc.unshare(CLONE_NEWNS | (CLONE_NEWNET if profile == "voice" else 0))
        libc.mount(None, "/", None, MS_REC | MS_PRIVATE)

        # $HOME is covered with a scratch tmpfs; what is bound back into it is
        # taken from a view of the real one kept aside first.
        stage = tempfile.mkdtemp(prefix="mywant-jail-")
        libc.mount(self.home, stage, None, MS_BIND | MS_REC)
        uid, gid = self.run_as()
        libc.mount("tmpfs", self.home, "tmpfs", MS_NOSUID | MS_NODEV,
                   f"size=16m,mode=0755,uid={uid},gid={gid}")
        inside = self.home.rstrip(os.sep) + os.sep
        links = []
        for path, writable in sorted(set(sees), key=lambda p: len(p[0])):
            real = os.path.realpath(path)
            if real != path:
                links.append((real, path))
            if not real.startswith(inside):
                continue  # outside $HOME: already visible, and read-only below
            src = os.path.join(stage, real[len(inside):])
            if not os.path.isdir(src):
                continue
            os.makedirs(real, exist_ok=True)
            libc.mount(src, real, None, MS_BIND | MS_REC)
            if not writable:
                libc.mount(None, real, None, MS_BIND | MS_REMOUNT | MS_RDONLY | MS_NOSUID | MS_NODEV)
        for real, path in links:
            if not os.path.lexists(path):
                os.makedirs(os.path.dirname(path), exist_ok=True)
                os.symlink(real, path)
        libc.umount(stage, MNT_DETACH)
        os.rmdir(stage)
        libc.mount("tmpfs", "/tmp", "tmpfs", MS_NOSUID | MS_NODEV, "size=16m,mode=1777")
        libc.mount(None, "/", None, MS_BIND | MS_REMOUNT | MS_RDONLY)


def _hung_up(conn):
    """The engine closed its end: nothing more is coming, nobody is reading."""
    import socket
    try:
        return conn.recv(1, socket.MSG_PEEK) == b""
    except BlockingIOError:
        return False
    except OSError:
        return True


def _die_with_parent():
    """On Linux, go when the process that forked us goes: a runner restarted
    leaves no calls behind it."""
    if sys.platform.startswith("linux"):
        import ctypes
        import signal
        ctypes.CDLL(None).prctl(1, signal.SIGKILL, 0, 0, 0)  # PR_SET_PDEATHSIG


def _recv_exact(sock, n):
    buf = b""
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise OSError("the other end went away")
        buf += chunk
    return buf


def _kill(pid, sig):
    try:
        os.kill(pid, sig)
    except ProcessLookupError:
        pass


def main():
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--runner", action="store_true", help="each call forked and shut in")
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=0)
    ap.add_argument("--no-jail", action="store_true", help="runner without the jail (tests on a Mac)")
    ap.add_argument("--expose", action="append", default=[], help="a directory main calls may also read")
    ap.add_argument("--api-url", default="", help="MYWANT_URL as seen from inside the runner")
    opts = ap.parse_args()

    global _runner
    if opts.runner:
        _runner = _Runner(jail=not opts.no_jail, expose=opts.expose, api_url=opts.api_url)
        _runner.start_zygote()  # before the server's first thread
    server = _Server((opts.host, opts.port), _Handler)
    _real_stdout.write(json.dumps({"port": server.server_address[1]}) + "\n")
    _real_stdout.flush()
    sys.stdout = _ThreadStream("stdout")
    sys.stderr = _ThreadStream("stderr")
    sys.stdin = io.StringIO("")
    sys.argv = _ThreadArgv()
    server.serve_forever()


if __name__ == "__main__":
    main()
