// Package grpcserver dung gRPC server cho giao tiep service-to-service.
package grpcserver

import (
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	chatv1 "github.com/abc-workspace/chat-service/gen/go/chat/v1"
	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/service/auth"
	"github.com/abc-workspace/chat-service/internal/service/bot"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
	"github.com/abc-workspace/chat-service/internal/service/message"
)

type Server struct {
	grpc   *grpc.Server
	addr   string
	health *health.Server
}

type Deps struct {
	Auth          *auth.Service
	Conversations *conversation.Service
	Messages      *message.Service
	Bots          *bot.Service
}

func New(cfg config.GRPC, d Deps) *Server {
	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(cfg.MaxRecvMB<<20),
		grpc.ChainUnaryInterceptor(
			RecoverUnary(),
			RequestIDUnary(),
			LogUnary(),
			AuthUnary(d.Auth),
			ErrorUnary(),
		),
	)

	chatv1.RegisterChatServiceServer(srv, NewChatHandler(d.Conversations, d.Messages))
	chatv1.RegisterBotServiceServer(srv, NewBotHandler(d.Bots, d.Messages))

	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)

	if cfg.EnableReflection {
		reflection.Register(srv) // cho phep `grpcurl` / Postman tu doc schema
	}
	return &Server{grpc: srv, addr: cfg.Addr, health: hs}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("grpcserver: listen %s: %w", s.addr, err)
	}
	s.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	slog.Info("grpc server listening", "addr", s.addr)
	return s.grpc.Serve(ln)
}

func (s *Server) Stop() {
	s.health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	s.grpc.GracefulStop()
	slog.Info("grpc server stopped")
}
