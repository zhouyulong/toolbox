package toolbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func toolFromJSON(t *testing.T, extra string) Tool {
	t.Helper()
	var tool Tool
	data := `{"name":"test","model":"test-model","temperature":0.7,"sendTemperature":false,"tokenParameter":"max_tokens","maxTokens":16384,"historyTurns":0` + extra + `}`
	if err := json.Unmarshal([]byte(data), &tool); err != nil {
		t.Fatal(err)
	}
	return tool
}

func TestReasoningParametersReachProvider(t *testing.T) {
	for _, kind := range []string{"openai", "anthropic", "gemini"} {
		for _, effort := range []string{"low", "medium", "high"} {
			t.Run(kind+"/"+effort, func(t *testing.T) {
				var got map[string]any
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Error(err)
					}
					w.Header().Set("Content-Type", "application/json")
					switch kind {
					case "openai":
						w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
					case "anthropic":
						w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
					case "gemini":
						w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
					}
				}))
				defer server.Close()
				tool := toolFromJSON(t, `,"sendReasoningEffort":true,"reasoningEffort":"`+effort+`"`)
				switch kind {
				case "anthropic":
					tool.Model = "claude-sonnet-4-6"
				case "gemini":
					tool.Model = "gemini-3-flash-preview"
				}
				var output strings.Builder
				if err := streamCompletion(context.Background(), Provider{Kind: kind, BaseURL: server.URL, TimeoutSeconds: 10}, tool, []Message{{Role: "user", Content: "hello"}}, func(s string) { output.WriteString(s) }); err != nil {
					t.Fatal(err)
				}
				if output.String() != "ok" {
					t.Fatalf("output=%q", output.String())
				}
				switch kind {
				case "openai":
					if got["reasoning_effort"] != effort {
						t.Fatalf("missing effort: %#v", got)
					}
				case "anthropic":
					effortCfg, _ := got["output_config"].(map[string]any)
					thinking, _ := got["thinking"].(map[string]any)
					if effortCfg["effort"] != effort || thinking["type"] != "adaptive" {
						t.Fatalf("missing adaptive effort: %#v", got)
					}
				case "gemini":
					generation, _ := got["generationConfig"].(map[string]any)
					thinking, _ := generation["thinkingConfig"].(map[string]any)
					if thinking["thinkingLevel"] != effort || generation["maxOutputTokens"] != float64(16384) {
						t.Fatalf("missing effort or token limit: %#v", got)
					}
				}
			})
		}
	}
}

func TestDisabledReasoningPreservesRequest(t *testing.T) {
	for _, kind := range []string{"openai", "anthropic", "gemini"} {
		t.Run(kind, func(t *testing.T) {
			var got map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewDecoder(r.Body).Decode(&got)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{}`))
			}))
			defer server.Close()
			tool := toolFromJSON(t, `,"sendReasoningEffort":false,"reasoningEffort":"high"`)
			if err := streamCompletion(context.Background(), Provider{Kind: kind, BaseURL: server.URL, TimeoutSeconds: 10}, tool, []Message{{Role: "user", Content: "hello"}}, func(string) {}); err != nil {
				t.Fatal(err)
			}
			if got["reasoning_effort"] != nil || got["thinking"] != nil || got["output_config"] != nil {
				t.Fatalf("unexpected reasoning: %#v", got)
			}
			if gen, ok := got["generationConfig"].(map[string]any); ok && gen["thinkingConfig"] != nil {
				t.Fatalf("unexpected thinking config: %#v", got)
			}
		})
	}
}

func TestReasoningRejectsInvalidEffort(t *testing.T) {
	for _, extra := range []string{`,"sendReasoningEffort":true`, `,"sendReasoningEffort":true,"reasoningEffort":"extreme"`} {
		if err := validateTool(toolFromJSON(t, extra)); err == nil {
			t.Fatal("invalid effort accepted")
		}
	}
	if err := validateTool(toolFromJSON(t, "")); err != nil {
		t.Fatalf("legacy tool rejected: %v", err)
	}
}

func TestGemini25BudgetsAndAnthropicTemperatureConflict(t *testing.T) {
	for _, tc := range []struct {
		effort string
		budget int
	}{{"low", 1024}, {"medium", 4096}, {"high", 8192}} {
		tool := toolFromJSON(t, `,"sendReasoningEffort":true,"reasoningEffort":"`+tc.effort+`"`)
		tool.Model = "models/gemini-2.5-pro"
		body := map[string]interface{}{"generationConfig": map[string]interface{}{"maxOutputTokens": 16384}}
		if err := applyReasoning(Provider{Kind: "gemini"}, tool, body); err != nil {
			t.Fatal(err)
		}
		gen := body["generationConfig"].(map[string]interface{})
		thinking := gen["thinkingConfig"].(map[string]interface{})
		if thinking["thinkingBudget"] != tc.budget || thinking["thinkingLevel"] != nil || gen["maxOutputTokens"] != 16384 {
			t.Fatalf("wrong budget mapping: %#v", body)
		}
		tool.MaxTokens = tc.budget
		if err := applyReasoning(Provider{Kind: "gemini"}, tool, map[string]interface{}{}); err == nil {
			t.Fatal("budget leaves no output tokens")
		}
	}
	tool := toolFromJSON(t, `,"sendReasoningEffort":true,"reasoningEffort":"medium"`)
	tool.SendTemperature = true
	if err := applyReasoning(Provider{Kind: "anthropic"}, tool, map[string]interface{}{}); err == nil {
		t.Fatal("incompatible temperature accepted")
	}
}

func TestProviderRejectionIsSurfacedWithoutRetry(t *testing.T) {
	requests := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"unsupported reasoning_effort"}`))
	}))
	defer server.Close()
	tool := toolFromJSON(t, `,"sendReasoningEffort":true,"reasoningEffort":"high"`)
	err := streamCompletion(context.Background(), Provider{Kind: "openai", BaseURL: server.URL, TimeoutSeconds: 10}, tool, []Message{{Role: "user", Content: "hi"}}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "unsupported reasoning_effort") {
		t.Fatalf("provider rejection hidden: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("retried %d times", len(requests))
	}
}
