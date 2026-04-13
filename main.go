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

// 嵌入前端构建产物，供 Wails 在运行时直接提供静态资源。
//
//go:embed frontend/dist
var frontendAssets embed.FS

// 嵌入应用图标
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

	// 首先获取快照以读取设置
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

	// 根据设置决定是否使用自定义标题栏
	if !useNativeTitleBar {
		windowOptions.Mac.TitleBar = application.MacTitleBarHiddenInsetUnified
	}

	app.Window.NewWithOptions(windowOptions)

	if err := app.Run(); err != nil {
		log.Printf("run app: %v", err)
		os.Exit(1)
	}
}

// defaultStatePath 返回状态文件的默认存储路径。
func defaultStatePath() string {
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "investgo", "state.json")
	}

	return filepath.Join(".", "data", "state.json")
}

// defaultLogPath 返回日志文件的默认存储路径。
// 日志文件和 state.json 都在 $HOME/Library/Application Support/investgo/ 路径下。
func defaultLogPath() string {
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "investgo", "logs", "app.log")
	}

	return filepath.Join(".", "data", "logs", "app.log")
}

// terminalLoggingEnabled 返回当前进程是否应把开发日志同步输出到终端。
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

// devToolsBuildEnabled 返回当前二进制是否启用了 DevTools 支持。
func devToolsBuildEnabled() bool {
	return defaultDevToolsBuild == "1"
}

// applySystemProxy 在 macOS 上读取系统代理设置，并在未手动配置时自动注入到进程环境变量。
// 后续所有 net/http 客户端都会通过 http.ProxyFromEnvironment 透明地走代理，
// 无需在工具侧额外配置"增强模式" / TUN 模式。
func applySystemProxy(logs *monitor.LogBook) {
	if runtime.GOOS != "darwin" {
		return
	}
	// 用户已手动配置代理时，保留显式配置，避免被系统设置覆盖。
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

	// 把排除列表同步到 NO_PROXY，避免局域网和 localhost 流量误走代理。
	if len(exceptions) > 0 && os.Getenv("NO_PROXY") == "" && os.Getenv("no_proxy") == "" {
		noProxy := strings.Join(exceptions, ",")
		_ = os.Setenv("NO_PROXY", noProxy)
		logs.Info("backend", "proxy", "NO_PROXY set from system exceptions: "+noProxy)
	}

	// 优先使用 HTTPS 代理，其次回退到 HTTP 代理。
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

// parseScutilProxy 把 scutil --proxy 的输出解析为键值映射以及排除列表。
// 普通行格式为 "  Key : Value"；ExceptionsList 段的每一个数组项都会被收集到 exceptions。
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
			// 数组条目格式：  "N : value"（value 可能本身是逗号分隔的多个条目）
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
