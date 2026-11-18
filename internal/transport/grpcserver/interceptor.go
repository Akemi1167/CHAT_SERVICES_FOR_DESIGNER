package grpcserver

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/abc-workspace/chat-service/internal/logger"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/service/auth"
)

// publicMethods khong can token.
var publicMethods = map[string]bool{
	"/grpc.health.v1.Health/Check": true,
	"/grpc.health.v1.Health/Watch": true,
}

// AuthUnary doc metadata "authorization" -> gan Identity vao context.
func AuthUnary(authSvc *auth.Service) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if publicMethods[info.FullMethod] || strings.HasPrefix(info.FullMethod, "/grpc.reflection") {
			return handler(ctx, req)
		}
		md, _ := metadata.FromIncomingContext(ctx)
		token := first(md, "authorization")
		if token == "" {
			token = first(md, "x-access-token")
		}
		identity, err := authSvc.Authenticate(ctx, token)
		if err != nil {
			return nil, toStatus(err)
		}
		return handler(auth.WithIdentity(ctx, identity), req)
	}
}

// RequestIDUnary gan request_id (tu client hoac sinh moi) de trace log.
func RequestIDUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		rid := first(md, "x-request-id")
		if rid == "" {
			rid = uuid.NewString()
		}
		return handler(logger.WithRequestID(ctx, rid), req)
	}
}

// LogUnary log moi RPC + do thoi gian (de debug nhanh).
func LogUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		attrs := []any{
			"method", info.FullMethod,
			"took_ms", time.Since(start).Milliseconds(),
			"request_id", logger.RequestID(ctx),
		}
		if err != nil {
			st, _ := status.FromError(err)
			attrs = append(attrs, "code", st.Code().String(), "err", st.Message())
			if st.Code() == codes.Internal {
				slog.Error("grpc call failed", attrs...)
			} else {
				slog.Warn("grpc call rejected", attrs...)
			}
			return resp, err
		}
		slog.Info("grpc call", attrs...)
		return resp, nil
	}
}

// ErrorUnary chuyen apperr -> grpc status (handler chi can return apperr).
func ErrorUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			return resp, toStatus(err)
		}
		return resp, nil
	}
}

// RecoverUnary chong panic lam chet server.
func RecoverUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc panic", "method", info.FullMethod, "panic", r)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

func toStatus(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		if _, isApp := err.(*apperr.Error); !isApp {
			return err
		}
	}
	e := apperr.From(err)
	return status.Error(apperr.GRPCCode(e), e.Code+": "+e.Message)
}

func first(md metadata.MD, key string) string {
	if md == nil {
		return ""
	}
	if vs := md.Get(key); len(vs) > 0 {
		return vs[0]
	}
	return ""
}
