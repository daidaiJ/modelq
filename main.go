// Command orx queries OpenRouter for model parameters and pricing.
package main

import (
	"fmt"
	"os"

	"github.com/daidaiJ/openrouter-cli/internal/cmd"
)

// version is injected at build time: -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	if err := cmd.NewRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "orx: "+err.Error())
		os.Exit(1)
	}
}
