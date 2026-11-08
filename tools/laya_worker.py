#!/usr/bin/env python3
"""A Laya worker: takes the robot's typed decisions off a MyWant server's queue.

The server queues model work (engine/server/model_jobs.go) and whoever polls
saying it can do it takes it — the iPhone app, the Mac app (their LayaDecider,
on Core AI), or this, on a machine with a GPU, beside the server or anywhere
that reaches it. Several take work side by side; the server needs no model of
its own (Fly):

    GET  /api/v1/web-wants/pending-action?only=model&can=decide,train-decider&device=<name>
    POST /api/v1/web-wants/model-result   {"request_id", "output" | "error"}

    decide         {"items": [{"state", "questions"}]}  ->  {"items": [{"answers"}], "worker"}
    train-decider  {"rows": [{"state", "gold"}], "questions"}  ->  what it came to

It decides as the apps do, so it does not matter which worker takes a job:
the same Laya (convaiinnovations/laya multilingual at the revision the Core AI
bundle was made from), the same sequence (window 256), the same calibrated
temperatures (the bundle's metadata.json), and the same head over the same
features — pooled_cls and Laya's own probabilities, softmax regression fitted
the same way (mywant-ios Common/LayaHead.swift), kept in the same JSON.

Nothing is kept here but the model. The heads come with each decision, from
the server, and a training hands the heads it fitted back to the server,
which sends them with every decision after — one worker's training serves
all of them. Each worker takes the next job as soon as it is free (a
training runs in a thread, so decisions are still taken meanwhile).

    python3.13 -m venv ~/.mywant/laya-torch && ~/.mywant/laya-torch/bin/pip install laya numpy
    LAYA_SERVER=https://x.fly.dev LAYA_AUTH=mywant:$MYWANT_AUTH_PASSWORD \\
        ~/.mywant/laya-torch/bin/python -I tools/laya_worker.py

Env: LAYA_SERVER (http://localhost:8080), LAYA_AUTH (user:password, Basic),
LAYA_WORKER (this worker's name), LAYA_HOME (~/.mywant/laya: the bundle's
metadata.json, for its temperatures).
"""

import base64
import json
import os
import socket
import sys
import threading
import time
import urllib.parse
import urllib.request

import numpy as np

SERVER = os.environ.get("LAYA_SERVER", "http://localhost:8080").rstrip("/")
AUTH = os.environ.get("LAYA_AUTH", "")
NAME = os.environ.get("LAYA_WORKER") or "laya-" + socket.gethostname()
HOME = os.path.expanduser(os.environ.get("LAYA_HOME", "~/.mywant/laya"))

# What the Core AI bundle was made from, and its calibration.
CHECKPOINT = ("convaiinnovations/laya", "multilingual", "1c5edc17a7acd8701df6fc341c0d179f1c62c982")
METADATA_URL = "https://huggingface.co/mlboydaisuke/Laya-Multilingual-CoreAI/resolve/main/macos/wfp16-s256/metadata.json"
WINDOW = HEAD_MAX = 256


def log(*args):
    sys.stderr.write("[laya-worker] " + " ".join(str(a) for a in args) + "\n")


# ── Laya, read as the apps read it ───────────────────────────────────────────

