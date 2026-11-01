package toolbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Only completed, successful history pairs are included. Failed or interrupted
// outputs remain visible locally but never become hidden context for a retry.
func history(messages []Message, turns int) []wireMessage {
	pairs := [][2]wireMessage{}
	for i := 0; i+1 < len(messages)-1; i++ {
		u, a := messages[i], messages[i+1]
		if u.Role == "user" && a.Role == "assistant" && a.Error == "" && a.Content != "" {
			pairs = append(pairs, [2]wireMessage{{Role: "user", Content: u.Content}, {Role: "assistant", Content: a.Content}})
			i++
		}
	}
	result := []wireMessage{}
	start := max(0, len(pairs)-turns)
	for _, pair := range pairs[start:] {
		result = append(result, pair[0], pair[1])
	}
	if len(messages) > 0 {
		result = append(result, wireMessage{Role: "user", Content: messages[len(messages)-1].Content})
	}
	return result
}

func redact(text, key string) string {
	if key != "" {
		text = strings.ReplaceAll(text, key, "[API KEY]")
		text = strings.ReplaceAll(text, url.QueryEscape(key), "[API KEY]")
	}
	return text
}

func streamCompletion(ctx context.Context, p Provider, t Tool, messages []Message, emit func(string)) error {
	msgs := history(messages, t.HistoryTurns)
	endpoint := ""
	body := map[string]interface{}{}
	switch p.Kind {
	case "openai":
		endpoint = p.BaseURL + "/chat/completions"
		if strings.TrimSpace(t.System) != "" {
			msgs = append([]wireMessage{{Role: "system", Content: t.System}}, msgs...)
		}
		body = map[string]interface{}{"model": t.Model, "messages": msgs, "stream": true, t.TokenParameter: t.MaxTokens}
		if t.SendTemperature {
			body["temperature"] = t.Temperature
		}
	case "anthropic":
		endpoint = p.BaseURL + "/messages"
		body = map[string]interface{}{"model": t.Model, "messages": msgs, "max_tokens": t.MaxTokens, "stream": true}
		if t.System != "" {
			body["system"] = t.System
		}
		if t.SendTemperature {
			body["temperature"] = t.Temperature
		}
	case "gemini":
		model := strings.TrimPrefix(t.Model, "models/")
		endpoint = p.BaseURL + "/models/" + url.PathEscape(model) + ":streamGenerateContent?alt=sse"
		contents := []interface{}{}
		for _, m := range msgs {
			role := m.Role
			if role == "assistant" {
				role = "model"
			}
			contents = append(contents, map[string]interface{}{"role": role, "parts": []interface{}{map[string]string{"text": m.Content}}})
		}
		generation := map[string]interface{}{"maxOutputTokens": t.MaxTokens}
		if t.SendTemperature {
			generation["temperature"] = t.Temperature
		}
		body = map[string]interface{}{"contents": contents, "generationConfig": generation}
		if t.System != "" {
			body["systemInstruction"] = map[string]interface{}{"parts": []interface{}{map[string]string{"text": t.System}}}
		}
	default:
		return errors.New("不支持的供应商协议")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	switch p.Kind {
	case "openai":
		if p.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.APIKey)
		}
	case "anthropic":
		req.Header.Set("anthropic-version", "2023-06-01")
		if p.APIKey != "" {
			req.Header.Set("x-api-key", p.APIKey)
		}
	case "gemini":
		if p.APIKey != "" {
			req.Header.Set("x-goog-api-key", p.APIKey)
		}
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: time.Duration(p.TimeoutSeconds) * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Duration(p.TimeoutSeconds) * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("供应商返回重定向，请直接配置最终 Base URL")
	}}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("供应商 HTTP %d：%s", resp.StatusCode, redact(strings.TrimSpace(string(b)), p.APIKey))
	}
	outputBytes := 0
	limitedEmit := func(s string) error {
		outputBytes += len(s)
		if outputBytes > 32<<20 {
			return errors.New("输出已超过本地 32 MiB 限制，请新建会话或减少输出长度")
		}
		if s != "" {
			emit(s)
		}
		return nil
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		b, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
		if err != nil {
			return err
		}
		if len(b) > 32<<20 {
			return errors.New("响应超过 32 MiB 限制")
		}
		_, err = consumeJSON(p.Kind, b, false, limitedEmit)
		return err
	}
	finished := false
	streamDone := errors.New("stream completed")
	err = readSSE(resp.Body, func(data string) error {
		if strings.TrimSpace(data) == "[DONE]" {
			finished = true
			return streamDone
		}
		done, err := consumeJSON(p.Kind, []byte(data), true, limitedEmit)
		if done {
			finished = true
		}
		if err == nil && done {
			return streamDone
		}
		return err
	})
	if errors.Is(err, streamDone) {
		err = nil
	}
	if err != nil {
		return fmt.Errorf("响应中断：%w", err)
	}
	if !finished {
		return errors.New("响应流提前结束，未收到完成标记；已保留收到的文本")
	}
	return nil
}

