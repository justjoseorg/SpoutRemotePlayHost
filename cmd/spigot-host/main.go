package main

import (
	"fmt"
	"os"

	"github.com/justjoseorg/SpigotRemotePlayHost/internal/display"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	fmt.Println("spigot-host", version)
	m := display.New()
	if err := m.Create(display.Mode{Width: 1280, Height: 800, RefreshHz: 60}); err != nil {
		fmt.Fprintln(os.Stderr, "spigot-host:", err)
		os.Exit(1)
	}
}
