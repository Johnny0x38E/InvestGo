package platform

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"investgo/internal/logger"
)

// ApplySystemProxy reads system proxy settings on macOS and injects them into
// process environment variables. Subsequent net/http clients will transparently
// use the proxy via http.ProxyFromEnvironment.
// Only call this when the configured proxy mode is "system".
func ApplySystemProxy(logs *logger.LogBook) {
	if runtime.GOOS != "darwin" {
		return
	}
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
	if len(exceptions) > 0 && os.Getenv("NO_PROXY") == "" && os.Getenv("no_proxy") == "" {
		noProxy := strings.Join(exceptions, ",")
		_ = os.Setenv("NO_PROXY", noProxy)
		logs.Info("backend", "proxy", "NO_PROXY set from system exceptions: "+noProxy)
	}

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
			parts := strings.SplitN(trimmed, " : ", 2)
			if len(parts) == 2 {
				for entry := range strings.SplitSeq(parts[1], ",") {
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
