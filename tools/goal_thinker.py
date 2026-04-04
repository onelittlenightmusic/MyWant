#!/usr/bin/env python3
"""
goal_thinker.py - Goal decomposition and replanning tool for MyWant.

Reads JSON from stdin, calls Anthropic LLM (or falls back to a static response),
and writes the result as JSON to stdout.

Input format includes system capabilities discovered via MCP.
"""

import sys
import json
import os


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
                    "want": description,
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


def decompose_with_llm(client, goal_text: str, available_capabilities: list) -> dict:
    """Use Anthropic LLM to decompose a goal into sub-wants."""
    cap_text = format_capabilities(available_capabilities)

    prompt = f"""You are a goal decomposition assistant for a productivity system called MyWant.
Your task is to break down a high-level goal into actionable sub-wants.

AVAILABLE CAPABILITIES:
{cap_text}

AVAILABLE WANT TYPES: goal, knowledge, reminder (use specific types when capabilities match)

User goal: {goal_text}

Each sub-want must have:
- name: a kebab-case identifier (e.g., "research-destinations", "book-flight")
- type: MUST be one of the available want types listed above.
- description: clear description of what this achieves
- params: object with type-specific parameters.

STRATEGY:
1. Prefer specific want types if they match the goal (e.g., "flight" for travel).
2. If a specific type is not yet available but seems needed, use "knowledge" to research it.
3. If the goal is too big, use "goal" to create a sub-goal.

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

    message = client.messages.create(
        model="claude-3-5-haiku-20241022",
        max_tokens=1024,
        messages=[{"role": "user", "content": prompt}],
    )

    content = message.content[0].text.strip()
    if "```json" in content:
        content = content.split("```json")[1].split("```")[0].strip()
    elif "```" in content:
        content = content.split("```")[1].split("```")[0].strip()

    return json.loads(content)


def replan_with_llm(
    client,
    goal_text: str,
    conversation_history: list,
    cc_responses: list,
    modification_request: str,
    available_capabilities: list,
) -> dict:
    """Use Anthropic LLM to replan based on conversation history."""
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

AVAILABLE WANT TYPES: goal, knowledge, reminder (use specific types when capabilities match)

Conversation history:
{history_text if history_text else "(no previous messages)"}

The user now wants to modify the plan: {modification_request}

Provide an updated breakdown of sub-wants. Respond ONLY with valid JSON."""

    message = client.messages.create(
        model="claude-3-5-haiku-20241022",
        max_tokens=1024,
        messages=[{"role": "user", "content": prompt}],
    )

    content = message.content[0].text.strip()
    if "```json" in content:
        content = content.split("```json")[1].split("```")[0].strip()
    elif "```" in content:
        content = content.split("```")[1].split("```")[0].strip()

    return json.loads(content)


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

    use_llm = False
    client = None
    try:
        import anthropic
        api_key = os.environ.get("ANTHROPIC_API_KEY", "")
        if api_key:
            client = anthropic.Anthropic(api_key=api_key)
            use_llm = True
    except ImportError:
        pass

    try:
        if phase == "decompose":
            if use_llm and client:
                result = decompose_with_llm(client, goal_text, available_capabilities)
            else:
                result = fallback_breakdown(goal_text)

        elif phase == "replan":
            conversation_history = input_data.get("conversation_history", [])
            cc_responses = input_data.get("cc_responses", [])
            modification_request = input_data.get("modification_request", "")

            if use_llm and client:
                result = replan_with_llm(
                    client,
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
