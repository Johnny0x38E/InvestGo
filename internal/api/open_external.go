package api

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openExternalURL dispatches to the platform default opener without exposing OS-specific commands to route handlers.
func openExternalURL(targetURL string) error {
	var command *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", targetURL)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		command = exec.Command("xdg-open", targetURL)
	}

	if err := command.Start(); err != nil {
		return fmt.Errorf("open external URL: %w", err)
	}
	return nil
}
