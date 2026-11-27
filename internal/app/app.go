// Package app lam viec "wiring": tao tat ca dependency va dieu phoi start/stop.
// Doc file nay la hieu toan bo so do phu thuoc cua service.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/abc-workspace/chat-service/internal/dblog"
	"github.com/abc-workspace/chat-service/internal/domain"
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
	"github.com/abc-workspace/chat-service/internal/service/presence"
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
	dbLog   *dblog.Writer // nil = LOG_DB_ENABLED=false

	logFile  io.Closer // file LOG_FILE (noop khi khong ghi file)
	presence *presence.Service
}

func New(cfg *config.Config) (*App, error) {
	_, logFile := logger.Setup(logger.Options{
		Level:   cfg.Log.Level,
		Format:  cfg.Log.Format,
		File:    cfg.Log.File,
		Stdout:  cfg.Log.Stdout,
		Service: cfg.App.Name,
		Env:     cfg.App.Env,
	})
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

	// Ghi them ban sao log vao app_logs (debug). Log truoc dong nay chi co o stdout/file.
	var dbLog *dblog.Writer
	if cfg.Log.DBEnabled {
		dbLog = dblog.NewWriter(db, cfg.Log.DBRetention)
		logger.AttachSink(dbLog, logger.ParseLevel(cfg.Log.DBLevel))
		slog.Info("ghi log vao DB (app_logs)", "level", cfg.Log.DBLevel, "retention", cfg.Log.DBRetention.String())
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

	// 3b) Tich hop CMS: dong bo nhom/user, card, thong bao, outbox.
	notifSvc := notification.New(store, hub)
	// Nhac ten @ trong tin nhan -> thong bao "X nhac den ban trong ..." (Chat tu gui).
	msgSvc.SetNotifier(notifSvc)
	// Nguong "online da cu" dung chung cho moi cho tra presence ra ngoai.
	domain.SetPresenceTTL(cfg.Presence.TTL)
	presenceSvc := presence.New(store, hub)
	presenceSvc.SetHeartbeat(cfg.Presence.Heartbeat)
	cardSvc := card.New(store, convSvc, cmsClient, hub, msgSvc, cfg.CMS.WebhookTimeout)
	syncSvc := cmssync.New(cmsClient, store, hub, cfg.CMS.SyncInterval)
	outboxSvc := outbox.New(store, cmsClient, cfg.CMS.OutboxInterval, cfg.CMS.OutboxMaxRetry)
	if cfg.CMS.Enabled {
		// Tin nhan thuong cua nguoi dung -> event message.created (bat dong bo).
		msgSvc.AddMessageHook(outboxSvc.EnqueueMessageCreated)
	}

	// Khi user mo/dong WebSocket: cap nhat DB va bao cho nhung nguoi chung nhom.
	hub.SetPresenceHook(func(ctx context.Context, userID uuid.UUID, online bool) {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := presenceSvc.SetOnline(c, userID, online); err != nil {
			slog.Warn("cap nhat presence that bai", "user_id", userID, "err", err)
		}
	})

	// 4) Transport
	httpSrv := httpserver.New(cfg, httpserver.Deps{
		Auth:          authSvc,
		Conversations: convSvc,
		Messages:      msgSvc,
		Bots:          botSvc,
		Hub:           hub,
		Store:         store,
		Cards:         cardSvc,
		Notifications: notifSvc,
		Presence:      presenceSvc,
		CMSSync:       syncSvc,
		DBLogs:        dblog.NewReader(db),
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
	})

	return &App{
		cfg: cfg, db: db, rdb: rdb, hub: hub,
		http: httpSrv, grpc: grpcSrv,
		cmsSync: syncSvc, outbox: outboxSvc, dbLog: dbLog,
		presence: presenceSvc,
		logFile:  logFile,
	}, nil
}

// Run chay HTTP + gRPC + hub, dung lai gon gang khi nhan SIGINT/SIGTERM.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error { return a.hub.Run(gctx) })
	g.Go(func() error { return a.presence.Run(gctx) })
	// Ghi log vao DB chay ngoai errgroup (ctx rieng): phai dung SAU cung, de ghi not
	// log cua qua trinh shutdown, nhung TRUOC khi dong DB (a.close).
	stopDBLog := func() {}
	if a.dbLog != nil {
		logCtx, cancel := context.WithCancel(context.Background())
		logDone := make(chan struct{})
		go func() { _ = a.dbLog.Run(logCtx); close(logDone) }()
		stopDBLog = func() {
			logger.AttachSink(nil, 0)
			cancel()
			<-logDone
		}
	}

	// Dong bo nhom tu CMS + gui event bat dong bo (chi khi bat tich hop).
	if a.cfg.CMS.Enabled {
		g.Go(func() error { return a.cmsSync.Run(gctx) })
		// Ho ten / vai tro doi tren CMS -> cap nhat actor (chip @ hien ten moi).
		g.Go(func() error { return a.cmsSync.RunProfileSync(gctx, a.cfg.CMS.UserSyncInterval) })
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
	stopDBLog()
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
	// Dong file log cuoi cung: moi dong tren van phai ghi duoc.
	if a.logFile != nil {
		_ = a.logFile.Close()
	}
}
