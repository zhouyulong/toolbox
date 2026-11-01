package main

import (
	"embed"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed frontend/index.html frontend/style.css frontend/app.js
var assets embed.FS

//go:embed VERSION
var versionFile string

func main() {
	app := NewApp(strings.TrimSpace(versionFile))
	if err := wails.Run(&options.App{
		Title: "ToolboxSelf", Width: 1240, Height: 820, MinWidth: 960, MinHeight: 640,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 246, G: 247, B: 251, A: 255},
		OnStartup:        app.startup, OnShutdown: app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "toolboxself.personal.llm.v1",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.show() },
		},
		Bind: []interface{}{app},
	}); err != nil {
		println("ToolboxSelf: " + err.Error())
	}
}
