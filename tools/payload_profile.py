#!/usr/bin/env python3
"""
payload_profile.py - where the size of a want, and of a want type, actually is.

Reads every want (GET /api/v1/wants/{id}) and every want type
(GET /api/v1/want-types/{name}) from a running MyWant server, walks each JSON
document, and charges every node's serialized size to its path. Array indices
are folded ("history.stateHistory[]"); map keys are kept, so a field inside
state.current or metadata.labels shows up as its own row.

Sizes are characters of compact JSON (no spaces) — what an API response costs
before compression, and roughly what it costs a language model to read. As a
rule of thumb a model reads JSON at about a third of a token per character.

Usage:
    python3 tools/payload_profile.py                      # local server
    python3 tools/payload_profile.py --server http://host:8080 --top 40
    python3 tools/payload_profile.py --json > profile.json

Only the standard library is used. See docs/payload-profile.md.
"""

import argparse
import base64
import json
import os
import sys
import urllib.parse
import urllib.request
from collections import Counter, defaultdict


def compact_size(v):
    return len(json.dumps(v, ensure_ascii=False, separators=(",", ":")))


class Client:
    def __init__(self, base, user=None, password=None):
        self.base = base.rstrip("/") + "/api/v1"
        self.auth = None
        if user:
            token = base64.b64encode(f"{user}:{password or ''}".encode()).decode()
            self.auth = "Basic " + token

    def get(self, path):
        req = urllib.request.Request(self.base + path)
        if self.auth:
            req.add_header("Authorization", self.auth)
        with urllib.request.urlopen(req) as resp:
            return json.load(resp)


def walk(v, path, out, depth, max_depth):
    out[path].append(compact_size(v))
    if depth >= max_depth:
        return
    if isinstance(v, dict):
        for k, child in v.items():
            walk(child, f"{path}.{k}" if path else k, out, depth + 1, max_depth)
    elif isinstance(v, list):
        for child in v:
            walk(child, path + "[]", out, depth + 1, max_depth)


def profile(docs, max_depth):
    """Rows of (path, total, max per document, occurrences), heaviest first."""
    agg = defaultdict(list)
    for d in docs:
        walk(d, "", agg, 0, max_depth)
    rows = [
        {"path": p, "total": sum(v), "max": max(v), "count": len(v)}
        for p, v in agg.items() if p
    ]
    rows.sort(key=lambda r: -r["total"])
    return rows


def summary(docs):
    sizes = [compact_size(d) for d in docs] or [0]
    return {"docs": len(docs), "total": sum(sizes),
            "avg": sum(sizes) // max(1, len(docs)), "max": max(sizes)}


def section(title, docs, max_depth, top):
    s = summary(docs)
    rows = profile(docs, max_depth)
    for r in rows:
        r["share"] = round(100 * r["total"] / s["total"], 1) if s["total"] else 0.0
    return {"title": title, **s, "rows": rows[:top]}


def print_section(sec):
    print(f"\n== {sec['title']}: {sec['docs']} docs, {sec['total']:,} chars total, "
          f"avg {sec['avg']:,}, max {sec['max']:,}")
    print(f"{'path':58} {'total':>10} {'share':>6} {'max/doc':>9} {'n':>5}")
    for r in sec["rows"]:
        print(f"{r['path'][:58]:58} {r['total']:>10,} {r['share']:>5.1f}% {r['max']:>9,} {r['count']:>5}")


def main():
    ap = argparse.ArgumentParser(description="Profile the size of wants and want types by JSON path.")
    ap.add_argument("--server", default=os.environ.get("MYWANT_SERVER_URL", "http://localhost:8080"),
                    help="MyWant server (default: $MYWANT_SERVER_URL or http://localhost:8080)")
    ap.add_argument("--user", default=os.environ.get("MYWANT_USER"), help="basic auth user, if the server asks")
    ap.add_argument("--password", default=os.environ.get("MYWANT_PASSWORD"), help="basic auth password")
    ap.add_argument("--depth", type=int, default=3, help="how deep paths are broken out (default 3)")
    ap.add_argument("--top", type=int, default=25, help="rows per section (default 25)")
    ap.add_argument("--heaviest", type=int, default=10, help="heaviest wants to list (default 10)")
    ap.add_argument("--show-names", action="store_true",
                    help="name the heaviest wants (by default only their types are shown)")
    ap.add_argument("--json", action="store_true", help="print the profile as JSON instead of tables")
    args = ap.parse_args()

    c = Client(args.server, args.user, args.password)

    wants = []
    for w in c.get("/wants").get("wants", []):
        wid = w.get("metadata", {}).get("id", "")
        try:
            wants.append(c.get("/wants/" + urllib.parse.quote(wid)))
        except Exception as e:  # a want deleted mid-run: profile what the list had
            print(f"[payload_profile] want {wid}: {e}", file=sys.stderr)
            wants.append(w)

    types = []
    for t in c.get("/want-types").get("wantTypes", []):
        try:
            types.append(c.get("/want-types/" + urllib.parse.quote(t["name"])))
        except Exception as e:
            print(f"[payload_profile] type {t.get('name')}: {e}", file=sys.stderr)

    state_defs = [s for t in types for s in (t.get("state") or [])]

    sections = [
        section("wants (GET /wants/{id})", wants, args.depth, args.top),
        section("wants: inside history", [w.get("history") or {} for w in wants], args.depth, 15),
        section("wants: inside state", [w.get("state") or {} for w in wants], 2, 15),
        section("want types (GET /want-types/{name})", types, 2, args.top),
        section("want types: inside one state definition (state[])", state_defs, 1, 15),
    ]

    # The heaviest wants, and the parts that make each heavy.
    heaviest = []
    for w in sorted(wants, key=compact_size, reverse=True)[:args.heaviest]:
        md = w.get("metadata", {})
        parts = sorted(((k, compact_size(v)) for k, v in w.items()), key=lambda kv: -kv[1])[:4]
        heaviest.append({
            "size": compact_size(w),
            "type": md.get("type", ""),
            **({"name": md.get("name", "")} if args.show_names else {}),
            "parts": dict(parts),
        })

    # State definitions every type carries: what the framework adds to each.
    names = Counter(s.get("name") for s in state_defs)
    common = sorted(n for n, k in names.items() if n and k >= 0.8 * max(1, len(types)))
    common_size = sum(compact_size(s) for s in state_defs if s.get("name") in common)
    all_size = sum(compact_size(s) for s in state_defs)
    shared = {"names": common, "chars": common_size, "of": all_size,
              "share": round(100 * common_size / all_size, 1) if all_size else 0.0}

    if args.json:
        json.dump({"server": args.server, "sections": sections, "heaviest": heaviest,
                   "shared_state_definitions": shared}, sys.stdout, ensure_ascii=False, indent=2)
        print()
        return

    for sec in sections:
        print_section(sec)
    print(f"\n== heaviest wants (top {len(heaviest)})")
    for h in heaviest:
        label = f"{h.get('name', '')[:36]:36} " if args.show_names else ""
        print(f"{h['size']:>8,}  {label}({h['type'][:28]})  "
              + ", ".join(f"{k}={v:,}" for k, v in h["parts"].items()))
    print(f"\n== state definitions in at least 80% of types: {', '.join(common) or '(none)'}")
    print(f"   {shared['chars']:,} of {shared['of']:,} chars of all state definitions ({shared['share']}%)")


if __name__ == "__main__":
    main()
