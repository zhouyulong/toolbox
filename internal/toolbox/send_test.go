package toolbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendIncludesOnlyGrantedSkillAcrossProtocols(t *testing.T) {
	for _, kind := range []string{"openai", "anthropic", "gemini"} {
		t.Run(kind, func(t *testing.T) {
			payloads := make(chan map[string]interface{}, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				payloads <- body
				w.Header().Set("Content-Type", "text/event-stream")
				switch kind {
				case "openai":
					w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"))
				case "anthropic":
					w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"))
				case "gemini":
					w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}]}\n\n"))
				}
			}))
			defer server.Close()
			s := testService(t)
			a := saveTestSkill(t, s, "Granted", "GRANTED_TEXT")
			saveTestSkill(t, s, "Unassigned", "UNASSIGNED_SECRET")
			tool := createTestTool(t, s, a.ID)
			tool.System = "BASE_RULE"
			tool.ProviderID = "provider"
			tool.SendReasoningEffort = true
			tool.ReasoningEffort = "low"
			tool.HistoryTurns = 3
			switch kind {
			case "anthropic":
				tool.Model = "claude-sonnet-4-6"
			case "gemini":
				tool.Model = "gemini-3-flash-preview"
			}
			s.config.Providers = []Provider{{ID: "provider", Kind: kind, BaseURL: server.URL, TimeoutSeconds: 10}}
			var err error
			tool, err = s.SaveTool(tool)
			if err != nil {
				t.Fatal(err)
			}
			session, err := s.NewSession(tool.ID)
			if err != nil {
				t.Fatal(err)
			}
			for turn := 0; turn < 2; turn++ {
				var delta strings.Builder
				result, err := s.Send(context.Background(), tool.ID, session.ID, "question", strings.Repeat("e", 32), func(d Delta) { delta.WriteString(d.Text) })
				if err != nil || result.Error != "" {
					t.Fatalf("send failed: %v %s", err, result.Error)
				}
				if delta.String() != "ok" || result.Session.Messages[len(result.Session.Messages)-1].Content != "ok" {
					t.Fatal("stream not saved")
				}
				body := <-payloads
				data, _ := json.Marshal(body)
				if strings.Contains(string(data), "UNASSIGNED_SECRET") {
					t.Fatal("unauthorized skill entered request")
				}
				var system string
				switch kind {
				case "openai":
					system = body["messages"].([]interface{})[0].(map[string]interface{})["content"].(string)
				case "anthropic":
					system = body["system"].(string)
				case "gemini":
					system = body["systemInstruction"].(map[string]interface{})["parts"].([]interface{})[0].(map[string]interface{})["text"].(string)
				}
				if turn == 0 && !strings.Contains(system, "GRANTED_TEXT") {
					t.Fatalf("granted skill missing from %s: %q", kind, system)
				}
				if turn == 1 && system != "BASE_RULE" {
					t.Fatalf("revoked skill still loaded: %q", system)
				}
				saved, err := s.tool(tool.ID)
				if err != nil {
					t.Fatal(err)
				}
				if saved.System != "BASE_RULE" {
					t.Fatal("original system overwritten")
				}
				for _, message := range result.Session.Messages {
					if strings.Contains(message.Content, "GRANTED_TEXT") {
						t.Fatal("skill stored as chat message")
					}
				}
				tool.SkillIDs = nil
				if _, err = s.SaveTool(tool); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
