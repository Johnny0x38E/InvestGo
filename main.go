package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"investgo/internal/api"
	"investgo/internal/marketdata"
	"investgo/internal/monitor"

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
	applySystemProxy(logs)

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

	windowOptions := application.WebviewWindowOptions{
		Name:             "main",
		Title:            "InvestGo",
		URL:              "/",
		Width:            1200,
		Height:           828,
		MinWidth:         1180,
		MinHeight:        828,
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
			Backdrop: application.MacBackdropTranslucent,
		},
	}

	// Determine whether to use custom title bar based on settings
	if !useNativeTitleBar {
		windowOptions.Mac.TitleBar = application.MacTitleBarHiddenInsetUnified
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

// applySystemProxy reads system proxy settings on macOS and automatically injects them into process environment variables when not manually configured.
// Subsequent net/http clients will transparently use the proxy via http.ProxyFromEnvironment,
// without requiring additional "enhanced mode" / TUN mode configuration on the tool side.
func applySystemProxy(logs *monitor.LogBook) {
	if runtime.GOOS != "darwin" {
		return
	}
	// When user has manually configured proxy, preserve explicit configuration to avoid being overridden by system settings.
	if os.Getenv("HTTPS_PROXY") != "" || os.Getenv("HTTP_PROXY") != "" ||
		os.Getenv("https_proxy") != "" || os.Getenv("http_proxy") != "" {
		logs.Info("backend", "proxy", "HTTPS_PROXY already set, skipping system proxy detection")
		return
	}

	out, err := exec.Command("scutil", "--proxy").Output()
	if err != nil {
		return
	}

	settings, exceptions := parseScutilProxy(out)

	// Synchronize exception list to NO_PROXY to prevent LAN and localhost traffic from erroneously going through proxy.
	if len(exceptions) > 0 && os.Getenv("NO_PROXY") == "" && os.Getenv("no_proxy") == "" {
		noProxy := strings.Join(exceptions, ",")
		_ = os.Setenv("NO_PROXY", noProxy)
		logs.Info("backend", "proxy", "NO_PROXY set from system exceptions: "+noProxy)
	}

	// Prefer HTTPS proxy, then fallback to HTTP proxy.
	applyEntry := func(hostKey, portKey, enableKey, defaultPort string) bool {
		if settings[enableKey] != "1" {
			return false
		}
		host := settings[hostKey]
		if host == "" {
			return false
		}
		port := settings[portKey]
		if port == "" {
			port = defaultPort
		}
		proxyURL := "http://" + host + ":" + port
		_ = os.Setenv("HTTPS_PROXY", proxyURL)
		_ = os.Setenv("HTTP_PROXY", proxyURL)
		logs.Info("backend", "proxy", "system proxy applied: "+proxyURL)
		return true
	}

	if applyEntry("HTTPSProxy", "HTTPSPort", "HTTPSEnable", "443") {
		return
	}
	applyEntry("HTTPProxy", "HTTPPort", "HTTPEnable", "8080")
}

// parseScutilProxy parses the output of scutil --proxy into a key-value mapping and an exception list.
// Normal lines have the format "  Key : Value"; each array item in the ExceptionsList section is collected into exceptions.
func parseScutilProxy(data []byte) (kvs map[string]string, exceptions []string) {
	kvs = make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inExceptions := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, "ExceptionsList : <array>"):
			inExceptions = true
		case inExceptions && trimmed == "}":
			inExceptions = false
		case inExceptions:
			// Array entry format: "N : value" (value may itself be comma-separated multiple entries)
			parts := strings.SplitN(trimmed, " : ", 2)
			if len(parts) == 2 {
				for _, entry := range strings.Split(parts[1], ",") {
					if entry = strings.TrimSpace(entry); entry != "" {
						exceptions = append(exceptions, entry)
					}
				}
			}
		default:
			parts := strings.SplitN(trimmed, " : ", 2)
			if len(parts) == 2 {
				kvs[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}
	return kvs, exceptions
}
