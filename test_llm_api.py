#!/usr/bin/env python3
"""
Test script for LLM inference API
Tests the POST /api/v1/llm/query endpoint
"""

import requests
import json
import sys
import time

def test_llm_api():
    """Test the LLM API endpoint"""
    url = "http://localhost:8080/api/v1/llm/query"
    headers = {"Content-Type": "application/json"}
    test_message = "What is 2+2? Please answer briefly."
    data = {"message": test_message, "model": "gpt-oss:20b"}

    print("📮 Sending request to " + url)
    print("📝 Message: " + test_message)
    print("🤖 Model: gpt-oss:20b")
    print("")
    print("⏳ Waiting for response (timeout: 120 seconds)...")
    print("")

    try:
        start_time = time.time()
        response = requests.post(url, json=data, headers=headers, timeout=120)
        elapsed = time.time() - start_time

        print(f"✅ Response received in {elapsed:.2f}s")
        print("")
        print(f"Status Code: {response.status_code}")

        if response.status_code == 200:
            result = response.json()
            print(f"Model: {result.get('model')}")
            print(f"Timestamp: {result.get('timestamp')}")
            print("")
            print("🤖 LLM Response:")
            print("-" * 60)
            response_text = result.get('response', 'No response field')
            truncated = response_text[:500] + ("..." if len(response_text) > 500 else "")
            print(truncated)
            print("-" * 60)
            print("")
            print("✅ LLM API test PASSED!")
            return 0
        else:
            print(f"❌ Error response:")
            print(response.text)
            return 1

    except requests.exceptions.ConnectionError as e:
        print(f"❌ Connection Error: Cannot connect to server at {url}")
        print(f"   Make sure MyWant server is running on http://localhost:8080")
        print(f"   Run: make restart-all")
        return 1
    except requests.exceptions.Timeout:
        print(f"❌ Request timed out after 120 seconds")
        print(f"   Ollama may be slow or not responding")
        print(f"   Check GPT_BASE_URL environment variable or Ollama status")
        return 1
    except Exception as e:
        print(f"❌ Error: {e}")
        return 1

if __name__ == "__main__":
    sys.exit(test_llm_api())
