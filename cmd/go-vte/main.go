// Command go-vte launches the GPU-accelerated terminal emulator.
package main

import (
	"log"

	"github.com/yzolkin/go-vte/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
