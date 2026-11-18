// Package httpserver dung Fiber app: REST API + WebSocket endpoint.
package httpserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/auth"
	"github.com/abc-workspace/chat-service/internal/service/bot"
	"github.com/abc-workspace/chat-service/internal/service/card"
	"github.com/abc-workspace/chat-service/internal/service/cmssync"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
	"github.com/abc-workspace/chat-service/internal/service/message"
	"github.com/abc-workspace/chat-service/internal/service/notification"
	"github.com/abc-workspace/chat-service/internal/service/presence"
	"github.com/abc-workspace/chat-service/internal/service/task"
)

type Deps struct {
	Auth          *auth.Service
	Conversations *conversation.Service
	Messages      *message.Service
	Bots          *bot.Service
	Tasks         *task.Service
	Hub           *realtime.Hub
	Store         repository.Store
	Cards         *card.Service
	Notifications *notification.Service
	Presence      *presence.Service
	CMSSync       *cmssync.Service
	Ready         func(ctx context.Context) error
}

type Server struct {
	app  *fiber.App
	addr string
}

func New(cfg *config.Config, d Deps) *Server {
	app := fiber.New(fiber.Config{
		AppName:               cfg.App.Name,
		BodyLimit:             cfg.HTTP.BodyLimitMB << 20,
		ReadTimeout:           cfg.HTTP.ReadTimeout,
		WriteTimeout:          cfg.HTTP.WriteTimeout,
		DisableStartupMessage: true,
		// Moi loi chua xu ly deu tra JSON theo chuan httpresp.
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return httpresp.Fail(c, err)
		},
	})

	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.HTTP.AllowOrigins,
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-ID",
		AllowCredentials: cfg.HTTP.AllowOrigins != "*",
	}))

	registerRoutes(app, cfg, d)
	return &Server{app: app, addr: cfg.HTTP.Addr}
}

func (s *Server) Start() error {
	slog.Info("http server listening", "addr", s.addr)
	return s.app.Listen(s.addr)
}

func (s *Server) Shutdown(timeout time.Duration) error {
	slog.Info("http server shutting down")
	return s.app.ShutdownWithTimeout(timeout)
}

// App de dung trong test (app.Test(req)).
func (s *Server) App() *fiber.App { return s.app }
