// Package app lam viec "wiring": tao tat ca dependency va dieu phoi start/stop.
// Doc file nay la hieu toan bo so do phu thuoc cua service.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/database"
	"github.com/abc-workspace/chat-service/internal/logger"
	"github.com/abc-workspace/chat-service/internal/pkg/jwtauth"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository/postgres"
	"github.com/abc-workspace/chat-service/internal/service/auth"
	"github.com/abc-workspace/chat-service/internal/service/bot"
	"github.com/abc-workspace/chat-service/internal/service/card"
	"github.com/abc-workspace/chat-service/internal/service/cmssync"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
	"github.com/abc-workspace/chat-service/internal/service/message"
	"github.com/abc-workspace/chat-service/internal/service/notification"
	"github.com/abc-workspace/chat-service/internal/service/outbox"
	"github.com/abc-workspace/chat-service/internal/service/task"
	"github.com/abc-workspace/chat-service/internal/transport/grpcserver"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver"
	"github.com/google/uuid"
)

type App struct {
	cfg  *config.Config
	db   *gorm.DB
	rdb  *redis.Client
	hub  *realtime.Hub
	http *httpserver.Server
	grpc *grpcserver.Server

	cmsSync *cmssync.Service
	outbox  *outbox.Service
}

func New(cfg *config.Config) (*App, error) {
	logger.Setup(cfg.Log.Level, cfg.Log.Format, cfg.App.Name, cfg.App.Env)
	for _, w := range cfg.WarnProd() {
		slog.Warn("canh bao cau hinh prod", "van_de", w)
	}

	// 1) Ha tang
	db, err := database.Open(cfg.Postgres)
	if err != nil {
		return nil, err
	}

	var (
		rdb    *redis.Client
		broker realtime.Broker
	)
	if cfg.Redis.Enabled {
		rdb = redis.NewClient(&redis.Options{
			Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			database.Close(db)
			return nil, fmt.Errorf("app: redis ping: %w", err)
		}
		broker = realtime.NewRedisBroker(rdb, cfg.Redis.Channel, 2048)
		slog.Info("realtime broker: redis", "addr", cfg.Redis.Addr)
	} else {
		broker = realtime.NewMemoryBroker(2048)
		slog.Info("realtime broker: memory (chi dung cho 1 instance)")
	}

	// 2) Repository
	store := postgres.NewStore(db)

	// 3) Service
	hub := realtime.NewHub(broker)

	// Xac thuc: khi tich hop CMS thi token do CMS phat (RS256 + JWKS).
	// Van cho phep token dev HS256 (make token / make ws / Postman) khi AUTH_ALLOW_DEV_TOKEN=true.
	var verifiers []jwtauth.Verifier
	cmsClient := cms.New(cfg.CMS)
	if cfg.CMS.Enabled {
		jwks := jwtauth.NewJWKSVerifier(
			cfg.CMS.JWKSURL, cfg.CMS.Issuer, cfg.CMS.Audience, cfg.CMS.JWKSCacheTTL,
			jwtauth.WithJWKSLeeway(cfg.Auth.Leeway),
		)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := jwks.Refresh(ctx); err != nil {
			// Khong chet: CMS co the chua san sang, se tu tai lai khi co token dau tien.
			slog.Warn("khong tai duoc JWKS cua CMS luc khoi dong", "url", cfg.CMS.JWKSURL, "err", err)
		}
		cancel()
		verifiers = append(verifiers, jwks)
		slog.Info("xac thuc bang JWKS cua CMS", "url", cfg.CMS.JWKSURL,
			"iss", cfg.CMS.Issuer, "aud", cfg.CMS.Audience)
	}
	if cfg.Auth.JWTSecret != "" && (!cfg.CMS.Enabled || cfg.Auth.AllowDevToken) {
		verifiers = append(verifiers, jwtauth.NewVerifier(cfg.Auth.JWTSecret,
			jwtauth.WithIssuer(cfg.Auth.JWTIssuer),
			jwtauth.WithAudience(cfg.Auth.JWTAudience),
			jwtauth.WithLeeway(cfg.Auth.Leeway),
		))
	}
	if len(verifiers) == 0 {
		database.Close(db)
		return nil, fmt.Errorf("app: khong co cach xac thuc nao (can CMS_ENABLED hoac JWT_SECRET)")
	}
	authSvc := auth.New(jwtauth.NewChainVerifier(verifiers...), store.Users())
	if cfg.CMS.Enabled {
		authSvc.SetProfileSource(cmsClient)
	}
	convSvc := conversation.New(store, hub)
	botSvc := bot.New(store)
	msgSvc := message.New(store, convSvc, botSvc, hub)
	taskSvc := task.New(store, convSvc, msgSvc, hub)

	// Bot "@bot": mac dinh chi LUU mau (TemplateHandler).
	// Chi khi bat task engine noi bo thi mau moi bien thanh task/phieu sua cua Chat —
	// khi tich hop CMS, don/task do CMS tao tu event form.submit.
	if cfg.App.TaskEngine {
		botSvc.Register("bot", task.NewBotHandler(bot.NewTemplateHandler(store), taskSvc))
	}

	// 3b) Tich hop CMS: dong bo nhom/user, card, thong bao, outbox.
	notifSvc := notification.New(store, hub)
	cardSvc := card.New(store, convSvc, cmsClient, hub, cfg.CMS.WebhookTimeout)
	syncSvc := cmssync.New(cmsClient, store, hub, cfg.CMS.SyncInterval)
	outboxSvc := outbox.New(store, cmsClient, cfg.CMS.OutboxInterval, cfg.CMS.OutboxMaxRetry)
	if cfg.CMS.Enabled {
		// Tin nhan thuong cua nguoi dung -> event message.created (bat dong bo).
		msgSvc.AddMessageHook(outboxSvc.EnqueueMessageCreated)
	}

	// Khi user online/offline -> cap nhat DB (phuc vu hien trang thai OFF canh ten).
	hub.SetPresenceHook(func(ctx context.Context, userID uuid.UUID, online bool) {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := authSvc.SetPresence(c, userID, online); err != nil {
			slog.Warn("cap nhat presence that bai", "user_id", userID, "err", err)
		}
	})

	// 4) Transport
	httpSrv := httpserver.New(cfg, httpserver.Deps{
		Auth:          authSvc,
		Conversations: convSvc,
		Messages:      msgSvc,
		Bots:          botSvc,
		Tasks:         taskSvc,
		Hub:           hub,
		Store:         store,
		Cards:         cardSvc,
		Notifications: notifSvc,
		CMSSync:       syncSvc,
		Ready: func(ctx context.Context) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			return sqlDB.PingContext(ctx)
		},
	})
	grpcSrv := grpcserver.New(cfg.GRPC, grpcserver.Deps{
		Auth:          authSvc,
		Conversations: convSvc,
		Messages:      msgSvc,
		Bots:          botSvc,
		Tasks:         taskSvc,
	})

	return &App{
		cfg: cfg, db: db, rdb: rdb, hub: hub,
		http: httpSrv, grpc: grpcSrv,
		cmsSync: syncSvc, outbox: outboxSvc,
	}, nil
}

// Run chay HTTP + gRPC + hub, dung lai gon gang khi nhan SIGINT/SIGTERM.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error { return a.hub.Run(gctx) })

	// Dong bo nhom tu CMS + gui event bat dong bo (chi khi bat tich hop).
	if a.cfg.CMS.Enabled {
		g.Go(func() error { return a.cmsSync.Run(gctx) })
		g.Go(func() error { return a.outbox.Run(gctx) })
	}
	g.Go(func() error { return a.http.Start() })
	g.Go(func() error { return a.grpc.Start() })

	// Cho tin hieu dung.
	g.Go(func() error {
		<-gctx.Done()
		slog.Info("nhan tin hieu dung, bat dau graceful shutdown",
			"timeout", a.cfg.App.ShutdownTTL.String())
		a.grpc.Stop()
		if err := a.http.Shutdown(a.cfg.App.ShutdownTTL); err != nil {
			slog.Error("http shutdown error", "err", err)
		}
		return nil
	})

	err := g.Wait()
	a.close()
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (a *App) close() {
	database.Close(a.db)
	if a.rdb != nil {
		_ = a.rdb.Close()
	}
	slog.Info("da dong tat ca ket noi")
}
