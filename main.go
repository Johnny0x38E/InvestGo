package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"invest-monitor-v3/internal/monitor"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var defaultTerminalLogging = "0"
var defaultDevToolsBuild = "0"

//go:embed frontend/dist
var frontendAssets embed.FS

// 用 PNG 直接嵌入运行时应用图标，这样开发构建和生产构建看到的是同一套品牌资产。
//
//go:embed build/appicon.png
var appIcon []byte

func main() {
	logs := monitor.NewLogBook(400)
	if terminalLoggingEnabled() {
		logs.EnableConsole(os.Stderr)
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(logs.Writer("backend", "stdlib", monitor.DeveloperLogError))
	if err := logs.ConfigureFile(defaultLogPath()); err != nil {
		log.Printf("configure log file: %v", err)
	}
	defer func() { _ = logs.Close() }()

	logs.Info("backend", "app", "starting Invest Monitor")

	quoteProviders, quoteSourceOptions := monitor.DefaultQuoteSourceRegistry(nil)
	store, err := monitor.NewStore(
		defaultStatePath(),
		quoteProviders,
		quoteSourceOptions,
		monitor.NewYahooChartProvider(nil),
		logs,
	)
	if err != nil {
		log.Fatalf("initialise store: %v", err)
	}
	hotService := monitor.NewHotService(nil)

	frontendFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		log.Fatalf("load frontend assets: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", monitor.NewAPI(store, hotService, logs))
	mux.Handle("/", application.BundledAssetFileServer(frontendFS))

	app := application.New(application.Options{
		Name:        "Invest Monitor",
		Description: "Go + Wails v3 投资监控桌面应用",
		Icon:        appIcon,
		Logger:      logs.NewSlogLogger("system", slog.LevelInfo),
		Assets: application.AssetOptions{
			Handler:        mux,
			DisableLogging: true,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		PanicHandler: func(details *application.PanicDetails) {
			logs.Error("backend", "panic", fmt.Sprintf("%s\n%s", details.Error, details.StackTrace))
		},
		OnShutdown: func() {
			logs.Info("backend", "app", "shutdown requested")
			if err := store.Save(); err != nil {
				logs.Error("backend", "storage", fmt.Sprintf("save state on shutdown failed: %v", err))
			}
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "Invest Monitor",
		URL:              "/",
		Width:            1420,
		Height:           980,
		MinWidth:         1180,
		MinHeight:        760,
		BackgroundColour: application.NewRGB(247, 243, 233),
		Windows: application.WindowsWindow{
			Theme: application.SystemDefault,
		},
		KeyBindings: map[string]func(window application.Window){
			"F12": func(window application.Window) {
				snapshot := store.Snapshot()
				if !snapshot.Settings.DeveloperMode {
					logs.Warn("system", "devtools", "ignored F12 because developer mode is disabled")
					return
				}
				if !devToolsBuildEnabled() {
					logs.Warn("system", "devtools", "ignored F12 because this binary was built without devtools support")
					return
				}
				logs.Info("system", "devtools", "opening web inspector")
				window.OpenDevTools()
			},
		},
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInsetUnified,
			InvisibleTitleBarHeight: 56,
		},
	})

	if err := app.Run(); err != nil {
		log.Printf("run app: %v", err)
		os.Exit(1)
	}
}

func defaultStatePath() string {
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "invest-monitor-v3", "state.json")
	}

	return filepath.Join(".", "data", "state.json")
}

// 日志文件和 state.json 放在同一个配置根目录下，便于用户备份和排障。
func defaultLogPath() string {
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "invest-monitor-v3", "logs", "app.log")
	}

	return filepath.Join(".", "data", "logs", "app.log")
}

func terminalLoggingEnabled() bool {
	if defaultTerminalLogging == "1" {
		return true
	}

	for _, arg := range os.Args[1:] {
		if arg == "-dev" || arg == "--dev" {
			return true
		}
	}

	return false
}

func devToolsBuildEnabled() bool {
	return defaultDevToolsBuild == "1"
}
