#!/usr/bin/env python3
"""
goal_thinker.py - Goal decomposition and replanning tool for MyWant.

Reads JSON from stdin, calls Anthropic LLM (or falls back to a static response),
and writes the result as JSON to stdout.

Input format (phase=decompose):
  {
    "phase": "decompose",
    "goal_text": "string"
  }

Input format (phase=replan):
  {
    "phase": "replan",
    "goal_text": "string",
    "conversation_history": [...],
    "cc_responses": [...],
    "modification_request": "string"
  }

Output format:
  {
    "breakdown": [
      {
        "name": "kebab-case-name",
        "type": "goal|knowledge|reminder",
        "description": "what this achieves",
        "params": {}
      }
    ],
    "response_text": "explanation"
  }
"""

import sys
import json
import os


def fallback_breakdown(goal_text: str, modification_request: str = "") -> dict:
    """Return a fixed breakdown when LLM is not available."""
    name_base = goal_text[:30].lower().replace(" ", "-").replace("、", "-").replace("。", "")
    # Remove non-alphanumeric/dash characters
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


def decompose_with_llm(client, goal_text: str) -> dict:
    """Use Anthropic LLM to decompose a goal into sub-wants."""
    prompt = f"""You are a goal decomposition assistant for a productivity system called MyWant.

Your task is to break down the following user goal into actionable sub-wants.

User goal: {goal_text}

Each sub-want must have:
- name: a kebab-case identifier (e.g., "research-destinations", "book-flight")
- type: one of "goal" (complex sub-goal), "knowledge" (information gathering), "reminder" (scheduled notification)
- description: clear description of what this achieves
- params: object with type-specific parameters
  - For "knowledge" type: {{"want": "description of what to learn"}}
  - For "reminder" type: {{"message": "reminder text", "duration_from_now": "1 hours", "require_reaction": false}}
  - For "goal" type: {{"goal_text": "the sub-goal text"}}

Respond ONLY with a valid JSON object in this exact format:
{{
  "breakdown": [
    {{
      "name": "kebab-case-name",
      "type": "goal|knowledge|reminder",
      "description": "what this achieves",
      "params": {{}}
    }}
  ],
  "response_text": "Brief explanation of the breakdown"
}}

Keep the breakdown to 2-5 items. Focus on concrete, actionable steps."""

    message = client.messages.create(
        model="claude-3-5-haiku-20241022",
        max_tokens=1024,
        messages=[{"role": "user", "content": prompt}],
    )

    content = message.content[0].text.strip()
    # Extract JSON from response (may have markdown code blocks)
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
) -> dict:
    """Use Anthropic LLM to replan based on conversation history."""
    history_text = ""
    if conversation_history:
        history_items = []
        for i, msg in enumerate(conversation_history[-10:]):  # Last 10 messages
            if isinstance(msg, dict):
                sender = msg.get("sender", "user")
                text = msg.get("text", "")
                history_items.append(f"  [{sender}]: {text}")
        if history_items:
            history_text = "\n".join(history_items)

    prompt = f"""You are a goal decomposition assistant for a productivity system called MyWant.

The user originally had this goal: {goal_text}

Conversation history:
{history_text if history_text else "(no previous messages)"}

The user now wants to modify the plan with this request: {modification_request}

Please provide an updated breakdown of sub-wants that reflects the user's modification request.

Each sub-want must have:
- name: a kebab-case identifier (e.g., "research-destinations", "book-flight")
- type: one of "goal" (complex sub-goal), "knowledge" (information gathering), "reminder" (scheduled notification)
- description: clear description of what this achieves
- params: object with type-specific parameters
  - For "knowledge" type: {{"want": "description of what to learn"}}
  - For "reminder" type: {{"message": "reminder text", "duration_from_now": "1 hours", "require_reaction": false}}
  - For "goal" type: {{"goal_text": "the sub-goal text"}}

Respond ONLY with a valid JSON object in this exact format:
{{
  "breakdown": [
    {{
      "name": "kebab-case-name",
      "type": "goal|knowledge|reminder",
      "description": "what this achieves",
      "params": {{}}
    }}
  ],
  "response_text": "Brief explanation of what changed and why"
}}

Keep the breakdown to 2-5 items."""

    message = client.messages.create(
        model="claude-3-5-haiku-20241022",
        max_tokens=1024,
        messages=[{"role": "user", "content": prompt}],
    )

    content = message.content[0].text.strip()
    # Extract JSON from response (may have markdown code blocks)
    if "```json" in content:
        content = content.split("```json")[1].split("```")[0].strip()
    elif "```" in content:
        content = content.split("```")[1].split("```")[0].strip()

    return json.loads(content)


def main():
    # Read input from stdin
    try:
        raw_input = sys.stdin.read()
        input_data = json.loads(raw_input)
    except json.JSONDecodeError as e:
        sys.stderr.write(f"[goal_thinker] Failed to parse stdin JSON: {e}\n")
        sys.exit(1)

    phase = input_data.get("phase", "decompose")
    goal_text = input_data.get("goal_text", "")

    # Try to use Anthropic LLM if available
    use_llm = False
    client = None
    try:
        import anthropic  # noqa: F401

        api_key = os.environ.get("ANTHROPIC_API_KEY", "")
        if api_key:
            client = anthropic.Anthropic(api_key=api_key)
            use_llm = True
        else:
            sys.stderr.write("[goal_thinker] ANTHROPIC_API_KEY not set, using fallback\n")
    except ImportError:
        sys.stderr.write("[goal_thinker] anthropic package not installed, using fallback\n")

    try:
        if phase == "decompose":
            if use_llm and client:
                result = decompose_with_llm(client, goal_text)
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
                )
            else:
                result = fallback_breakdown(goal_text, modification_request)

        else:
            sys.stderr.write(f"[goal_thinker] Unknown phase: {phase}\n")
            result = fallback_breakdown(goal_text)

    except Exception as e:
        sys.stderr.write(f"[goal_thinker] LLM call failed: {e}, using fallback\n")
        modification_request = input_data.get("modification_request", "")
        result = fallback_breakdown(goal_text, modification_request)

    # Ensure breakdown is a list
    if "breakdown" not in result or not isinstance(result["breakdown"], list):
        result["breakdown"] = []
    if "response_text" not in result:
        result["response_text"] = "Goal breakdown complete."

    print(json.dumps(result))


if __name__ == "__main__":
    main()
