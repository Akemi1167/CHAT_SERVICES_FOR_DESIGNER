// Package config doc toan bo cau hinh tu bien moi truong (12-factor).
// Them config moi: khai bao field + doc trong Load() + them vao .env.example.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/abc-workspace/chat-service/internal/domain"
)

type Config struct {
	App      App
	HTTP     HTTP
	GRPC     GRPC
	Postgres Postgres
	Redis    Redis
	Auth     Auth
	CMS      CMS
	Log      Log
	Presence Presence
}

type App struct {
	Name        string
	Env         string // local | dev | staging | prod
	ShutdownTTL time.Duration
	// TaskEngine: bat module task noi bo cua Chat (tasks/assignments/revisions).
	// Mac dinh TAT vi khi tich hop CMS thi CMS la nguon su that cua don/task.
	TaskEngine bool
}

type HTTP struct {
	Addr           string
	AllowOrigins   string
	BodyLimitMB    int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	WSPingInterval time.Duration
	// WSReadTimeout: khong doc duoc gi (ke ca pong) trong khoang nay thi dong ket noi.
	// Phai lon hon WSPingInterval; client thu cong (Postman) hay ngoi im nen de rong rai.
	WSReadTimeout   time.Duration
	WSWriteTimeout  time.Duration
	WSReadLimitByte int64
	WSSendBuffer    int
	// WebDemoDir: thu muc trang thu tay (web/). Rong = khong phuc vu (dung o prod).
	WebDemoDir string
	// DocsKey = DOCS_API_KEY: khoa cho GET /docs/flow (header X-Docs-Key).
	// Rong = khong mo route.
	DocsKey string
}

type GRPC struct {
	Addr             string
	EnableReflection bool
	MaxRecvMB        int
}

type Postgres struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	AutoMigrate     bool // chi nen bat o local
	LogSQL          bool
	LogPretty       bool // in SQL nhieu dong + mau (chi dung o dev)
	SlowQuery       time.Duration
}

type Redis struct {
	Addr     string
	Password string
	DB       int
	// Enabled = false -> dung broker in-memory (1 instance). Bat khi scale >1 pod.
	Enabled bool
	Channel string
}

// Presence: chong "online mac ket" khi process bi kill (xem internal/domain/presence.go).
type Presence struct {
	// Heartbeat: chu ky lam tuoi last_seen_at cho nguoi dang mo WebSocket.
	Heartbeat time.Duration
	// TTL: online ma last_seen_at cu hon nguong nay thi coi la offline.
	// Nen >= 3 lan Heartbeat. <=0 = tat (luon tin cot presence).
	TTL time.Duration
}

type Auth struct {
	JWTSecret   string
	JWTIssuer   string
	JWTAudience string
	Leeway      time.Duration
	// AllowDevToken: chap nhan token HS256 ky bang JWTSecret (cho `make token`,
	// `make ws`, Postman). Mac dinh chi bat khi khong phai prod.
	AllowDevToken bool
}

// CMS la cau hinh tich hop voi CMS/Task Engine (Node).
// CMS la nguon su that cho tai khoan, nhom, thanh vien va don (card);
// Chat la nguon su that cho actorId, tin nhan, cardVersion, trang thai doc, presence.
type CMS struct {
	Enabled bool
	// BaseURL dung de ghep avatarUrl tuong doi CMS tra ve (/api/avatars/...).
	BaseURL string
	// WorkspaceBaseURL: $CMS/api/client/v1/workspace
	WorkspaceBaseURL string
	// WebhookURL: URL webhook cua bot, vd http://localhost:3001/webhook/web/designer
	WebhookURL string
	// ServiceKey = CHAT_SERVICE_KEY: dung cho CA 3 huong
	// (ky webhook, goi Workspace API, kiem request Bot API cua CMS).
	ServiceKey string

	// Kiem access token do CMS phat (RS256 + JWKS).
	JWKSURL      string
	JWKSCacheTTL time.Duration
	Issuer       string
	Audience     string

	// Dong bo nhom dinh ky (ngoai viec CMS day PUT /bot/v1/groups/:id).
	SyncInterval time.Duration
	// Timeout goi Workspace API va webhook dong bo (form.submit, card.action).
	HTTPTimeout    time.Duration
	WebhookTimeout time.Duration
	// Gui lai event bat dong bo (message.created, user.updated) tu outbox.
	OutboxInterval time.Duration
	OutboxMaxRetry int
}

type Log struct {
	Level  string // debug | info | warn | error
	Format string // json | text
	// File: ghi log ra file (vd ./logs/app.log). Chi dung o local de grep lai;
	// prod de trong, log do docker/k8s thu gom tu stdout.
	File string
	// Stdout=false (can co File): khong in log ra console nua. Dung o local khi
	// muon console docker sach, chi con thu minh chu dong in (vd SQL pretty log).
	Stdout bool
}

