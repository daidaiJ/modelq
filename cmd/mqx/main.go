// Command mqx queries model parameters and pricing from OpenRouter and
// models.dev.
package main

import (
	"fmt"
	"os"

	"github.com/daidaiJ/modelq/internal/cmd"
	"github.com/daidaiJ/modelq/internal/locale"
)

// version is injected at build time: -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	// The language must be resolved before the command tree is built: help
	// strings are baked in at construction time.
	if err := locale.FromArgs(os.Args[1:], os.Getenv("MQX_LANG")); err != nil {
		fmt.Fprintln(os.Stderr, "mqx: "+err.Error())
		os.Exit(1)
	}
	if err := cmd.NewRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mqx: "+err.Error())
		os.Exit(1)
	}
}