class Laya:
    def __init__(self):
        import torch
        import laya
        from laya import common

        self.torch, self.common = torch, common
        repo, sub, rev = CHECKPOINT
        self.agent = laya.load(repo, subfolder=sub, revision=rev)
        self.model, self.tok = self.agent.model, self.agent.tok
        self.device = next(self.model.parameters()).device
        self.lock = threading.Lock()
        self._pooled = None
        # pooled_cls is the act head's input, before the four features.
        self.model.act_head.register_forward_pre_hook(lambda _m, args: setattr(self, "_pooled", args[0]))
        meta = self._metadata()["decision"]
        self.t_by_options = meta.get("temperature_by_options", {})
        self.t_by_type = meta.get("temperature", [1.0, 1.0, 1.0])

    @staticmethod
    def _metadata():
        path = os.path.join(HOME, "metadata.json")
        if not os.path.exists(path):
            os.makedirs(HOME, exist_ok=True)
            urllib.request.urlretrieve(METADATA_URL, path)
        return json.load(open(path, encoding="utf-8"))

    def temperature(self, t, k):
        """The bucket LayaModel.temperature picks: by type and option count, then by type."""
        if t == "choice":
            b = "choice:2" if k <= 2 else "choice:3-5" if k <= 5 else "choice:6-10" if k <= 10 else "choice:11+"
        elif t == "score":
            b = "score:3-5" if 3 <= k <= 5 else "score:%d" % k
        else:
            b = "noul:%d" % k
        v = self.t_by_options.get(b)
        if v:
            return v
        i = 1 if t == "score" else 2 if t == "noul" else 0
        return self.t_by_type[i] if i < len(self.t_by_type) and self.t_by_type[i] > 0 else 1.0

    @staticmethod
    def labels(q):
        if q["type"] == "noul":
            return ["false", "true"]
        crit = q.get("criteria") or {}
        if isinstance(crit, list):
            return list(crit) if q["type"] == "choice" else [str(i) for i in range(len(crit))]
        return list(crit.keys())

    def read(self, state, q):
        """(labels, probabilities, pooled) — LayaModel.read."""
        torch = self.torch
        internal = self.agent._to_internal(q)  # laya's own reading of the question
        ids, markers = self.common.build_sequence(self.tok, state, internal, max_len=WINDOW, head_max_len=HEAD_MAX)
        qtype = {"choice": 0, "score": 1, "noul": 2}.get(q["type"], 0)
        with self.lock, torch.no_grad():
            logits, _ = self.model(
                torch.tensor([ids], device=self.device),
                torch.ones(1, len(ids), dtype=torch.long, device=self.device),
                torch.tensor([markers], device=self.device),
                torch.ones(1, len(markers), dtype=torch.bool, device=self.device),
                torch.tensor([qtype], device=self.device),
            )
            pooled = self._pooled[0, : self.model.encoder.config.hidden_size].float().cpu().numpy()
        raw = logits[0, : len(markers)].float().cpu().numpy().astype(np.float64)
        z = raw / self.temperature(q["type"], len(raw))
        p = np.exp(z - z.max())
        p /= p.sum()
        return self.labels(q)[: len(raw)], p, pooled.astype(np.float32)


# ── The head, as LayaHead.swift fits it ──────────────────────────────────────

def softmax(x):
    e = np.exp(x - x.max(axis=-1, keepdims=True))
    return e / e.sum(axis=-1, keepdims=True)


