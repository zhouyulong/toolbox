package toolbox

import "context"

const SchemaVersion = 1

type Provider struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	BaseURL        string   `json:"baseUrl"`
	APIKey         string   `json:"apiKey,omitempty"`
	Models         []string `json:"models"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

// ProviderView deliberately never exposes credentials to the webview.
type ProviderView struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	BaseURL        string   `json:"baseUrl"`
	HasAPIKey      bool     `json:"hasApiKey"`
	Models         []string `json:"models"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

type ProviderInput struct {
	Provider
	ClearAPIKey bool `json:"clearApiKey"`
}

type ToolRef struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type Config struct {
	SchemaVersion int        `json:"schemaVersion"`
	Providers     []Provider `json:"providers"`
	Tools         []ToolRef  `json:"tools"`
}

type Tool struct {
	SchemaVersion   int     `json:"schemaVersion"`
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	System          string  `json:"system"`
	ProviderID      string  `json:"providerId"`
	Model           string  `json:"model"`
	Temperature     float64 `json:"temperature"`
	SendTemperature bool    `json:"sendTemperature"`
	TokenParameter  string  `json:"tokenParameter"`
	MaxTokens       int     `json:"maxTokens"`
	HistoryTurns    int     `json:"historyTurns"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	// Path belongs to the local registry; the workspace file is portable.
	Path string `json:"path,omitempty"`
}

type Message struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	CreatedAt  string `json:"createdAt"`
	Error      string `json:"error,omitempty"`
	ProviderID string `json:"providerId,omitempty"`
	Model      string `json:"model,omitempty"`
}

type Session struct {
	SchemaVersion int       `json:"schemaVersion"`
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	CreatedAt     string    `json:"createdAt"`
	UpdatedAt     string    `json:"updatedAt"`
	Messages      []Message `json:"messages"`
}

type SessionSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	UpdatedAt    string `json:"updatedAt"`
	MessageCount int    `json:"messageCount"`
}

type ToolSummary struct {
	Tool
	LoadError string `json:"loadError,omitempty"`
}

type State struct {
	Version             string         `json:"version"`
	ConfigDir           string         `json:"configDir"`
	DefaultWorkspaceDir string         `json:"defaultWorkspaceDir"`
	Providers           []ProviderView `json:"providers"`
	Tools               []ToolSummary  `json:"tools"`
}

type Delta struct {
	RequestID string `json:"requestId"`
	ToolID    string `json:"toolId"`
	SessionID string `json:"sessionId"`
	Text      string `json:"text"`
}

type ChatResult struct {
	Session   Session `json:"session"`
	Error     string  `json:"error,omitempty"`
	Cancelled bool    `json:"cancelled"`
}

type activeRequest struct {
	ID     string
	Cancel context.CancelFunc
}
