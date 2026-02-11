package mywant

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// JSONParser provides utilities for parsing JSON data with fallback strategies
type JSONParser struct {
	input string
}

// NewJSONParser creates a new JSON parser
func NewJSONParser(input string) *JSONParser {
	return &JSONParser{input: input}
}

// ParseAsArray attempts to parse the input as a JSON array
func (jp *JSONParser) ParseAsArray() ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(jp.input), &result); err != nil {
		return nil, fmt.Errorf("failed to parse as array: %w", err)
	}
	return result, nil
}

// ParseAsObject attempts to parse the input as a JSON object
func (jp *JSONParser) ParseAsObject() (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jp.input), &result); err != nil {
		return nil, fmt.Errorf("failed to parse as object: %w", err)
	}
	return result, nil
}

// ParseFlexible attempts to parse as array first, then object, then returns the input as-is
func (jp *JSONParser) ParseFlexible() (interface{}, error) {
	// Try array
	if arr, err := jp.ParseAsArray(); err == nil {
		return arr, nil
	}

	// Try object
	if obj, err := jp.ParseAsObject(); err == nil {
		return obj, nil
	}

	// Return raw string if not valid JSON
	return jp.input, nil
}

// ExtractJSON attempts to extract JSON from text that may contain markdown code blocks or other formatting
func (jp *JSONParser) ExtractJSON() (string, error) {
	text := jp.input

	// Remove markdown code blocks (```json ... ``` or ``` ... ```)
	re := regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		text = strings.TrimSpace(matches[1])
	}

	// Try to find JSON-like content between { } or [ ]
	if strings.Contains(text, "{") || strings.Contains(text, "[") {
		// Find first { or [
		startIdx := strings.IndexAny(text, "{[")
		if startIdx == -1 {
			return "", fmt.Errorf("no JSON structure found")
		}

		// Determine closing bracket
		var closeBracket byte
		if text[startIdx] == '{' {
			closeBracket = '}'
		} else {
			closeBracket = ']'
		}

		// Find matching closing bracket
		depth := 0
		for i := startIdx; i < len(text); i++ {
			if text[i] == text[startIdx] {
				depth++
			} else if text[i] == closeBracket {
				depth--
				if depth == 0 {
					return text[startIdx : i+1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("no valid JSON found in text")
}

// ParseWithExtraction attempts to extract and parse JSON from text
func (jp *JSONParser) ParseWithExtraction() (interface{}, error) {
	// First try direct parsing
	if result, err := jp.ParseFlexible(); err == nil {
		// Check if result is not just the raw string
		if _, isString := result.(string); !isString || result != jp.input {
			return result, nil
		}
	}

	// Try extracting JSON first
	extracted, err := jp.ExtractJSON()
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// Parse extracted JSON
	extractedParser := NewJSONParser(extracted)
	return extractedParser.ParseFlexible()
}

// Helper function for common use case: parse Goose/MCP response
func ParseGooseResponse(responseText string) (interface{}, error) {
	parser := NewJSONParser(responseText)
	return parser.ParseWithExtraction()
}