// Load doc .env (neu co) roi doc env var. Tra loi neu thieu config bat buoc.
func Load() (*Config, error) {
	_ = godotenv.Load()

	env := str("APP_ENV", "local")
	isProd := env == "prod" || env == "production"

	cfg := &Config{
		App: App{
			Name:        str("APP_NAME", "chat-service"),
			Env:         env,
			ShutdownTTL: dur("APP_SHUTDOWN_TIMEOUT", 15*time.Second),
			TaskEngine:  boolean("TASK_ENGINE_ENABLED", false),
		},
		HTTP: HTTP{
			Addr:            str("HTTP_ADDR", ":8080"),
			AllowOrigins:    str("HTTP_ALLOW_ORIGINS", "*"),
			WebDemoDir:      str("WEB_DEMO_DIR", "./web"),
			DocsKey:         str("DOCS_API_KEY", ""),
			BodyLimitMB:     num("HTTP_BODY_LIMIT_MB", 25),
			ReadTimeout:     dur("HTTP_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    dur("HTTP_WRITE_TIMEOUT", 30*time.Second),
			WSPingInterval:  dur("WS_PING_INTERVAL", 25*time.Second),
			WSReadTimeout:   dur("WS_READ_TIMEOUT", 90*time.Second),
			WSWriteTimeout:  dur("WS_WRITE_TIMEOUT", 10*time.Second),
			WSReadLimitByte: int64(num("WS_READ_LIMIT_BYTES", 1<<20)),
			WSSendBuffer:    num("WS_SEND_BUFFER", 256),
		},
		GRPC: GRPC{
			Addr:             str("GRPC_ADDR", ":9090"),
			EnableReflection: boolean("GRPC_REFLECTION", true),
			MaxRecvMB:        num("GRPC_MAX_RECV_MB", 16),
		},
		Postgres: Postgres{
			DSN:             str("POSTGRES_DSN", ""),
			MaxOpenConns:    num("POSTGRES_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    num("POSTGRES_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: dur("POSTGRES_CONN_MAX_LIFETIME", time.Hour),
			AutoMigrate:     boolean("POSTGRES_AUTO_MIGRATE", false),
			LogSQL:          boolean("POSTGRES_LOG_SQL", false),
			LogPretty:       boolean("POSTGRES_LOG_PRETTY", false),
			SlowQuery:       dur("POSTGRES_SLOW_QUERY", 300*time.Millisecond),
		},
		Redis: Redis{
			Addr:     str("REDIS_ADDR", "localhost:6379"),
			Password: str("REDIS_PASSWORD", ""),
			DB:       num("REDIS_DB", 0),
			Enabled:  boolean("REDIS_ENABLED", false),
			Channel:  str("REDIS_EVENT_CHANNEL", "chat:events"),
		},
		Auth: Auth{
			JWTSecret:     str("JWT_SECRET", ""),
			JWTIssuer:     str("JWT_ISSUER", ""),
			JWTAudience:   str("JWT_AUDIENCE", ""),
			Leeway:        dur("JWT_LEEWAY", 30*time.Second),
			AllowDevToken: boolean("AUTH_ALLOW_DEV_TOKEN", !isProd),
		},
		CMS: CMS{
			Enabled:          boolean("CMS_ENABLED", false),
			BaseURL:          strings.TrimRight(str("CMS_BASE_URL", "http://localhost:3001"), "/"),
			WorkspaceBaseURL: strings.TrimRight(str("CMS_WORKSPACE_BASE_URL", "http://localhost:3001/api/client/v1/workspace"), "/"),
			WebhookURL:       str("CMS_WEBHOOK_URL", "http://localhost:3001/webhook/web/designer"),
			ServiceKey:       str("CHAT_SERVICE_KEY", ""),
			JWKSURL:          str("CMS_JWKS_URL", ""),
			JWKSCacheTTL:     dur("CMS_JWKS_CACHE_TTL", 6*time.Hour),
			Issuer:           str("CMS_JWT_ISSUER", "designer-cms"),
			Audience:         str("CMS_JWT_AUDIENCE", "abc-workspace"),
			SyncInterval:     dur("CMS_SYNC_INTERVAL", 60*time.Second),
			HTTPTimeout:      dur("CMS_HTTP_TIMEOUT", 10*time.Second),
			WebhookTimeout:   dur("CMS_WEBHOOK_TIMEOUT", 5*time.Second),
			OutboxInterval:   dur("CMS_OUTBOX_INTERVAL", 10*time.Second),
			OutboxMaxRetry:   num("CMS_OUTBOX_MAX_RETRY", 8),
		},
		Presence: Presence{
			Heartbeat: dur("PRESENCE_HEARTBEAT", domain.DefaultPresenceHeartbeat),
			TTL:       dur("PRESENCE_TTL", domain.DefaultPresenceTTL),
		},
		Log: Log{
			Level:  str("LOG_LEVEL", "info"),
			Format: str("LOG_FORMAT", "json"),
			File:   str("LOG_FILE", ""),
			Stdout: boolean("LOG_STDOUT", true),
		},
	}

	if cfg.CMS.JWKSURL == "" {
		cfg.CMS.JWKSURL = cfg.CMS.WorkspaceBaseURL + "/.well-known/jwks.json"
	}

	var missing []string
	if cfg.Postgres.DSN == "" {
		missing = append(missing, "POSTGRES_DSN")
	}
	// Khi bat CMS thi token nguoi dung do CMS phat (RS256) nen khong bat buoc JWT_SECRET;
	// chi can khi van cho phep token dev HS256.
	if cfg.Auth.JWTSecret == "" && (!cfg.CMS.Enabled || cfg.Auth.AllowDevToken) {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.CMS.Enabled && cfg.CMS.ServiceKey == "" {
		missing = append(missing, "CHAT_SERVICE_KEY (CMS_ENABLED=true)")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("config: thieu bien moi truong bat buoc: %s", strings.Join(missing, ", "))
	}
	if err := cfg.validateProd(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) IsProd() bool { return c.App.Env == "prod" || c.App.Env == "production" }

// Cac gia tri mau trong .env.example — khong duoc phep dung o prod.
var unsafeSecrets = map[string]bool{
	"doi-secret-nay-trong-moi-truong-that": true,
	"mock_service_key":                     true,
	"changeme":                             true,
	"secret":                               true,
}

// validateProd chan nhung cau hinh de gay su co that khi chay that.
// Loi => service khong khoi dong (tot hon la chay voi lo hong ma khong ai biet).
func (c *Config) validateProd() error {
	if !c.IsProd() {
		return nil
	}
	var errs []string

	if c.Auth.AllowDevToken {
		errs = append(errs, "AUTH_ALLOW_DEV_TOKEN=true o prod: token HS256 tu ky se dang nhap duoc bang bat ky ai")
	}
	if unsafeSecrets[strings.ToLower(strings.TrimSpace(c.Auth.JWTSecret))] {
		errs = append(errs, "JWT_SECRET dang dung gia tri mau trong .env.example")
	}
	if c.CMS.Enabled && unsafeSecrets[strings.ToLower(strings.TrimSpace(c.CMS.ServiceKey))] {
		errs = append(errs, "CHAT_SERVICE_KEY dang dung gia tri mau")
	}
	if c.Postgres.AutoMigrate {
		errs = append(errs, "POSTGRES_AUTO_MIGRATE=true o prod: phai chay migration bang golang-migrate")
	}
	if len(errs) > 0 {
		return fmt.Errorf("config khong an toan cho prod:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// warnProd canh bao nhung thu nen sua nhung khong chan khoi dong.
// WarnProd tra danh sach canh bao cau hinh prod (khong chan khoi dong).
func (c *Config) WarnProd() []string {
	if !c.IsProd() {
		return nil
	}
	var warns []string
	if c.HTTP.AllowOrigins == "*" {
		warns = append(warns, "HTTP_ALLOW_ORIGINS=* : nen liet ke domain FE")
	}
	if c.GRPC.EnableReflection {
		warns = append(warns, "GRPC_REFLECTION=true : nen tat o prod")
	}
	if c.Log.Level == "debug" {
		warns = append(warns, "LOG_LEVEL=debug : nhieu log va co the lo du lieu")
	}
	if c.Redis.Enabled && c.Redis.Password == "" {
		warns = append(warns, "REDIS_ENABLED=true nhung REDIS_PASSWORD rong : Redis khong co mat khau, ai vao duoc mang noi bo deu doc/ghi duoc event chat")
	}
	if c.Log.File != "" {
		warns = append(warns, "LOG_FILE=... : chi dung o local; prod de trong de docker/k8s thu gom stdout (file se phinh, khong tu rotate)")
	}
	if c.Presence.TTL > 0 && c.Presence.Heartbeat > 0 && c.Presence.TTL < 2*c.Presence.Heartbeat {
		warns = append(warns, "PRESENCE_TTL < 2x PRESENCE_HEARTBEAT : mat 1 nhip heartbeat la nguoi dang online bi coi la offline")
	}
	if !c.Log.Stdout {
		warns = append(warns, "LOG_STDOUT=false : prod phai de true, neu khong docker/k8s khong thu duoc log nao")
	}
	if c.Postgres.LogSQL {
		warns = append(warns, "POSTGRES_LOG_SQL=true : khong nen bat o prod")
	}
	if c.Postgres.LogPretty {
		warns = append(warns, "POSTGRES_LOG_PRETTY=true : chi dung o dev, SQL log se chua gia tri tham so that")
	}
	return warns
}

func str(key, def string) string {
	if v := env(key); v != "" {
		return v
	}
	return def
}

// env doc bien moi truong va CAT KHOANG TRANG hai dau.
// Bat buoc phai cat: docker compose truyen gia tri cua `.env` kem khoang trang
// dem truoc dau `#` (vd "true        "), ParseBool/Atoi se loi va am tham rot
// ve gia tri mac dinh — cau hinh coi nhu bi bo qua ma khong bao gi.
func env(key string) string { return strings.TrimSpace(os.Getenv(key)) }

func num(key string, def int) int {
	if v := env(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func boolean(key string, def bool) bool {
	if v := env(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func dur(key string, def time.Duration) time.Duration {
	if v := env(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
