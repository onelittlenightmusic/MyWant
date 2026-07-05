#!/usr/bin/env python3
"""
Claude Code Skill → OpenTelemetry Hook

PreToolUse  : タイムスタンプを一時ファイルに記録
PostToolUse : 実行時間・成功/失敗を計算し skill_activated ログを OTLP へ送信

settings.json 設定例:
  "hooks": {
    "PreToolUse":  [{"matcher": "Skill", "hooks": [{"type": "command", "command": "python3 /path/to/skill_hook.py pre"}]}],
    "PostToolUse": [{"matcher": "Skill", "hooks": [{"type": "command", "command": "python3 /path/to/skill_hook.py post"}]}]
  }

依存:
  pip3 install opentelemetry-sdk opentelemetry-exporter-otlp-proto-grpc
"""

import hashlib
import json
import os
import sys
import tempfile
import time


# ---------------------------------------------------------------------------
# 相関キー (pre/post を紐付け)
# ---------------------------------------------------------------------------

def _corr_key(session_id: str, tool_input: dict) -> str:
    raw = f"{session_id}|{json.dumps(tool_input, sort_keys=True)}"
    return hashlib.md5(raw.encode()).hexdigest()[:16]


def _tmp_path(key: str) -> str:
    return os.path.join(tempfile.gettempdir(), f"ccskill_{key}.json")


# ---------------------------------------------------------------------------
# OTLP ログ送信
# ---------------------------------------------------------------------------

def _emit(skill_name: str, skill_args: str, duration_ms, success: bool, trigger: str):
    endpoint = os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
    try:
        from opentelemetry._logs import LogRecord
        from opentelemetry._logs.severity import SeverityNumber
        from opentelemetry.exporter.otlp.proto.grpc._log_exporter import OTLPLogExporter
        from opentelemetry.sdk._logs import LoggerProvider
        from opentelemetry.sdk._logs.export import SimpleLogRecordProcessor
        from opentelemetry.sdk.resources import Resource

        resource = Resource.create({"service.name": "claude-code"})
        exporter = OTLPLogExporter(endpoint=endpoint, insecure=True, timeout=2)
        provider = LoggerProvider(resource=resource)
        provider.add_log_record_processor(SimpleLogRecordProcessor(exporter))
        logger = provider.get_logger("claude-code.skill-hook")

        now_ns = int(time.time() * 1e9)
        attrs: dict = {
            "event_name": "skill_activated",
            "skill_name": skill_name,
            "skill_args": skill_args[:200],
            "success": str(success).lower(),
            "invocation_trigger": trigger,
        }
        if duration_ms is not None:
            attrs["duration_ms"] = int(duration_ms)

        record = LogRecord(
            timestamp=now_ns,
            observed_timestamp=now_ns,
            severity_text="INFO",
            severity_number=SeverityNumber.INFO,
            body=f"skill_activated: {skill_name}",
            attributes=attrs,
        )
        logger.emit(record)
        provider.force_flush(timeout_millis=3000)
        provider.shutdown()
    except Exception:
        pass  # otelcol が停止中でも Claude Code をブロックしない


# ---------------------------------------------------------------------------
# メイン
# ---------------------------------------------------------------------------

def main():
    phase = sys.argv[1] if len(sys.argv) > 1 else "post"

    try:
        data = json.load(sys.stdin)
    except Exception:
        sys.exit(0)

    if data.get("tool_name") != "Skill":
        sys.exit(0)

    tool_input = data.get("tool_input", {})
    session_id = data.get("session_id", "global")
    skill_name = tool_input.get("skill", "unknown")
    skill_args = str(tool_input.get("args", ""))

    key = _corr_key(session_id, tool_input)
    tp = _tmp_path(key)

    if phase == "pre":
        try:
            with open(tp, "w") as f:
                json.dump({"t": time.time() * 1000}, f)
        except Exception:
            pass
        sys.exit(0)

    # ----- post -----
    end_ms = time.time() * 1000
    duration_ms = None
    if os.path.exists(tp):
        try:
            pre = json.loads(open(tp).read())
            duration_ms = end_ms - pre["t"]
            os.unlink(tp)
        except Exception:
            pass

    tool_resp = data.get("tool_response", {})
    success = not bool(tool_resp.get("is_error", False))

    _emit(skill_name, skill_args, duration_ms, success, "claude-proactive")
    sys.exit(0)


if __name__ == "__main__":
    main()
