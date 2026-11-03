package main

// robot_tools.go — the board robot's own tools, for Claude.
//
// The robot on the MyWant board (the iPhone's, the Mac's) works the board
// with the tools the server lists at /api/v1/tools — board, point,
// look, use_capability (a want's capabilities: its type's, and the standard
// ones every want has, such as restart), deploy_want, … — and calls them at
// /api/v1/tools/call. These are those same tools, built from that same list and
// sent to that same call, so Claude asks the board what the robot asks, and a
// tool, a capability or a choice added on the server is here without a
// change to this file. The choices (the types a want can be, the
// capabilities on the board now) are the board's as it is, so the list is
// read again every refreshInterval.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const refreshInterval = 30 * time.Second

// robotPrefix keeps the robot's tools apart from this server's own catalogue
// tools (list_want_types, list_capabilities — an agent's, not a want's).
const robotPrefix = "board_"

type robotManifest struct {
	Instructions string      `json:"instructions"`
	Tools        []robotTool `json:"tools"`
}

type robotTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Arguments   []struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Required    bool     `json:"required"`
		Choices     []string `json:"choices"`
	} `json:"arguments"`
	Writes bool `json:"writes"`
}

// robotQuestionTools hear what the person asked (the server's
// fmHearsQuestion): look picks which of a want's answers it means, a
// capability's skill is handed it.
var robotQuestionTools = map[string]bool{"look": true, "deploy_want": true, "use_capability": true}

func (s *MCPServer) robotManifest() (robotManifest, error) {
	var m robotManifest
	resp, err := s.httpClient.Get(s.apiBaseURL + "/api/v1/tools")
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return m, fmt.Errorf("manifest: %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return m, json.NewDecoder(resp.Body).Decode(&m)
}

// robotCall runs one of the robot's tools on the board and says what it said.
func (s *MCPServer) robotCall(ctx context.Context, tool string, args map[string]string) (string, error) {
	body, _ := json.Marshal(map[string]any{"tool": tool, "arguments": args})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiBaseURL+"/api/v1/tools/call", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// A capability's skill, a want run again: longer than a catalogue read.
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %d %s", tool, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out struct {
		Output string `json:"output"`
		Card   *struct {
			Kind, ID, Name string
		} `json:"card"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	text := out.Output
	if out.Card != nil && out.Card.Name != "" {
		text += fmt.Sprintf("\n(shown on the board: %s %s)", out.Card.Kind, out.Card.Name)
	}
	return text, nil
}

// robotSchema is a tool's arguments as a JSON schema: each a string, its
// choices an enum, and — for a tool that hears it — the person's question.
func robotSchema(t robotTool) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, a := range t.Arguments {
		p := map[string]any{"type": "string", "description": a.Description}
		if len(a.Choices) > 0 {
			p["enum"] = a.Choices
		}
		props[a.Name] = p
		if a.Required {
			required = append(required, a.Name)
		}
	}
	if robotQuestionTools[t.Name] {
		props["question"] = map[string]any{
			"type":        "string",
			"description": "What the person asked, in their words — which of a want's answers it is about, what a capability is asked.",
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

// registerRobotTools adds the robot's tools as they are listed now, and keeps
// them in step with the board: read again every refreshInterval, a tool gone
// from the list taken away.
func (s *MCPServer) registerRobotTools(ctx context.Context, server *mcp.Server) {
	have := map[string]string{} // name → its schema and description, as added
	sync := func() {
		m, err := s.robotManifest()
		if err != nil {
			log.Printf("[MCP] robot tools: %v", err)
			return
		}
		seen := map[string]bool{}
		for _, t := range m.Tools {
			t := t
			name := robotPrefix + t.Name
			seen[name] = true
			schema := robotSchema(t)
			desc := t.Description
			if t.Writes {
				desc += " (Changes the board.)"
			}
			sig, _ := json.Marshal([]any{desc, schema})
			if have[name] == string(sig) {
				continue
			}
			have[name] = string(sig)
			server.AddTool(&mcp.Tool{
				Name:        name,
				Description: desc,
				InputSchema: schema,
				Annotations: &mcp.ToolAnnotations{ReadOnlyHint: !t.Writes},
			}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				raw := map[string]any{}
				if len(req.Params.Arguments) > 0 {
					if err := json.Unmarshal(req.Params.Arguments, &raw); err != nil {
						return ErrorResult("bad arguments: " + err.Error()), nil
					}
				}
				args := map[string]string{}
				for k, v := range raw {
					if v == nil {
						continue
					}
					if k == "question" {
						k = "_question" // the server's fmQuestionArg
					}
					args[k] = fmt.Sprint(v)
				}
				out, err := s.robotCall(ctx, t.Name, args)
				if err != nil {
					return ErrorResult(err.Error()), nil
				}
				return SuccessResult(out), nil
			})
		}
		var gone []string
		for name := range have {
			if !seen[name] {
				gone = append(gone, name)
				delete(have, name)
			}
		}
		if len(gone) > 0 {
			server.RemoveTools(gone...)
		}
	}
	sync()
	go func() {
		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sync()
			}
		}
	}()
}

// robotInstructions is how the robot is told to work the board, for Claude:
// the same words, named for these tools.
func (s *MCPServer) robotInstructions() string {
	m, err := s.robotManifest()
	if err != nil || m.Instructions == "" {
		return "The board_* tools work the MyWant board as its robot does (board, point, look, use_capability, deploy_want …). Start with board_board to see what is on it."
	}
	return "The board_* tools are the MyWant board robot's own tools (board_look is the robot's look, board_use_capability its use_capability, …). The robot's instructions, which hold for them too:\n\n" + m.Instructions
}
