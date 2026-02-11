package templates

import (
	"encoding/json"
	"fmt"
	"strings"
)

// WASMTemplate represents a pre-compiled WASM template
type WASMTemplate struct {
	ToolName        string
	Category        string
	Description     string
	SourceCode      string
	ExampleRequest  map[string]interface{}
	ExampleResponse map[string]interface{}
}

// GmailSearchTemplate - Gmail検索用テンプレート
var GmailSearchTemplate = WASMTemplate{
	ToolName:    "search_emails",
	Category:    "gmail",
	Description: "Gmail検索クエリを実行し、メール一覧を返す",
	SourceCode: `package main

import (
	"encoding/json"
	"fmt"
)

func TransformRequest(params map[string]any) map[string]any {
	query := ""
	if q, ok := params["prompt"].(string); ok {
		query = q
	} else if q, ok := params["query"].(string); ok {
		query = q
	}

	maxResults := 10
	if mr, ok := params["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	return map[string]any{
		"query":      query,
		"maxResults": maxResults,
	}
}

func ParseResponse(rawResponse map[string]any) map[string]any {
	result := map[string]any{
		"status": "completed",
		"emails": []map[string]any{},
	}

	// Extract "content" field from MCP response
	if content, ok := rawResponse["content"].([]interface{}); ok {
		var emails []map[string]any
		for _, item := range content {
			if textContent, ok := item.(map[string]any); ok {
				if text, ok := textContent["text"].(string); ok {
					// Parse email JSON from text
					var email map[string]any
					if err := json.Unmarshal([]byte(text), &email); err == nil {
						emails = append(emails, email)
					}
				}
			}
		}
		result["emails"] = emails
		result["count"] = len(emails)
	}

	return result
}`,
	ExampleRequest: map[string]interface{}{
		"query":      "from:boss about:project",
		"maxResults": 10,
	},
	ExampleResponse: map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": `{"id":"msg123","from":"boss@company.com","subject":"Project X Update"}`,
			},
		},
	},
}

// GmailSendTemplate - Gmail送信用テンプレート
var GmailSendTemplate = WASMTemplate{
	ToolName:    "send_email",
	Category:    "gmail",
	Description: "Gmailメールを送信する",
	SourceCode: `package main

import (
	"encoding/json"
)

func TransformRequest(params map[string]any) map[string]any {
	to := ""
	if t, ok := params["to"].(string); ok {
		to = t
	}

	subject := ""
	if s, ok := params["subject"].(string); ok {
		subject = s
	}

	body := ""
	if b, ok := params["body"].(string); ok {
		body = b
	} else if b, ok := params["message"].(string); ok {
		body = b
	}

	return map[string]any{
		"to":      to,
		"subject": subject,
		"body":    body,
	}
}

func ParseResponse(rawResponse map[string]any) map[string]any {
	result := map[string]any{
		"status": "sent",
	}

	if content, ok := rawResponse["content"].([]interface{}); ok && len(content) > 0 {
		if textContent, ok := content[0].(map[string]any); ok {
			if text, ok := textContent["text"].(string); ok {
				var data map[string]any
				if err := json.Unmarshal([]byte(text), &data); err == nil {
					if msgId, ok := data["messageId"].(string); ok {
						result["message_id"] = msgId
					}
				}
			}
		}
	}

	return result
}`,
	ExampleRequest: map[string]interface{}{
		"to":      "colleague@company.com",
		"subject": "Meeting Tomorrow",
		"body":    "Let's meet at 10am.",
	},
	ExampleResponse: map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": `{"messageId":"msg456","status":"sent"}`,
			},
		},
	},
}

// GmailReadTemplate - Gmail読み取り用テンプレート
var GmailReadTemplate = WASMTemplate{
	ToolName:    "get_message",
	Category:    "gmail",
	Description: "指定されたIDのGmailメッセージを取得",
	SourceCode: `package main

import (
	"encoding/json"
)

func TransformRequest(params map[string]any) map[string]any {
	messageID := ""
	if id, ok := params["message_id"].(string); ok {
		messageID = id
	} else if id, ok := params["id"].(string); ok {
		messageID = id
	}

	return map[string]any{
		"id": messageID,
	}
}

func ParseResponse(rawResponse map[string]any) map[string]any {
	result := map[string]any{
		"status": "retrieved",
	}

	if content, ok := rawResponse["content"].([]interface{}); ok && len(content) > 0 {
		if textContent, ok := content[0].(map[string]any); ok {
			if text, ok := textContent["text"].(string); ok {
				var email map[string]any
				if err := json.Unmarshal([]byte(text), &email); err == nil {
					result["email"] = email
					if from, ok := email["from"].(string); ok {
						result["from"] = from
					}
					if subject, ok := email["subject"].(string); ok {
						result["subject"] = subject
					}
				}
			}
		}
	}

	return result
}`,
	ExampleRequest: map[string]interface{}{
		"id": "msg123",
	},
	ExampleResponse: map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": `{"id":"msg123","from":"sender@example.com","subject":"Hello","body":"Message body"}`,
			},
		},
	},
}

// AllTemplates - 利用可能な全テンプレート
var AllTemplates = []WASMTemplate{
	GmailSearchTemplate,
	GmailSendTemplate,
	GmailReadTemplate,
}

// FindRelevantTemplates searches for templates matching the user request
func FindRelevantTemplates(userRequest string, category string) []WASMTemplate {
	var relevant []WASMTemplate

	lowerRequest := strings.ToLower(userRequest)

	for _, tmpl := range AllTemplates {
		// Category filter
		if category != "" && tmpl.Category != category {
			continue
		}

		// Simple keyword matching
		if tmpl.ToolName == "search_emails" {
			if containsAny(lowerRequest, "search", "find", "emails", "メール検索", "探す") {
				relevant = append(relevant, tmpl)
			}
		} else if tmpl.ToolName == "send_email" {
			if containsAny(lowerRequest, "send", "email", "message", "送信", "メール送る") {
				relevant = append(relevant, tmpl)
			}
		} else if tmpl.ToolName == "get_message" {
			if containsAny(lowerRequest, "read", "get", "retrieve", "読む", "取得") {
				relevant = append(relevant, tmpl)
			}
		}
	}

	// If no specific match, return all templates in the category for reference
	if len(relevant) == 0 && category != "" {
		for _, tmpl := range AllTemplates {
			if tmpl.Category == category {
				relevant = append(relevant, tmpl)
			}
		}
	}

	return relevant
}

// containsAny checks if text contains any of the given keywords
func containsAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// FormatTemplatesForPrompt formats templates as a string for inclusion in prompts
func FormatTemplatesForPrompt(templates []WASMTemplate) string {
	if len(templates) == 0 {
		return ""
	}

	result := "\n\nREFERENCE TEMPLATES (学習用 - これらのパターンを参考にしてください):\n\n"
	for i, tmpl := range templates {
		result += "--- Template " + fmt.Sprint(i+1) + ": " + tmpl.Description + " ---\n"
		result += "Tool Name: " + tmpl.ToolName + "\n\n"
		result += "Source Code:\n```go\n" + tmpl.SourceCode + "\n```\n\n"

		exampleReqJSON, _ := json.MarshalIndent(tmpl.ExampleRequest, "", "  ")
		exampleRespJSON, _ := json.MarshalIndent(tmpl.ExampleResponse, "", "  ")

		result += "Example Request:\n" + string(exampleReqJSON) + "\n\n"
		result += "Example Response:\n" + string(exampleRespJSON) + "\n\n"
	}

	return result
}
