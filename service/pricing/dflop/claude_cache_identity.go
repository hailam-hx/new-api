package dflop

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// ClaudeCacheSnapshot is a private forensic snapshot, not Anthropic's internal
// cache key. Payload contains prompt text and must never enter public logs.
type ClaudeCacheSnapshot struct {
	Hash      string            `json:"hash"`
	Payload   []byte            `json:"-"`
	TextBytes map[string]string `json:"-"`
}

// ClaudeCacheIdentity compares explicit cache prefixes without normalizing text.
// It retains unknown request controls conservatively; differing snapshots do not
// prove a provider cache miss. Automatic caching and malformed captures fail closed.
func ClaudeCacheIdentity(body []byte, headers http.Header) (ClaudeCacheSnapshot, error) {
	if !utf8.Valid(body) {
		return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: invalid UTF-8")
	}
	decoded, err := decodeClaudeCacheJSON(body)
	if err != nil {
		return ClaudeCacheSnapshot{}, err
	}
	request, ok := decoded.(map[string]any)
	if !ok {
		return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: request object")
	}
	if model, ok := request["model"].(string); !ok || model == "" {
		return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: model")
	}
	if headers.Get("anthropic-version") == "" {
		return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: anthropic-version")
	}
	if _, automatic := request["cache_control"]; automatic {
		return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: automatic caching")
	}
	nodes := []map[string]any{}
	for _, level := range []string{"tools", "system"} {
		value, exists := request[level]
		if !exists {
			continue
		}
		if text, ok := value.(string); ok && level == "system" {
			nodes = append(nodes, map[string]any{"path": "system", "value": text})
			continue
		}
		blocks, ok := value.([]any)
		if !ok {
			return ClaudeCacheSnapshot{}, fmt.Errorf("INSUFFICIENT_CAPTURE: %s", level)
		}
		for i, block := range blocks {
			if _, ok := block.(map[string]any); !ok {
				return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: block")
			}
			nodes = append(nodes, map[string]any{"path": fmt.Sprintf("%s[%d]", level, i), "value": block})
		}
	}
	messages, ok := request["messages"].([]any)
	if !ok {
		return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: messages")
	}
	for i, value := range messages {
		message, ok := value.(map[string]any)
		if !ok {
			return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: message")
		}
		metadata := map[string]any{}
		for key, value := range message {
			if key != "content" {
				metadata[key] = value
			}
		}
		switch content := message["content"].(type) {
		case string:
			nodes = append(nodes, map[string]any{"path": fmt.Sprintf("messages[%d].content", i), "message": metadata, "value": content})
		case []any:
			for j, block := range content {
				if _, ok := block.(map[string]any); !ok {
					return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: message block")
				}
				nodes = append(nodes, map[string]any{"path": fmt.Sprintf("messages[%d].content[%d]", i, j), "message": metadata, "value": block})
			}
		default:
			return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: content")
		}
	}
	breakpoint := -1
	for i, node := range nodes {
		if block, ok := node["value"].(map[string]any); ok {
			if control, present := block["cache_control"]; present {
				c, ok := control.(map[string]any)
				if !ok || c["type"] != "ephemeral" {
					return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: cache_control")
				}
				breakpoint = i
			}
		}
	}
	if breakpoint < 0 {
		return ClaudeCacheSnapshot{}, errors.New("INSUFFICIENT_CAPTURE: explicit breakpoint")
	}
	controls := map[string]any{}
	for key, value := range request {
		switch key {
		case "tools", "system", "messages", "max_tokens", "stream":
			// max_tokens controls generated output; stream controls response transport.
		default:
			controls[key] = value
		}
	}
	// Presence of images/citations anywhere can alter provider-generated prompts,
	// even when the user content itself is after the explicit breakpoint.
	features := map[string]any{}
	collectClaudeCacheFacts(request, "request", nil, features)
	texts := map[string]string{}
	for _, node := range nodes[:breakpoint+1] {
		collectClaudeCacheFacts(node["value"], node["path"].(string), texts, nil)
	}
	payload, err := common.Marshal(map[string]any{"version": 1, "controls": controls, "prefix": nodes[:breakpoint+1], "prompt_features": features, "headers": map[string]any{"anthropic-version": headers.Values("anthropic-version"), "anthropic-beta": headers.Values("anthropic-beta")}})
	if err != nil {
		return ClaudeCacheSnapshot{}, err
	}
	return ClaudeCacheSnapshot{Hash: fmt.Sprintf("%x", sha256.Sum256(payload)), Payload: payload, TextBytes: texts}, nil
}

// collectClaudeCacheFacts preserves decoded UTF-8 text bytes and records prompt
// feature switches across both cached blocks and the uncached suffix.
func collectClaudeCacheFacts(value any, path string, texts map[string]string, features map[string]any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if texts != nil && key == "text" {
				if text, ok := child.(string); ok {
					texts[path+".text"] = text
				}
			}
			if features != nil {
				if key == "type" && child == "image" {
					features["image_present"] = true
				}
				if key == "citations" {
					features[path+".citations"] = child
				}
			}
			collectClaudeCacheFacts(child, path+"."+key, texts, features)
		}
	case []any:
		for i, child := range v {
			collectClaudeCacheFacts(child, fmt.Sprintf("%s[%d]", path, i), texts, features)
		}
	case string:
		if texts != nil && (path == "system" || len(path) >= 8 && path[len(path)-8:] == ".content") {
			texts[path] = v
		}
	}
}

// decodeClaudeCacheJSON retains numeric literals so unknown controls cannot
// collapse to the same float64 value during forensic comparison.
func decodeClaudeCacheJSON(raw common.RawMessage) (any, error) {
	switch common.GetJsonType(raw) {
	case "object":
		var values map[string]common.RawMessage
		if err := common.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		result := map[string]any{}
		for key, value := range values {
			decoded, err := decodeClaudeCacheJSON(value)
			if err != nil {
				return nil, err
			}
			result[key] = decoded
		}
		return result, nil
	case "array":
		var values []common.RawMessage
		if err := common.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		result := make([]any, len(values))
		for i, value := range values {
			decoded, err := decodeClaudeCacheJSON(value)
			if err != nil {
				return nil, err
			}
			result[i] = decoded
		}
		return result, nil
	case "number":
		var value json.Number
		err := common.Unmarshal(raw, &value)
		return value, err
	default:
		var value any
		err := common.Unmarshal(raw, &value)
		return value, err
	}
}
