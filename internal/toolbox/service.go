package toolbox

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
)

type Service struct {
	mu           sync.Mutex
	config       Config
	configDir    string
	workspaceDir string
	version      string
	active       map[string]activeRequest
	wg           sync.WaitGroup
	closed       bool
}

func NewService(version string) (*Service, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "ToolboxSelf")
	c, err := loadConfig(dir)
	if err != nil {
		return nil, err
	}
	return &Service{config: c, configDir: dir, workspaceDir: filepath.Join(home, "ToolboxSelf"), version: version, active: map[string]activeRequest{}}, nil
}

func (s *Service) persist(c Config) error {
	if err := writeJSON(filepath.Join(s.configDir, "config.json"), c); err != nil {
		return err
	}
	s.config = c
	return nil
}

func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := State{Version: s.version, ConfigDir: s.configDir, DefaultWorkspaceDir: s.workspaceDir, Providers: []ProviderView{}, Tools: []ToolSummary{}}
	for _, p := range s.config.Providers {
		state.Providers = append(state.Providers, ProviderView{ID: p.ID, Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, HasAPIKey: p.APIKey != "", Models: p.Models, TimeoutSeconds: p.TimeoutSeconds})
	}
	for _, ref := range s.config.Tools {
		t, err := loadTool(ref)
		if err != nil {
			state.Tools = append(state.Tools, ToolSummary{Tool: Tool{ID: ref.ID, Path: ref.Path, Name: filepath.Base(ref.Path)}, LoadError: err.Error()})
			continue
		}
		state.Tools = append(state.Tools, ToolSummary{Tool: t})
	}
	return state
}

func (s *Service) SaveProvider(input ProviderInput) (ProviderView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := input.Provider
	p.Name = strings.TrimSpace(p.Name)
	p.APIKey = strings.TrimSpace(p.APIKey)
	p.BaseURL = strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if p.Name == "" {
		return ProviderView{}, errors.New("请填写供应商名称")
	}
	if p.Kind != "openai" && p.Kind != "anthropic" && p.Kind != "gemini" {
		return ProviderView{}, errors.New("不支持的接口协议")
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ProviderView{}, errors.New("Base URL 必须是 http(s) 地址，不能包含凭据、查询参数或片段")
	}
	if p.TimeoutSeconds < 10 || p.TimeoutSeconds > 1800 {
		return ProviderView{}, errors.New("请求超时应为 10–1800 秒")
	}
	models := []string{}
	seen := map[string]bool{}
	for _, model := range p.Models {
		model = strings.TrimSpace(model)
		if model != "" && !seen[model] {
			models = append(models, model)
			seen[model] = true
		}
	}
	p.Models = models
	if len(models) == 0 {
		return ProviderView{}, errors.New("至少填写一个模型 ID")
	}
	c := s.config
	c.Providers = append([]Provider{}, s.config.Providers...)
	index := -1
	for i, existing := range c.Providers {
		if existing.ID == p.ID {
			index = i
			if p.APIKey == "" && !input.ClearAPIKey {
				p.APIKey = existing.APIKey
			}
			break
		}
	}
	if input.ClearAPIKey {
		p.APIKey = ""
	}
	if index < 0 {
		if p.ID != "" {
			return ProviderView{}, errors.New("供应商不存在，请重新打开设置")
		}
		p.ID, err = newID()
		if err != nil {
			return ProviderView{}, err
		}
		c.Providers = append(c.Providers, p)
	} else {
		c.Providers[index] = p
	}
	if err := s.persist(c); err != nil {
		return ProviderView{}, err
	}
	return ProviderView{ID: p.ID, Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, HasAPIKey: p.APIKey != "", Models: p.Models, TimeoutSeconds: p.TimeoutSeconds}, nil
}

func (s *Service) DeleteProvider(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ref := range s.config.Tools {
		t, err := loadTool(ref)
		if err != nil {
			return fmt.Errorf("先修复或移除不可读取的工作区：%w", err)
		}
		if t.ProviderID == id {
			return fmt.Errorf("工具「%s」正在使用该供应商，请先修改工具设置", t.Name)
		}
	}
	c := s.config
	c.Providers = []Provider{}
	for _, p := range s.config.Providers {
		if p.ID != id {
			c.Providers = append(c.Providers, p)
		}
	}
	return s.persist(c)
}

func (s *Service) tool(id string) (Tool, error) {
	for _, ref := range s.config.Tools {
		if ref.ID == id {
			return loadTool(ref)
		}
	}
	return Tool{}, errors.New("工具未登记")
}

