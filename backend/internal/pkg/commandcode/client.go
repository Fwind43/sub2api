// CommandCode GO protocol adapted from the local CommandCode provider.
package commandcode

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type Client struct {
	BaseURL    string
	HTTPClient HTTPDoer
	UserAgent  string
	Version    string
	// relay, when set by ChatCompletion, receives the terminal upstream
	// response exactly once. generate publishes it only after model-route
	// repair has been resolved, so an intermediate 403 can never abort a repair.
	relay *responseRelay
}

type Request struct {
	customTools map[string]bool
	Tools       []map[string]any `json:"tools"`
	ToolChoice  any              `json:"tool_choice"`
	OnToolCall  func(map[string]any) error
	OnReasoning func(string) error

	Model           string           `json:"model"`
	Messages        []map[string]any `json:"messages"`
	MaxTokens       int              `json:"max_tokens"`
	MaxComplete     int              `json:"max_completion_tokens"`
	Stream          bool             `json:"stream"`
	Temperature     *float64         `json:"temperature"`
	ReasoningEffort string           `json:"reasoning_effort"`
}

type Result struct {
	ToolCalls []map[string]any

	Text         string
	Reasoning    string
	FinishReason string
	Usage        map[string]any
}

func (c *Client) Generate(ctx context.Context, apiKey, model string, payload Request, onDelta func(string) error) (Result, error) {
	return c.generate(ctx, apiKey, model, payload, onDelta, true)
}

