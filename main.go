// Xlink Genesis Client - Wails Edition
// 主入口文件
package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"xlink-wails/internal/models"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 检查启动参数
	isAutoStart := false
	for _, arg := range os.Args[1:] {
		if strings.Contains(arg, "-autostart") {
			isAutoStart = true
			break
		}
	}

	// 获取可执行文件目录
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("无法获取程序路径:", err)
	}
	exeDir := filepath.Dir(exePath)

	// 创建应用实例
	app := NewApp()
	app.state.ExeDir = exeDir
	app.state.IsAutoStart = isAutoStart

	// 创建 Wails 应用
	err = wails.Run(&options.App{
		Title:     models.AppTitle,
		Width:     1024,
		Height:    768,
		MinWidth:  800,
		MinHeight: 600,

		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},

		// 绑定生命周期
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,

		// 绑定后端方法供前端调用
		Bind: []interface{}{
			app,
		},

		// Windows 特定配置
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
			WebviewUserDataPath:               filepath.Join(exeDir, "webview_data"),
			Theme:                             windows.SystemDefault,
		},

		// 启用单实例锁 (防止重复启动)
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "xlink-client-v22-unique-lock",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				// 当第二个实例启动时，唤醒主窗口
				app.ShowWindow()
			},
		},
	})

	if err != nil {
		log.Fatal("启动失败:", err)
	}
}