class Head:
    def __init__(self, d):
        self.d = d

    @staticmethod
    def features(probs, pooled):
        return np.concatenate([pooled.astype(np.float32), probs.astype(np.float32)])

    def probabilities(self, probs, pooled):
        x = self.features(probs, pooled)
        if x.shape[0] != len(self.d["mean"]):
            return probs
        z = (x - np.array(self.d["mean"], np.float32)) * np.array(self.d["scale"], np.float32)
        logits = np.array(self.d["weights"], np.float32) @ z + np.array(self.d["bias"], np.float32)
        return softmax(logits.astype(np.float64))

    @property
    def serving(self):
        return self.d["heldOutAccuracy"] > self.d["layaHeldOutAccuracy"]

    @staticmethod
    def _train(xs, ys, k):
        xs = np.asarray(xs, np.float32)
        n, d = xs.shape
        mean = xs.mean(0)
        var = ((xs - mean) ** 2).mean(0)
        scale = np.where(var > 1e-8, 1 / np.sqrt(np.maximum(var, 1e-30)), 1).astype(np.float32)
        z = (xs - mean) * scale
        y = np.zeros((n, k), np.float32)
        y[np.arange(n), ys] = 1
        w = np.zeros((k, d), np.float32)
        b = np.zeros(k, np.float32)
        mw, vw, mb, vb = np.zeros_like(w), np.zeros_like(w), np.zeros_like(b), np.zeros_like(b)
        lr, l2, b1, b2 = 0.05, 1e-2, 0.9, 0.999
        for t in range(1, 301):
            p = softmax(z @ w.T + b).astype(np.float32)
            g = (p - y) / n
            gw = g.T @ z + l2 * w
            gb = g.sum(0)
            mw = b1 * mw + (1 - b1) * gw
            vw = b2 * vw + (1 - b2) * gw * gw
            w -= lr * (mw / (1 - b1 ** t)) / (np.sqrt(vw / (1 - b2 ** t)) + 1e-8)
            mb = b1 * mb + (1 - b1) * gb
            vb = b2 * vb + (1 - b2) * gb * gb
            b -= lr * (mb / (1 - b1 ** t)) / (np.sqrt(vb / (1 - b2 ** t)) + 1e-8)
        return mean, scale, w, b

    @staticmethod
    def fold(state):
        """LayaHead.fold: FNV-1a over UTF-8, mod 5 — copies of a row fall together."""
        h = 2166136261
        for byte in state.encode("utf-8"):
            h = ((h ^ byte) * 16777619) & 0xFFFFFFFF
        return h % 5

    @staticmethod
    def balanced(picks):
        """Each label's accuracy, averaged (LayaHead.balanced)."""
        hit, every = {}, {}
        for picked, y in picks:
            every[y] = every.get(y, 0) + 1
            hit[y] = hit.get(y, 0) + int(picked == y)
        return sum(hit[y] / every[y] for y in every) / len(every) if every else 0.0

    @classmethod
    def fit(cls, readings, gold, labels, states):
        """As LayaHead.fit: served if it beats Laya in five-fold cross-validation
        (folds by the row's words), then fitted on all of the rows."""
        rows = [(r, labels.index(g), cls.fold(st)) for r, g, st in zip(readings, gold, states) if g in labels]
        if len(rows) < 10 or len({y for _, y, _ in rows}) < 2:
            return None
        k = len(labels)

        def head_of(rs):
            mean, scale, w, b = cls._train([cls.features(p, po) for (p, po), _ in rs], [y for _, y in rs], k)
            return Head({"labels": labels, "mean": mean.tolist(), "scale": scale.tolist(),
                         "weights": w.tolist(), "bias": b.tolist(), "heldOutAccuracy": 0, "layaHeldOutAccuracy": 0})

        head_picks, laya_picks = [], []
        for f in range(5):
            test = [(r, y) for r, y, fo in rows if fo == f]
            fit_rows = [(r, y) for r, y, fo in rows if fo != f]
            if not test or len({y for _, y in fit_rows}) < 2:
                continue
            trial = head_of(fit_rows)
            for r, y in test:
                head_picks.append((int(np.argmax(trial.probabilities(*r))), y))
                laya_picks.append((int(np.argmax(r[0])), y))
        final = head_of([(r, y) for r, y, _ in rows])
        final.d["heldOutAccuracy"], final.d["layaHeldOutAccuracy"] = cls.balanced(head_picks), cls.balanced(laya_picks)
        return final


def head_key(qid, q, labels):
    """LayaDecider.headKey: the question as it is worded."""
    return "%s|%s|%s|%s" % (qid, q["type"], q.get("instructions", ""), ",".join(labels))


# ── The worker ───────────────────────────────────────────────────────────────