func validateTool(t Tool) error {
	if strings.TrimSpace(t.Name) == "" || utf8.RuneCountInString(t.Name) > 80 {
		return errors.New("工具名称应为 1–80 个字符")
	}
	if len(t.System) > 1<<20 {
		return errors.New("system 提示词不能超过 1 MiB")
	}
	if t.Temperature < 0 || t.Temperature > 2 {
		return errors.New("temperature 应为 0–2")
	}
	if t.MaxTokens < 1 || t.MaxTokens > 128000 {
		return errors.New("最大输出 token 应为 1–128000")
	}
	if t.HistoryTurns < 0 || t.HistoryTurns > 100 {
		return errors.New("历史轮数应为 0–100；0 表示仅发送本次提问")
	}
	if t.TokenParameter != "max_tokens" && t.TokenParameter != "max_completion_tokens" {
		return errors.New("无效的 OpenAI token 参数名")
	}
	return nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func isWithin(path, root string) bool {
	if samePath(path, root) {
		return true
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		path = strings.ToLower(path)
		root = strings.ToLower(root)
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (s *Service) checkWorkspace(path string) error {
	if isWithin(path, s.configDir) || isWithin(s.configDir, path) {
		return errors.New("工作区不能与程序配置目录重叠")
	}
	for _, ref := range s.config.Tools {
		if isWithin(path, ref.Path) || isWithin(ref.Path, path) {
			return errors.New("每个工具必须使用独立目录，不能与已登记的工作区重叠")
		}
	}
	return nil
}

func (s *Service) CreateTool(t Tool) (Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateTool(t); err != nil {
		return Tool{}, err
	}
	id, err := newID()
	if err != nil {
		return Tool{}, err
	}
	if strings.TrimSpace(t.Path) == "" {
		t.Path = filepath.Join(s.workspaceDir, id)
	}
	t.Path, err = canonicalPath(t.Path)
	if err != nil {
		return Tool{}, err
	}
	if err := s.checkWorkspace(t.Path); err != nil {
		return Tool{}, err
	}
	entries, err := os.ReadDir(t.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Tool{}, err
	}
	if len(entries) > 0 {
		return Tool{}, errors.New("新工作区请选择空目录；已有工作区请使用「导入工作区」")
	}
	t.SchemaVersion = SchemaVersion
	t.ID = id
	t.Name = strings.TrimSpace(t.Name)
	t.CreatedAt = now()
	t.UpdatedAt = t.CreatedAt
	if err := saveTool(t); err != nil {
		return Tool{}, err
	}
	c := s.config
	c.Tools = append(append([]ToolRef{}, c.Tools...), ToolRef{ID: t.ID, Path: t.Path})
	if err := s.persist(c); err != nil {
		return Tool{}, fmt.Errorf("工作区已创建但登记失败，可稍后导入 %s：%w", t.Path, err)
	}
	return t, nil
}

func (s *Service) ImportTool(path string) (Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := canonicalPath(path)
	if err != nil {
		return Tool{}, err
	}
	if err := s.checkWorkspace(path); err != nil {
		return Tool{}, err
	}
	var manifest Tool
	if err := readJSON(filepath.Join(path, workspaceFile), &manifest); err != nil {
		return Tool{}, err
	}
	t, err := loadTool(ToolRef{ID: manifest.ID, Path: path})
	if err != nil {
		return Tool{}, err
	}
	if err := validateTool(t); err != nil {
		return Tool{}, err
	}
	for _, ref := range s.config.Tools {
		if ref.ID == t.ID {
			return Tool{}, errors.New("该工具 ID 已登记，不能同时导入同一工作区的副本")
		}
	}
	c := s.config
	c.Tools = append(append([]ToolRef{}, c.Tools...), ToolRef{ID: t.ID, Path: path})
	if err := s.persist(c); err != nil {
		return Tool{}, err
	}
	return t, nil
}

func (s *Service) SaveTool(input Tool) (Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tool(input.ID)
	if err != nil {
		return t, err
	}
	if _, busy := s.active[t.ID]; busy {
		return t, errors.New("请先停止该工具的请求，再修改设置")
	}
	if err := validateTool(input); err != nil {
		return t, err
	}
	t.Name = strings.TrimSpace(input.Name)
	t.Description = input.Description
	t.System = input.System
	t.ProviderID = input.ProviderID
	t.Model = strings.TrimSpace(input.Model)
	t.Temperature = input.Temperature
	t.MaxTokens = input.MaxTokens
	t.HistoryTurns = input.HistoryTurns
	t.UpdatedAt = now()
	t.SendTemperature = input.SendTemperature
	t.TokenParameter = input.TokenParameter
	return t, saveTool(t)
}

// ForgetTool unregisters a path and never deletes workspace data.
func (s *Service) ForgetTool(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.active[id]; busy {
		return errors.New("请先停止该工具的请求")
	}
	c := s.config
	c.Tools = []ToolRef{}
	for _, ref := range s.config.Tools {
		if ref.ID != id {
			c.Tools = append(c.Tools, ref)
		}
	}
	return s.persist(c)
}

func (s *Service) ListSessions(toolID string) ([]SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tool(toolID)
	if err != nil {
		return nil, err
	}
	return listSessions(t)
}

func (s *Service) GetSession(toolID, id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tool(toolID)
	if err != nil {
		return Session{}, err
	}
	return readSession(t, id)
}

func (s *Service) NewSession(toolID string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.tool(toolID)
	if err != nil {
		return Session{}, err
	}
	id, err := newID()
	if err != nil {
		return Session{}, err
	}
	stamp := now()
	chat := Session{SchemaVersion: SchemaVersion, ID: id, Title: "新会话", CreatedAt: stamp, UpdatedAt: stamp, Messages: []Message{}}
	return chat, saveSession(t, chat)
}

func (s *Service) RenameSession(toolID, id, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.active[toolID]; busy {
		return errors.New("请先停止该工具的请求")
	}
	t, err := s.tool(toolID)
	if err != nil {
		return err
	}
	chat, err := readSession(t, id)
	if err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("标题不能为空")
	}
	chat.Title = string([]rune(title)[:min(len([]rune(title)), 120)])
	chat.UpdatedAt = now()
	return saveSession(t, chat)
}

func (s *Service) DeleteSession(toolID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.active[toolID]; busy {
		return errors.New("请先停止该工具的请求")
	}
	t, err := s.tool(toolID)
	if err != nil {
		return err
	}
	path, err := sessionPath(t, id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *Service) Cancel(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.active {
		if r.ID == requestID {
			r.Cancel()
		}
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	for _, r := range s.active {
		r.Cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) Send(ctx context.Context, toolID, sessionID, content, requestID string, emit func(Delta)) (ChatResult, error) {
	if strings.TrimSpace(content) == "" {
		return ChatResult{}, errors.New("提问不能为空")
	}
	if len(content) > 4<<20 {
		return ChatResult{}, errors.New("单次提问不能超过 4 MiB")
	}
	if !validID.MatchString(requestID) {
		return ChatResult{}, errors.New("无效的请求 ID")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ChatResult{}, errors.New("应用正在退出")
	}
	t, err := s.tool(toolID)
	if err != nil {
		s.mu.Unlock()
		return ChatResult{}, err
	}
	if _, busy := s.active[toolID]; busy {
		s.mu.Unlock()
		return ChatResult{}, errors.New("该工具已有正在进行的请求")
	}
	var p Provider
	found := false
	for _, candidate := range s.config.Providers {
		if candidate.ID == t.ProviderID {
			p = candidate
			found = true
			break
		}
	}
	if !found || t.Model == "" {
		s.mu.Unlock()
		return ChatResult{}, errors.New("请在工具设置中选择供应商并填写模型 ID")
	}
	chat, err := readSession(t, sessionID)
	if err != nil {
		s.mu.Unlock()
		return ChatResult{}, err
	}
	userID, err := newID()
	if err != nil {
		s.mu.Unlock()
		return ChatResult{}, err
	}
	assistantID, err := newID()
	if err != nil {
		s.mu.Unlock()
		return ChatResult{}, err
	}
	chat.Messages = append(chat.Messages, Message{ID: userID, Role: "user", Content: content, CreatedAt: now()})
	if len(chat.Messages) == 1 {
		title := []rune(strings.Join(strings.Fields(content), " "))
		chat.Title = string(title[:min(len(title), 36)])
	}
	chat.UpdatedAt = now()
	if err := saveSession(t, chat); err != nil {
		s.mu.Unlock()
		return ChatResult{}, err
	}
	requestCtx, cancel := context.WithCancel(ctx)
	s.active[toolID] = activeRequest{ID: requestID, Cancel: cancel}
	s.wg.Add(1)
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); delete(s.active, toolID); s.mu.Unlock(); s.wg.Done() }()

	var output strings.Builder
	err = streamCompletion(requestCtx, p, t, chat.Messages, func(chunk string) {
		output.WriteString(chunk)
		if emit != nil {
			emit(Delta{RequestID: requestID, ToolID: toolID, SessionID: sessionID, Text: chunk})
		}
	})
	result := ChatResult{}
	if err != nil {
		result.Cancelled = errors.Is(err, context.Canceled)
		if result.Cancelled {
			result.Error = "请求已停止"
		} else {
			result.Error = redact(err.Error(), p.APIKey)
		}
	}
	if output.Len() == 0 && err == nil {
		result.Error = "供应商未返回可显示的文本；请确认模型支持文本问答，或检查内容过滤及输出 token 设置"
	}
	chat.Messages = append(chat.Messages, Message{ID: assistantID, Role: "assistant", Content: output.String(), CreatedAt: now(), Error: result.Error, ProviderID: p.ID, Model: t.Model})
	chat.UpdatedAt = now()
	s.mu.Lock()
	saveErr := saveSession(t, chat)
	s.mu.Unlock()
	result.Session = chat
	if saveErr != nil {
		result.Error = strings.TrimSpace(result.Error + "；回复已收到，但保存失败：" + saveErr.Error())
	}
	return result, nil
}