func (c *Client) generate(ctx context.Context, apiKey, model string, payload Request, onDelta func(string) error, repair bool) (Result, error) {
	if strings.TrimSpace(apiKey) == "" {
		return Result{}, fmt.Errorf("missing CommandCode API key")
	}
	if strings.TrimSpace(model) == "" {
		return Result{}, fmt.Errorf("missing CommandCode model")
	}
	upstreamMessages := make([]map[string]any, 0, len(payload.Messages))
	var systemInstructions []string
	toolNames := map[string]string{}
	for _, message := range payload.Messages {
		role := firstString(message, "role")
		if role == "" {
			role = "user"
		}
		text := messageText(message)
		if role == "system" || role == "developer" {
			systemInstructions = append(systemInstructions, text)
			continue
		}
		parts := []map[string]any{}
		if role == "tool" {
			id := firstString(message, "tool_call_id")
			name := toolNames[id]
			if name == "" {
				return Result{}, fmt.Errorf("tool result has no matching tool call: %s", id)
			}
			parts = append(parts, map[string]any{"type": "tool-result", "toolCallId": id, "toolName": name, "output": map[string]any{"type": "text", "value": text}})
		} else {
			// Tool calls are structured content, never a text fallback.
			contentParts, err := chatContentParts(message["content"])
			if err != nil {
				return Result{}, err
			}
			parts = append(parts, contentParts...)
			if calls, ok := message["tool_calls"].([]any); ok {
				for _, raw := range calls {
					call, ok := raw.(map[string]any)
					if !ok {
						return Result{}, fmt.Errorf("invalid tool call")
					}
					fn, ok := call["function"].(map[string]any)
					if !ok {
						return Result{}, fmt.Errorf("invalid tool function")
					}
					id, name := firstString(call, "id"), firstString(fn, "name")
					var input any
					if err := json.Unmarshal([]byte(rawString(fn, "arguments")), &input); err != nil {
						return Result{}, fmt.Errorf("invalid arguments for %s: %w", name, err)
					}
					toolNames[id] = name
					parts = append(parts, map[string]any{"type": "tool-call", "toolCallId": id, "toolName": name, "input": input})
				}
			}
		}
		if len(parts) > 0 {
			upstreamMessages = append(upstreamMessages, map[string]any{"role": role, "content": parts})
		}
	}
	tools := []map[string]any{}
	for _, tool := range payload.Tools {
		fn, ok := tool["function"].(map[string]any)
		if !ok || firstString(tool, "type") != "function" {
			return Result{}, fmt.Errorf("unsupported tool type")
		}
		schema := fn["parameters"]
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, map[string]any{"name": fn["name"], "description": fn["description"], "input_schema": schema})
	}
	if payload.ToolChoice == "none" {
		tools = []map[string]any{}
	}
	if len(upstreamMessages) == 0 {
		return Result{}, fmt.Errorf("no text content in chat messages")
	}
	maxTokens := payload.MaxTokens
	if payload.MaxComplete > 0 {
		maxTokens = payload.MaxComplete
	}
	// CommandCode requires a positive limit even when the caller omits it.
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	body := map[string]any{
		"config": map[string]any{
			"workingDir":    "/tmp",
			"date":          time.Now().Format("2006-01-02"),
			"environment":   "linux",
			"structure":     []any{},
			"isGitRepo":     false,
			"currentBranch": "",
			"mainBranch":    "",
			"gitStatus":     "",
			"recentCommits": []any{},
		},
		"memory":         nil,
		"permissionMode": "standard",
		"threadId":       randomUUID(),
		"params": map[string]any{
			"model":       model,
			"canonicalID": model,
			"messages":    upstreamMessages,
			"tools":       tools,
			"max_tokens":  maxTokens,
		},
	}
	if len(systemInstructions) > 0 {
		body["params"].(map[string]any)["system"] = strings.Join(systemInstructions, "\n\n")
	}
	params := body["params"].(map[string]any)
	if payload.Temperature != nil {
		params["temperature"] = *payload.Temperature
	}
	if payload.ReasoningEffort != "" {
		params["reasoning_effort"] = payload.ReasoningEffort
	}
	encoded, errMarshal := json.Marshal(body)
	if errMarshal != nil {
		return Result{}, errMarshal
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.commandcode.ai"
	}
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodPost, base+"/alpha/generate", bytes.NewReader(encoded))
	if errRequest != nil {
		return Result{}, errRequest
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if c.UserAgent != "" {
		request.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Version != "" {
		request.Header.Set("x-command-code-version", c.Version)
	}
	request.Header.Set("x-cli-environment", "production")
	if c.HTTPClient == nil {
		return Result{}, fmt.Errorf("CommandCode HTTP client is required")
	}
	response, errDo := c.HTTPClient.Do(request)
	if errDo != nil {
		c.relay.send(nil)
		return Result{}, errDo
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if repair && response.StatusCode == http.StatusForbidden && strings.Contains(string(detail), "Model/provider not recognized:") {
			response.Body.Close()
			resolved, lookupErr := c.resolveRejectedModel(ctx, apiKey, model)
			if lookupErr == nil && resolved != model {
				return c.generate(ctx, apiKey, resolved, payload, onDelta, false)
			}
		}
		// Terminal upstream error: relay the exact status/headers/body so
		// ChatCompletion can preserve the caller-visible error contract.
		snapshot := *response
		snapshot.Body = io.NopCloser(bytes.NewReader(detail))
		c.relay.send(&snapshot)
		return Result{}, fmt.Errorf("upstream status %d: %s", response.StatusCode, truncate(string(detail), 400))
	}
	// A 200 is terminal: relay before draining the body so ChatCompletion can
	// begin delivering chunks while generate continues to stream them.
	c.relay.send(response)
	result := Result{}
	finished := false
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, ":") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if line == "[DONE]" {
			break
		}
		var event map[string]any
		if errUnmarshal := json.Unmarshal([]byte(line), &event); errUnmarshal != nil {
			continue
		}
		eventType := firstString(event, "type")
		switch eventType {
		case "text-delta", "reasoning-delta", "reasoning":
			text := rawString(event, "text", "delta", "content")
			if text == "" {
				text = firstString(event, "text", "delta", "content")
			}
			if text == "" {
				continue
			}
			if eventType == "reasoning-delta" || eventType == "reasoning" {
				result.Reasoning += text
				if payload.OnReasoning != nil {
					if err := payload.OnReasoning(text); err != nil {
						return result, err
					}
				}
				continue
			}
			result.Text += text
			if onDelta != nil {
				if errDelta := onDelta(text); errDelta != nil {
					return result, errDelta
				}
			}
		case "tool-call":
			if executed, _ := event["providerExecuted"].(bool); executed {
				continue
			}
			input, ok := event["input"]
			if !ok {
				input = event["args"]
			}
			arguments := ""
			if value, ok := input.(string); ok {
				arguments = value
			} else {
				encoded, err := json.Marshal(input)
				if err != nil {
					return result, err
				}
				arguments = string(encoded)
			}
			// Only declared custom tools may return verbatim non-JSON input.
			if payload.customTools[firstString(event, "toolName")] {
				if text, ok := input.(string); ok {
					var wrapped struct {
						Input *string `json:"input"`
					}
					if json.Unmarshal([]byte(arguments), &wrapped) != nil || wrapped.Input == nil {
						encoded, _ := json.Marshal(map[string]string{"input": text})
						arguments = string(encoded)
					}
				}
			}
			if !json.Valid([]byte(arguments)) {
				return result, fmt.Errorf("invalid upstream tool arguments")
			}
			call := map[string]any{"id": firstString(event, "toolCallId"), "type": "function", "function": map[string]any{"name": firstString(event, "toolName"), "arguments": arguments}}
			if call["id"] == "" {
				return result, fmt.Errorf("missing upstream tool call id")
			}
			result.ToolCalls = append(result.ToolCalls, call)
			if payload.OnToolCall != nil {
				delta := map[string]any{"index": len(result.ToolCalls) - 1, "id": call["id"], "type": "function", "function": call["function"]}
				if err := payload.OnToolCall(delta); err != nil {
					return result, err
				}
			}
		case "finish":
			finished = true
			if value := firstString(event, "finishReason", "finish_reason"); value != "" {
				result.FinishReason = mapFinishReason(value)
			}
			if usage, ok := event["totalUsage"].(map[string]any); ok {
				result.Usage = usage
			} else if usage, ok := event["usage"].(map[string]any); ok {
				result.Usage = usage
			}
		case "error":
			message := firstString(event, "message", "error", "detail")
			if message == "" {
				message = "upstream error event"
			}
			return result, fmt.Errorf("%s", message)
		}
	}
	if errScan := scanner.Err(); errScan != nil {
		return result, errScan
	}
	if !finished {
		return result, io.ErrUnexpectedEOF
	}
	if len(result.ToolCalls) > 0 {
		result.FinishReason = "tool_calls"
	}
	return result, nil
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text, isString := value.(string); isString && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	for _, nested := range []string{"auth", "credentials", "data", "user", "commandCode", "commandcode"} {
		if child, ok := values[nested].(map[string]any); ok {
			if found := firstString(child, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func rawString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text, isString := value.(string); isString {
				return text
			}
		}
	}
	for _, nested := range []string{"data", "delta"} {
		if child, ok := values[nested].(map[string]any); ok {
			if found := rawString(child, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func randomUUID() string {
	buffer := make([]byte, 16)
	if _, errRead := rand.Read(buffer); errRead != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buffer[0:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:16])
}

func messageText(message map[string]any) string {
	if text, ok := message["content"].(string); ok {
		return text
	}
	if parts, ok := message["content"].([]any); ok {
		var builder strings.Builder
		for _, part := range parts {
			if item, isMap := part.(map[string]any); isMap {
				if text := firstString(item, "text", "content"); text != "" {
					builder.WriteString(text)
				}
			} else if text, isString := part.(string); isString {
				builder.WriteString(text)
			}
		}
		return builder.String()
	}
	if message["content"] == nil {
		if toolCalls, ok := message["tool_calls"].([]any); ok && len(toolCalls) > 0 {
			encoded, _ := json.Marshal(toolCalls)
			return string(encoded)
		}
	}
	return ""
}

func mapFinishReason(value string) string {
	switch value {
	case "stop", "end_turn", "stop_sequence", "complete":
		return "stop"
	case "length", "max_tokens", "max_output_tokens":
		return "length"
	case "tool_calls", "tool_use", "tool-calls":
		return "tool_calls"
	case "content_filter":
		return "content_filter"
	default:
		return "stop"
	}
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
