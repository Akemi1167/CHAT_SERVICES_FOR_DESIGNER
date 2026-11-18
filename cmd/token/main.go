// Command token sinh JWT de test local (FE that se dung token tu service auth).
//
//	go run ./cmd/token -sub user-001 -name "Nguyen Van A" -role designer
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/pkg/jwtauth"
)

func main() {
	sub := flag.String("sub", "user-001", "external user id (claim sub)")
	name := flag.String("name", "Dev User", "ten hien thi")
	role := flag.String("role", "designer", "role: admin | order | designer")
	ttl := flag.Duration("ttl", 24*time.Hour, "thoi gian song cua token")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	token, err := jwtauth.Sign(cfg.Auth.JWTSecret, *sub, *name, *role, *ttl)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sign error:", err)
		os.Exit(1)
	}
	fmt.Println(token)
}