class Worker:
    """Keeps nothing but the model: the heads come with each decision, and a
    training hands its heads back for the server to keep and send on."""

    def __init__(self):
        self.laya = Laya()
        self.training = threading.Lock()

    def decide_one(self, state, questions, heads):
        answers = {}
        for qid, q in sorted(questions.items()):
            labels, probs, pooled = self.laya.read(state, q)
            d = heads.get(head_key(qid, q, labels))
            head = Head(d) if d else None
            if head and head.serving and head.d["labels"] == labels:
                probs = head.probabilities(probs, pooled)
            answers[qid] = {"choice": labels[int(np.argmax(probs))],
                            "probabilities": {l: float(p) for l, p in zip(labels, probs)}}
        return {"answers": answers}

    def decide(self, inp):
        heads = inp.get("heads") or {}
        items = inp.get("items") or [{"state": inp.get("state", ""), "questions": inp.get("questions", {})}]
        return {"items": [self.decide_one(it["state"], it["questions"], heads) for it in items], "worker": NAME}

    def train(self, inp):
        started = time.time()
        rows, questions = inp.get("rows") or [], inp.get("questions") or {}
        fitted, serving, held = {}, [], {}
        for qid, q in sorted(questions.items()):
            labelled = [r for r in rows if qid in (r.get("gold") or {})]
            readings, gold, labels = [], [], None
            states = [r["state"] for r in labelled]
            for r in labelled:
                labels, probs, pooled = self.laya.read(r["state"], q)
                readings.append((probs, pooled))
                gold.append(r["gold"][qid])
            if not readings:
                continue
            head = Head.fit(readings, gold, labels, states)
            if head is None:
                continue
            fitted[head_key(qid, q, labels)] = head.d
            held[qid] = {"head": head.d["heldOutAccuracy"], "laya": head.d["layaHeldOutAccuracy"], "rows": len(labelled)}
            if head.serving:
                serving.append(qid)
        return {"rows": len(rows), "fit_seconds": round(time.time() - started), "serving": serving,
                "held_out": held, "worker": NAME, "heads": fitted}


def request(method, path, body=None, query=None, timeout=30):
    url = SERVER + path + ("?" + urllib.parse.urlencode(query) if query else "")
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers={"Content-Type": "application/json"})
    if AUTH:
        req.add_header("Authorization", "Basic " + base64.b64encode(AUTH.encode()).decode())
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.load(r)


def post_result(request_id, output=None, error=None):
    body = {"request_id": request_id}
    if error is not None:
        body["error"] = error
    else:
        body["output"] = output
    try:
        request("POST", "/api/v1/web-wants/model-result", body, timeout=30)
    except Exception as e:
        log("could not post a result:", e)


def main():
    started = time.time()
    worker = Worker()
    log("laya %s@%s loaded in %.1fs on %s; taking work from %s as %s"
        % (CHECKPOINT[1], CHECKPOINT[2][:8], time.time() - started, worker.laya.device, SERVER, NAME))
    query = {"only": "model", "can": "decide,train-decider", "device": NAME}
    while True:
        try:
            pending = request("GET", "/api/v1/web-wants/pending-action", query=query, timeout=15)
        except Exception as e:
            log("server unreachable:", e)
            time.sleep(5)
            continue
        claim = pending.get("model") if pending.get("kind") == "model" else None
        if not claim:
            time.sleep(1)
            continue
        op, rid, inp = claim.get("op"), claim.get("request_id"), claim.get("input") or {}
        if op == "decide":
            try:
                post_result(rid, output=worker.decide(inp))
            except Exception as e:
                post_result(rid, error="laya: %s" % e)
        elif op == "train-decider":
            if not worker.training.acquire(blocking=False):
                post_result(rid, error="a training is under way on " + NAME)
                continue

            def run(rid=rid, inp=inp):
                try:
                    report = worker.train(inp)
                    log("trained:", json.dumps({k: v for k, v in report.items() if k != "heads"}, ensure_ascii=False))
                    post_result(rid, output=report)
                except Exception as e:
                    post_result(rid, error="laya: %s" % e)
                finally:
                    worker.training.release()

            threading.Thread(target=run, daemon=True).start()
        else:
            post_result(rid, error="%s cannot do %s" % (NAME, op))


if __name__ == "__main__":
    main()
