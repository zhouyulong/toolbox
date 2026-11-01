package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"toolboxself/internal/toolbox"
)

type App struct {
	ctx          context.Context
	service      *toolbox.Service
	startupError error
	version      string
	initialized  chan struct{}
}

func NewApp(version string) *App { return &App{version: version, initialized: make(chan struct{})} }
func (a *App) startup(ctx context.Context) {
	defer close(a.initialized)
	a.ctx = ctx
	a.service, a.startupError = toolbox.NewService(a.version)
}
func (a *App) shutdown(context.Context) {
	<-a.initialized
	if a.service != nil {
		a.service.Close()
	}
}
func (a *App) show() {
	<-a.initialized
	if a.ctx != nil {
		runtime.WindowUnminimise(a.ctx)
		runtime.WindowShow(a.ctx)
	}
}
func (a *App) ready() error {
	<-a.initialized
	if a.startupError != nil {
		return a.startupError
	}
	if a.service == nil {
		return errors.New("应用尚未初始化")
	}
	return nil
}

func (a *App) GetState() (toolbox.State, error) {
	if err := a.ready(); err != nil {
		return toolbox.State{}, err
	}
	return a.service.State(), nil
}
func (a *App) SaveProvider(p toolbox.ProviderInput) (toolbox.ProviderView, error) {
	if err := a.ready(); err != nil {
		return toolbox.ProviderView{}, err
	}
	return a.service.SaveProvider(p)
}
func (a *App) DeleteProvider(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.DeleteProvider(id)
}
func (a *App) CreateTool(t toolbox.Tool) (toolbox.Tool, error) {
	if err := a.ready(); err != nil {
		return toolbox.Tool{}, err
	}
	return a.service.CreateTool(t)
}
func (a *App) SaveTool(t toolbox.Tool) (toolbox.Tool, error) {
	if err := a.ready(); err != nil {
		return toolbox.Tool{}, err
	}
	return a.service.SaveTool(t)
}
func (a *App) ImportTool(path string) (toolbox.Tool, error) {
	if err := a.ready(); err != nil {
		return toolbox.Tool{}, err
	}
	return a.service.ImportTool(path)
}
func (a *App) ForgetTool(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.ForgetTool(id)
}
func (a *App) ListSessions(toolID string) ([]toolbox.SessionSummary, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListSessions(toolID)
}
func (a *App) GetSession(toolID, id string) (toolbox.Session, error) {
	if err := a.ready(); err != nil {
		return toolbox.Session{}, err
	}
	return a.service.GetSession(toolID, id)
}
func (a *App) NewSession(toolID string) (toolbox.Session, error) {
	if err := a.ready(); err != nil {
		return toolbox.Session{}, err
	}
	return a.service.NewSession(toolID)
}
func (a *App) RenameSession(toolID, id, title string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.RenameSession(toolID, id, title)
}
func (a *App) DeleteSession(toolID, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.DeleteSession(toolID, id)
}
func (a *App) SendMessage(toolID, sessionID, content, requestID string) (toolbox.ChatResult, error) {
	if err := a.ready(); err != nil {
		return toolbox.ChatResult{}, err
	}
	return a.service.Send(a.ctx, toolID, sessionID, content, requestID, func(d toolbox.Delta) { runtime.EventsEmit(a.ctx, "chat:delta", d) })
}
func (a *App) CancelRequest(id string) {
	if a.service != nil {
		a.service.Cancel(id)
	}
}

func (a *App) ChooseWorkspace() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择工具的独立工作区目录", CanCreateDirectories: true})
}

func (a *App) ExportSession(toolID, id string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	chat, err := a.service.GetSession(toolID, id)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "导出会话为 Markdown", DefaultFilename: "conversation-" + chat.ID[:8] + ".md", Filters: []runtime.FileFilter{{DisplayName: "Markdown", Pattern: "*.md"}}})
	if err != nil || path == "" {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", chat.Title)
	for _, m := range chat.Messages {
		label := "你"
		if m.Role == "assistant" {
			label = "助手"
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", label, m.Content)
		if m.Error != "" {
			fmt.Fprintf(&b, "> %s\n\n", m.Error)
		}
	}
	return path, os.WriteFile(path, []byte(b.String()), 0600)
}
