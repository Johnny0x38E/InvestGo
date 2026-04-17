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

	"investgo/internal/api"
	"investgo/internal/marketdata"
	"investgo/internal/monitor"
	"investgo/internal/platform"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var defaultTerminalLogging = "0"
var defaultDevToolsBuild = "0"
var appVersion = "dev"

// Embed frontend build assets for Wails to serve as static resources at runtime.
//
//go:embed frontend/dist
var frontendAssets embed.FS

// Embed application icon
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

	logs.Info("backend", "app", "starting InvestGo")
	platform.ApplySystemProxy(logs)

	quoteProviders, quoteSourceOptions := marketdata.DefaultQuoteSourceRegistry(nil)
	store, err := monitor.NewStore(
		defaultStatePath(),
		quoteProviders,
		quoteSourceOptions,
		marketdata.NewSmartHistoryProvider(nil),
		logs,
		appVersion,
	)
	if err != nil {
		log.Fatalf("initialise store: %v", err)
	}
	hotService := marketdata.NewHotService(nil)

	frontendFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		log.Fatalf("load frontend assets: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewHandler(store, hotService, logs))
	mux.Handle("/", application.BundledAssetFileServer(frontendFS))

	app := application.New(application.Options{
		Name:        "InvestGo",
		Description: "Go + Wails v3 Investment Monitor Desktop App",
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

	// First fetch a snapshot to read settings
	snapshot := store.Snapshot()
	useNativeTitleBar := snapshot.Settings.UseNativeTitleBar

	windowOptions := platform.BuildMainWindowOptions(useNativeTitleBar)
	windowOptions.KeyBindings = map[string]func(window application.Window){
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
	}

	app.Window.NewWithOptions(windowOptions)

	if err := app.Run(); err != nil {
		log.Printf("run app: %v", err)
		os.Exit(1)
	}
}

// defaultStatePath returns the default storage path for the state file.
func defaultStatePath() string {
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "investgo", "state.json")
	}

	return filepath.Join(".", "data", "state.json")
}

// defaultLogPath returns the default storage path for the log file.
// Log files and state.json are located at $HOME/Library/Application Support/investgo/.
func defaultLogPath() string {
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "investgo", "logs", "app.log")
	}

	return filepath.Join(".", "data", "logs", "app.log")
}

// terminalLoggingEnabled returns whether the current process should output development logs to the terminal.
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

// devToolsBuildEnabled returns whether the current binary has DevTools support enabled.
func devToolsBuildEnabled() bool {
	return defaultDevToolsBuild == "1"
}
