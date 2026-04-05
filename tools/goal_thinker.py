#!/usr/bin/env python3
"""
goal_thinker.py - Goal decomposition and replanning tool for MyWant.

Reads JSON from stdin, calls LLM via Claude CLI (or falls back to a static response),
and writes the result as JSON to stdout.

Input format includes system capabilities discovered via MCP.
"""

import sys
import json
import os
import subprocess
import shutil


def fallback_breakdown(goal_text: str, modification_request: str = "") -> dict:
    """Return a fixed breakdown when LLM is not available."""
    name_base = goal_text[:30].lower().replace(" ", "-").replace("、", "-").replace("。", "")
    import re
    name_base = re.sub(r"[^a-z0-9\-]", "", name_base)
    name_base = name_base.strip("-") or "goal-item"

    description = goal_text
    if modification_request:
        description = f"{goal_text} (modified: {modification_request})"

    return {
        "breakdown": [
            {
                "name": f"{name_base}-knowledge",
                "type": "knowledge",
                "description": f"Research and gather information about: {description}",
                "params": {
                    "topic": description,
                    "output_path": f"knowledge/{name_base}.md",
                },
            },
            {
                "name": f"{name_base}-reminder",
                "type": "reminder",
                "description": f"Reminder to review progress on: {description}",
                "params": {
                    "message": f"Review progress on: {description}",
                    "duration_from_now": "1 hours",
                    "require_reaction": False,
                },
            },
        ],
        "response_text": (
            f"I've broken down your goal into {2} sub-tasks. "
            f"Note: This is a fallback response (LLM not available)."
        ),
    }


def format_capabilities(available_capabilities: list) -> str:
    """Format the available capabilities list for the prompt."""
    if not available_capabilities:
        return "No special capabilities available. Use knowledge and reminder want types."
    lines = ["Unlocked capabilities (can influence which want types to choose):"]
    for cap in available_capabilities:
        lines.append(f"  - {cap}")
    return "\n".join(lines)


def call_claude_cli(prompt: str) -> str:
    """Call Claude CLI in print mode and return the response text."""
    claude_path = shutil.which("claude")
    if not claude_path:
        raise RuntimeError("claude CLI not found in PATH")

    result = subprocess.run(
        [claude_path, "-p", "--output-format", "text"],
        input=prompt,
        capture_output=True,
        text=True,
        timeout=120,
    )
    if result.returncode != 0:
        raise RuntimeError(f"claude CLI failed (rc={result.returncode}): {result.stderr}")
    return result.stdout.strip()


def parse_json_response(content: str) -> dict:
    """Extract JSON from LLM response, handling markdown code fences."""
    if "```json" in content:
        content = content.split("```json")[1].split("```")[0].strip()
    elif "```" in content:
        content = content.split("```")[1].split("```")[0].strip()
    return json.loads(content)


def decompose_with_llm(goal_text: str, available_capabilities: list) -> dict:
    """Use Claude CLI to decompose a goal into sub-wants."""
    cap_text = format_capabilities(available_capabilities)

    prompt = f"""You are a goal decomposition assistant for a productivity system called MyWant.
Your task is to break down a high-level goal into actionable sub-wants.

AVAILABLE CAPABILITIES:
{cap_text}

AVAILABLE WANT TYPES: knowledge, reminder (use specific types when capabilities match)

IMPORTANT: Do NOT use type "goal". Break down the goal into concrete, actionable items using only knowledge and reminder types.

User goal: {goal_text}

Each sub-want must have:
- name: a kebab-case identifier (e.g., "research-destinations", "morning-stretch-reminder")
- type: MUST be one of: knowledge, reminder (NEVER use "goal")
- description: clear description of what this achieves
- params: object with type-specific parameters.
  - For "reminder" type: params MUST include "message" (string) and "duration_from_now" (e.g., "30 seconds", "1 hours").
  - For "knowledge" type: params MUST include "topic" (string) and "output_path" (string, e.g., "knowledge/topic-name.md").

STRATEGY:
1. Prefer specific want types if they match the goal (e.g., "flight" for travel).
2. If a specific type is not yet available but seems needed, use "knowledge" to research it.
3. Break big goals into multiple small, concrete knowledge/reminder items instead of nesting sub-goals.

Respond ONLY with a valid JSON object:
{{
  "breakdown": [
    {{
      "name": "kebab-case-name",
      "type": "want-type",
      "description": "what this achieves",
      "params": {{}}
    }}
  ],
  "response_text": "Brief explanation of the breakdown"
}}"""

    content = call_claude_cli(prompt)
    return parse_json_response(content)


def replan_with_llm(
    goal_text: str,
    conversation_history: list,
    cc_responses: list,
    modification_request: str,
    available_capabilities: list,
) -> dict:
    """Use Claude CLI to replan based on conversation history."""
    cap_text = format_capabilities(available_capabilities)
    history_text = ""
    if conversation_history:
        history_items = []
        for i, msg in enumerate(conversation_history[-10:]):
            if isinstance(msg, dict):
                sender = msg.get("sender", "user")
                text = msg.get("text", "")
                history_items.append(f"  [{sender}]: {text}")
        if history_items:
            history_text = "\n".join(history_items)

    prompt = f"""You are a goal decomposition assistant for MyWant.
Original goal: {goal_text}

AVAILABLE CAPABILITIES:
{cap_text}

AVAILABLE WANT TYPES: knowledge, reminder (NEVER use "goal" type)

Conversation history:
{history_text if history_text else "(no previous messages)"}

The user now wants to modify the plan: {modification_request}

Provide an updated breakdown of sub-wants. Respond ONLY with valid JSON:
{{
  "breakdown": [
    {{
      "name": "kebab-case-name",
      "type": "want-type",
      "description": "what this achieves",
      "params": {{}}
    }}
  ],
  "response_text": "Brief explanation of the updated breakdown"
}}"""

    content = call_claude_cli(prompt)
    return parse_json_response(content)


def main():
    try:
        raw_input = sys.stdin.read()
        input_data = json.loads(raw_input)
    except json.JSONDecodeError as e:
        sys.stderr.write(f"[goal_thinker] Failed to parse stdin JSON: {e}\n")
        sys.exit(1)

    phase = input_data.get("phase", "decompose")
    goal_text = input_data.get("goal_text", "")
    available_capabilities = input_data.get("available_capabilities", [])

    # Check if Claude CLI is available
    use_llm = shutil.which("claude") is not None

    try:
        if phase == "decompose":
            if use_llm:
                result = decompose_with_llm(goal_text, available_capabilities)
            else:
                result = fallback_breakdown(goal_text)

        elif phase == "replan":
            conversation_history = input_data.get("conversation_history", [])
            cc_responses = input_data.get("cc_responses", [])
            modification_request = input_data.get("modification_request", "")

            if use_llm:
                result = replan_with_llm(
                    goal_text,
                    conversation_history,
                    cc_responses,
                    modification_request,
                    available_capabilities,
                )
            else:
                result = fallback_breakdown(goal_text, modification_request)
        else:
            result = fallback_breakdown(goal_text)

    except Exception as e:
        sys.stderr.write(f"[goal_thinker] LLM call failed: {e}\n")
        result = fallback_breakdown(goal_text)

    print(json.dumps(result))


if __name__ == "__main__":
    main()
