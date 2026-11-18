package config

import (
	"strings"
	"testing"
)

// Prod khong duoc phep chay voi cau hinh de lo: token dev, secret mau, auto migrate.
func TestValidateProd(t *testing.T) {
	base := func() *Config {
		return &Config{
			App:      App{Env: "prod"},
			Auth:     Auth{JWTSecret: "mot-secret-that-dai-va-ngau-nhien"},
			Postgres: Postgres{},
			CMS:      CMS{},
		}
	}

	if err := base().validateProd(); err != nil {
		t.Fatalf("cau hinh prod hop le van bi chan: %v", err)
	}

	cases := map[string]func(*Config){
		"token dev":       func(c *Config) { c.Auth.AllowDevToken = true },
		"secret mau":      func(c *Config) { c.Auth.JWTSecret = "doi-secret-nay-trong-moi-truong-that" },
		"service key mau": func(c *Config) { c.CMS.Enabled = true; c.CMS.ServiceKey = "mock_service_key" },
		"auto migrate":    func(c *Config) { c.Postgres.AutoMigrate = true },
	}
	for name, mutate := range cases {
		cfg := base()
		mutate(cfg)
		if err := cfg.validateProd(); err == nil {
			t.Errorf("%s: phai bi chan o prod", name)
		}
	}

	// Moi truong khac prod thi khong chan.
	dev := base()
	dev.App.Env = "dev"
	dev.Auth.AllowDevToken = true
	dev.Postgres.AutoMigrate = true
	if err := dev.validateProd(); err != nil {
		t.Errorf("moi truong dev khong duoc chan: %v", err)
	}
}

func TestWarnProd(t *testing.T) {
	cfg := &Config{
		App:      App{Env: "prod"},
		HTTP:     HTTP{AllowOrigins: "*"},
		GRPC:     GRPC{EnableReflection: true},
		Log:      Log{Level: "debug"},
		Postgres: Postgres{LogSQL: true},
	}
	warns := strings.Join(cfg.WarnProd(), " | ")
	for _, want := range []string{"HTTP_ALLOW_ORIGINS", "GRPC_REFLECTION", "LOG_LEVEL", "POSTGRES_LOG_SQL"} {
		if !strings.Contains(warns, want) {
			t.Errorf("thieu canh bao %s (got: %s)", want, warns)
		}
	}
}
