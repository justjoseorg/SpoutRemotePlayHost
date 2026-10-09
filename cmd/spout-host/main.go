package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/justjoseorg/SpoutRemotePlayHost/internal/config"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/display"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/hotkey"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/notify"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/pairing"
	"github.com/justjoseorg/SpoutRemotePlayHost/internal/server"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	// Loopback by default; non-loopback clients must present the API token.
	addr := flag.String("listen", "127.0.0.1:47995", "address for the web UI/API (use 0.0.0.0:47995 so the Decky plugin can reach it; non-local requests need the token)")
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

	token, err := config.LoadOrCreateToken(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("API token for the Decky plugin / remote access:", token)

	disp := display.New()

	pair, err := pairing.New(filepath.Join(filepath.Dir(*cfgPath), "paired.json"), notify.Send)
	if err != nil {
		log.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(*addr)
	pair.URL = "http://127.0.0.1:" + port + "/"

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

	handler := server.New(store, disp, version, token, pair)
	fmt.Printf("spout-host %s: UI at http://%s\n", version, *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
