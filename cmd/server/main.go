// Command server chay chat service (HTTP + WebSocket + gRPC).
package main

import (
	"fmt"
	"os"

	"github.com/abc-workspace/chat-service/internal/app"
	"github.com/abc-workspace/chat-service/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	a, err := app.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap error:", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "runtime error:", err)
		os.Exit(1)
	}
}
