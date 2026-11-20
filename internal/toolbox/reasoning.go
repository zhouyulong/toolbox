package toolbox

import (
	"errors"
	"fmt"
	"strings"
)

func validateReasoning(t Tool) error {
	if t.ReasoningEffort == "" && !t.SendReasoningEffort {
		return nil
	}
	switch t.ReasoningEffort {
	case "low", "medium", "high":
		return nil
	default:
		return errors.New("思考强度请选择低、中或高")
	}
}

// Only explicit opt-in changes requests. Compatibility errors are surfaced;
// parameters are never silently dropped and requests are never retried.
func applyReasoning(p Provider, t Tool, body map[string]interface{}) error {
	if err := validateReasoning(t); err != nil {
		return err
	}
	if !t.SendReasoningEffort {
		return nil
	}
	switch p.Kind {
	case "openai":
		body["reasoning_effort"] = t.ReasoningEffort
	case "anthropic":
		if t.SendTemperature {
			return errors.New("Anthropic 自适应思考不能同时指定 temperature，请先关闭发送 temperature 参数")
		}
		body["thinking"] = map[string]interface{}{"type": "adaptive"}
		body["output_config"] = map[string]interface{}{"effort": t.ReasoningEffort}
	case "gemini":
		thinking := map[string]interface{}{"thinkingLevel": t.ReasoningEffort}
		// Gemini 2.5 uses budgets rather than the level field accepted by Gemini 3.
		if strings.HasPrefix(strings.TrimPrefix(t.Model, "models/"), "gemini-2.5-") {
			budget := map[string]int{"low": 1024, "medium": 4096, "high": 8192}[t.ReasoningEffort]
			if t.MaxTokens <= budget {
				return fmt.Errorf("当前思考档位需要 %d token 预算，请将最大输出 token 设为大于 %d，或降低思考强度", budget, budget)
			}
			thinking = map[string]interface{}{"thinkingBudget": budget}
		}
		generation, _ := body["generationConfig"].(map[string]interface{})
		if generation == nil {
			generation = map[string]interface{}{}
			body["generationConfig"] = generation
		}
		generation["thinkingConfig"] = thinking
	default:
		return errors.New("当前协议不支持指定思考强度")
	}
	return nil
}
