package platform

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// BuildMainWindowOptions creates the baseline main window options and applies platform-specific window behaviour.
func BuildMainWindowOptions(useNativeTitleBar bool) application.WebviewWindowOptions {
	return buildMainWindowOptions(useNativeTitleBar, runtime.GOOS)
}

func buildMainWindowOptions(useNativeTitleBar bool, targetOS string) application.WebviewWindowOptions {
	options := application.WebviewWindowOptions{
		Name:             "main",
		Title:            "InvestGo",
		URL:              "/",
		Width:            1200,
		Height:           828,
		MinWidth:         1200,
		MinHeight:        828,
		BackgroundColour: application.NewRGB(247, 243, 233),
		Windows: application.WindowsWindow{
			Theme: application.SystemDefault,
		},
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropTranslucent,
		},
	}

	if !useNativeTitleBar {
		if targetOS == "darwin" {
			options.Mac.TitleBar = application.MacTitleBarHiddenInsetUnified
		} else {
			options.Frameless = true
		}
	}

	return options
}
