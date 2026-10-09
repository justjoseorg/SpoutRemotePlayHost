package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/justjoseorg/SpigotRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpigotRemotePlayHost/internal/display"
	"github.com/justjoseorg/SpigotRemotePlayHost/internal/hotkey"
	"github.com/justjoseorg/SpigotRemotePlayHost/internal/server"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	// Loopback by default; the UI has no authentication.
	addr := flag.String("listen", "127.0.0.1:47995", "address for the web UI")
	cfgPath := flag.String("config", "", "config file path (default: user config dir)")
	flag.Parse()

	if *cfgPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			log.Fatal(err)
		}
		*cfgPath = p
	}
	store, err := config.Open(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	disp := display.New()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		err := hotkey.Listen(stop, func() {
			log.Println("Ctrl+Alt+Shift+Q: closing virtual monitor")
			if err := disp.Destroy(); err != nil {
				log.Println("destroy:", err)
			}
		})
		if err != nil {
			log.Println("hotkey disabled:", err)
		}
	}()

	handler := server.New(store, disp, version)
	fmt.Printf("spigot-host %s: UI at http://%s\n", version, *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
