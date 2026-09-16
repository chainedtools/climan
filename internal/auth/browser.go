package auth

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// OpenBrowser launches url in the default browser. It does not wait for the
// browser window to close.
func OpenBrowser(rawURL string) error {
	if env := os.Getenv("BROWSER"); env != "" {
		cmd := exec.Command(env, rawURL)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("open browser: %w", err)
		}
		return nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