func readSSE(r io.Reader, handle func(string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	var parts []string
	eventBytes := 0
	flush := func() error {
		if len(parts) == 0 {
			return nil
		}
		data := strings.Join(parts, "\n")
		parts = nil
		eventBytes = 0
		return handle(data)
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			eventBytes += len(line)
			if eventBytes > 16<<20 {
				return errors.New("单个 SSE 事件超过 16 MiB 限制")
			}
			parts = append(parts, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

func consumeJSON(kind string, data []byte, stream bool, emit func(string) error) (bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return false, fmt.Errorf("供应商返回了无效 JSON：%w", err)
	}
	if raw, ok := root["error"]; ok && string(raw) != "null" {
		return false, fmt.Errorf("供应商错误：%s", string(raw))
	}
	switch kind {
	case "openai":
		var v struct {
			Choices []struct {
				Delta struct {
					Content json.RawMessage `json:"content"`
				} `json:"delta"`
				Message struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(data, &v); err != nil {
			return false, err
		}
		if len(v.Choices) == 0 {
			return false, nil
		}
		c := v.Choices[0]
		content := c.Delta.Content
		if !stream {
			content = c.Message.Content
		}
		if err := emitContent(content, emit); err != nil {
			return false, err
		}
		switch c.FinishReason {
		case "length":
			return true, errors.New("已达到输出 token 上限，回复可能不完整")
		case "content_filter":
			return true, errors.New("供应商内容过滤中止了回复")
		case "tool_calls", "function_call":
			return true, errors.New("模型返回了工具调用；本应用只支持文本问答，请通过 system 要求直接输出文本")
		}
		return c.FinishReason != "" || !stream, nil
	case "anthropic":
		var v struct {
			Type  string `json:"type"`
			Delta struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			ContentBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content_block"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			StopReason string `json:"stop_reason"`
		}
		if err := json.Unmarshal(data, &v); err != nil {
			return false, err
		}
		if v.Type == "content_block_delta" && v.Delta.Type == "text_delta" {
			if err := emit(v.Delta.Text); err != nil {
				return false, err
			}
		}
		if v.Type == "content_block_start" && v.ContentBlock.Type == "text" {
			if err := emit(v.ContentBlock.Text); err != nil {
				return false, err
			}
		}
		if !stream {
			for _, c := range v.Content {
				if c.Type == "text" {
					if err := emit(c.Text); err != nil {
						return false, err
					}
				}
			}
		}
		stop := v.Delta.StopReason
		if !stream {
			stop = v.StopReason
		}
		if stop == "max_tokens" {
			return true, errors.New("已达到输出 token 上限，回复可能不完整")
		}
		if stop == "refusal" {
			return true, errors.New("供应商拒绝了该请求")
		}
		return v.Type == "message_stop" || !stream, nil
	case "gemini":
		var v struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text    string `json:"text"`
						Thought bool   `json:"thought"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			PromptFeedback struct {
				BlockReason string `json:"blockReason"`
			} `json:"promptFeedback"`
		}
		if err := json.Unmarshal(data, &v); err != nil {
			return false, err
		}
		if v.PromptFeedback.BlockReason != "" {
			return true, fmt.Errorf("供应商拒绝了提示词：%s", v.PromptFeedback.BlockReason)
		}
		if len(v.Candidates) == 0 {
			return false, nil
		}
		c := v.Candidates[0]
		for _, part := range c.Content.Parts {
			if !part.Thought {
				if err := emit(part.Text); err != nil {
					return false, err
				}
			}
		}
		if c.FinishReason != "" && c.FinishReason != "STOP" {
			return true, fmt.Errorf("供应商中止回复（%s），文本可能不完整", c.FinishReason)
		}
		return c.FinishReason != "" || !stream, nil
	}
	return false, errors.New("不支持的响应协议")
}

func emitContent(raw json.RawMessage, emit func(string) error) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return emit(text)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return errors.New("模型返回了不支持的文本格式")
	}
	for _, p := range parts {
		if p.Type == "text" || p.Type == "output_text" {
			if err := emit(p.Text); err != nil {
				return err
			}
		}
	}
	return nil
}
