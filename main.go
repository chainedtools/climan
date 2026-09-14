// climan — a unified, declarative way to install, update and manage popular
// developer CLIs (mise, asdf, sdkman, flyctl, doctl, grok, codex, claude).
//
// Declare what you want in climan.yaml; climan reconciles your system against
// it. Scriptable CLI + TUI.
package main

import (
	"os"

	"go.solved.gg/climan/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
