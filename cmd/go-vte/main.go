// Command go-vte launches the GPU-accelerated terminal emulator.
package main

import (
	"flag"
	"log"

	"github.com/yzolkin/go-vte/internal/config"
	"github.com/yzolkin/go-vte/internal/ui/app"
)

func main() {
	debug := flag.Bool("debug", false, "log config loading and renderer diagnostics to stderr")
	flag.Parse()

	// Must be set before Load runs, and before the renderer reads it.
	config.Debug = *debug

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
