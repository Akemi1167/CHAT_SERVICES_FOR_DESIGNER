# ============================================================
#  chat-service — Makefile
#  Chay `make help` de xem toan bo lenh.
# ============================================================
SHELL := /bin/bash

APP_NAME    ?= chat-service
MODULE      := github.com/abc-workspace/chat-service
BIN_DIR     := bin
PROTO_DIR   := api/proto
GEN_DIR     := gen/go
MIGRATE_DIR := migrations
DOCKER_IMG  ?= $(APP_NAME):local

# Doc POSTGRES_DSN tu .env neu co (dung cho migrate).
ifneq (,$(wildcard .env))
include .env
export
endif
POSTGRES_DSN ?= postgres://chat:chat@localhost:5432/chat?sslmode=disable

.DEFAULT_GOAL := help

# ---------- help ----------
.PHONY: help
help: ## Hien thi danh sach lenh
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ---------- setup ----------
.PHONY: setup
setup: ## Cai cac tool can thiet (protoc plugins, migrate, golangci-lint)
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Luu y: can co 'protoc' (brew install protobuf)"

.PHONY: deps
deps: ## go mod tidy + download
	go mod tidy
	go mod download

# ---------- proto / grpc ----------
.PHONY: proto
proto: ## Sinh code Go tu file .proto
	@mkdir -p $(GEN_DIR)
	protoc -I $(PROTO_DIR) \
	  --go_out=$(GEN_DIR)      --go_opt=module=$(MODULE)/gen/go \
	  --go-grpc_out=$(GEN_DIR) --go-grpc_opt=module=$(MODULE)/gen/go \
	  $(PROTO_DIR)/chat/v1/*.proto
	@echo "✓ da sinh code vao $(GEN_DIR)"

.PHONY: proto-clean
proto-clean: ## Xoa code sinh tu proto
	rm -rf $(GEN_DIR)

.PHONY: grpc-list
grpc-list: ## Liet ke RPC qua reflection (can grpcurl)
	grpcurl -plaintext localhost:9090 list

# ---------- build / run ----------
.PHONY: build
build: ## Build binary vao bin/
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/server ./cmd/server
	CGO_ENABLED=0 go build -trimpath -o $(BIN_DIR)/token ./cmd/token

.PHONY: run
run: ## Chay server (doc .env)
	go run ./cmd/server

.PHONY: token
token: ## Sinh JWT de test: make token SUB=user-001 NAME="Nguyen A" ROLE=designer
	go run ./cmd/token -sub "$(or $(SUB),user-001)" -name "$(or $(NAME),Dev User)" -role "$(or $(ROLE),designer)"

.PHONY: cms-check
cms-check: ## Kiem tra ket noi CMS (JWKS, Workspace API, webhook HMAC, Bot API)
	@./scripts/check-cms.sh

.PHONY: route
route: ## Tra URL -> route -> handler -> service: make route URL=/api/client/v1/chat/auth/me
	@test -n "$(URL)$(path)" || (echo 'Dung: make route URL=/api/client/v1/chat/auth/me'; exit 1)
	@./scripts/find-route.sh "$(or $(URL),$(path))"

.PHONY: demo
demo: ## In duong dan giao dien live + token de dan vao: make demo [CMS_USER=6]
	@echo "Giao dien live: http://localhost:8080/demo/live.html"
	@if [ -n "$(CMS_USER)" ]; then echo "Token (user CMS $(CMS_USER)):"; $(MAKE) -s token-cms CMS_USER=$(CMS_USER); \
	 else echo "Token: make token-cms CMS_USER=<id>  (hoac dung token that cua CMS)"; fi

.PHONY: token-cms
token-cms: ## Token dev dong vai 1 user CMS da dong bo (chi dung o local): make token-cms CMS_USER=6
	@test -n "$(CMS_USER)" || (echo "Thieu CMS_USER (vd: make token-cms CMS_USER=6)"; exit 1)
	@info=$$(docker compose -f docker-compose.dev.yml exec -T postgres psql -U chat -d chat -tA \
	  -c "SELECT open_id || '|' || display_name FROM users WHERE cms_user_id=$(CMS_USER);" | tr -d '\r'); \
	test -n "$$info" || (echo "Khong thay user cms_user_id=$(CMS_USER) trong Chat (chay dong bo truoc)"; exit 1); \
	go run ./cmd/token -sub "$${info%%|*}" -name "$${info##*|}" -role designer -ttl 24h

.PHONY: seed
seed: ## Dung du lieu demo de test (3 user, 1 nhom, reply/thread, 1 yeu cau) + in token
	go run ./cmd/seeddemo -base "$(or $(BASE),http://localhost:8080)"

.PHONY: ws
ws: ## WS client dong lenh de test chat: make ws SUB=an NAME=An CONV=<conversation_id>
	@test -n "$(CONV)" || echo "(chua co CONV -> chi lang nghe, khong gui duoc tin nhan)"
	go run ./cmd/wsclient -sub "$(or $(SUB),wsclient)" -name "$(or $(NAME),WS Client)" -conv "$(CONV)"

# ---------- chat luong ----------
.PHONY: test
test: ## Chay unit test
	go test ./... -race -count=1

.PHONY: cover
cover: ## Test + bao cao coverage HTML
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ mo coverage.html"

.PHONY: lint
lint: ## Chay golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## gofmt toan bo code
	gofmt -w .
	go vet ./...

# ---------- migration ----------
.PHONY: migrate-up
migrate-up: ## Chay migration len moi nhat
	migrate -path $(MIGRATE_DIR) -database "$(POSTGRES_DSN)" up

.PHONY: migrate-down
migrate-down: ## Rollback 1 buoc
	migrate -path $(MIGRATE_DIR) -database "$(POSTGRES_DSN)" down 1

.PHONY: migrate-status
migrate-status: ## Xem version migration hien tai
	migrate -path $(MIGRATE_DIR) -database "$(POSTGRES_DSN)" version

.PHONY: migrate-create
migrate-create: ## Tao migration moi: make migrate-create name=add_field_x
	@test -n "$(name)" || (echo "Thieu name. Vi du: make migrate-create name=add_pinned_to_messages"; exit 1)
	migrate create -ext sql -dir $(MIGRATE_DIR) -seq $(name)

.PHONY: migrate-force
migrate-force: ## Go co dirty: make migrate-force version=1
	@test -n "$(version)" || (echo "Thieu version"; exit 1)
	migrate -path $(MIGRATE_DIR) -database "$(POSTGRES_DSN)" force $(version)

# --- phim tat cho migration (ten dai o tren van dung duoc) ---
# Co y KHONG lam tat 1 ky tu cho rollback: `md` canh `mu` tren ban phim,
# go lech 1 phim la mat du lieu.
.PHONY: mu ms mn mdown mforce
mu: migrate-up            ## = migrate-up (chay migration len moi nhat)
ms: migrate-status        ## = migrate-status (xem version hien tai)
mn: migrate-create        ## = migrate-create (migration moi): make mn name=add_field_x
mdown: migrate-down       ## = migrate-down (rollback 1 buoc)
mforce: migrate-force     ## = migrate-force: make mforce version=1

# ---------- postman ----------
.PHONY: postman
postman: ## Sinh lai 4 file Postman JSON (ban day du + [FE] + [CMS] + environment)
	python3 scripts/gen_postman.py

.PHONY: postman-run
postman-run: ## Chay cac folder luon dung duoc (chat, tin nhan, loi): make postman-run TOKEN=<jwt> [USER2=<uuid>]
	@test -n "$(TOKEN)" || (echo "Thieu TOKEN. Vi du: make postman-run TOKEN=$$(make -s token)"; exit 1)
	postman collection run internal/postman/chat-service.postman_collection.json \
	  -e internal/postman/chat-service.local.postman_environment.json \
	  --env-var "token=$(TOKEN)" --env-var "user_id_2=$(or $(USER2),)" \
	  -i "00 · Bắt đầu" -i "01 · Hội thoại" -i "02 · Tin nhắn" \
	  -i "07 · Lỗi & quyền (xem cách API phản hồi)" -i "99 · Dọn dẹp (chạy cuối, tuỳ ý)"

.PHONY: postman-run-cms
postman-run-cms: ## Chay 2 folder CMS (can CMS_ENABLED=true): make postman-run-cms TOKEN=<cms jwt> KEY=<service key> CHAT_ID=wg_...
	@test -n "$(TOKEN)" || (echo "Thieu TOKEN (access token do CMS phat)"; exit 1)
	@test -n "$(KEY)" || (echo "Thieu KEY (= CHAT_SERVICE_KEY)"; exit 1)
	postman collection run internal/postman/chat-service.postman_collection.json \
	  -e internal/postman/chat-service.local.postman_environment.json \
	  --env-var "base_url=$(or $(BASE_URL),http://localhost:8080)" \
	  --env-var "token=$(TOKEN)" --env-var "cms_token=$(TOKEN)" \
	  --env-var "service_key=$(KEY)" \
	  --env-var "group_id=$(CHAT_ID)" --env-var "cms_chat_id=$(CHAT_ID)" \
	  -i "08 · CMS → Chat (Bot API)" -i "09 · API cho FE (hợp đồng CMS)"

.PHONY: postman-flow
postman-flow: ## Chay folder FLOW theo thu tu: make postman-flow TOKEN=<jwt> [KEY=<service key>] [STOP=2.3]
	@test -n "$(TOKEN)" || (echo 'Thieu TOKEN (vd: make postman-flow TOKEN=$$(make -s token SUB=demo-an))'; exit 1)
	postman collection run internal/postman/chat-service.postman_collection.json \
	  -e internal/postman/chat-service.local.postman_environment.json \
	  --env-var "base_url=$(or $(BASE_URL),http://localhost:8080)" \
	  --env-var "token=$(TOKEN)" --env-var "cms_token=$(or $(CMS_TOKEN),$(TOKEN))" \
	  --env-var "service_key=$(KEY)" --env-var "stop_at=$(STOP)" \
	  -i "FLOW · Chạy từ trên xuống để hiểu luồng"

.PHONY: postman-fe
postman-fe: ## Chay ban [FE] (chi API web chat goi): make postman-fe TOKEN=<cms jwt>
	@test -n "$(TOKEN)" || (echo 'Thieu TOKEN (vd: make postman-fe TOKEN=$$(make -s token-cms CMS_USER=6))'; exit 1)
	postman collection run internal/postman/chat-service-fe.postman_collection.json \
	  -e internal/postman/chat-service.local.postman_environment.json \
	  --env-var "base_url=$(or $(BASE_URL),http://localhost:8080)" \
	  --env-var "token=$(TOKEN)" --env-var "cms_token=$(TOKEN)" \
	  --env-var "stop_at=$(STOP)" \
	  -i "FLOW · Luồng của FE"

.PHONY: postman-cms
postman-cms: ## Chay ban [CMS] (chi API CMS goi sang): make postman-cms KEY=<service key>
	@test -n "$(KEY)" || (echo "Thieu KEY (= CHAT_SERVICE_KEY trong .env)"; exit 1)
	postman collection run internal/postman/chat-service-cms.postman_collection.json \
	  -e internal/postman/chat-service.local.postman_environment.json \
	  --env-var "base_url=$(or $(BASE_URL),http://localhost:8080)" \
	  --env-var "service_key=$(KEY)" --env-var "stop_at=$(STOP)" \
	  -i "FLOW · Luồng của CMS"

.PHONY: postman-lint
postman-lint: ## Kiem tra file collection JSON (cau truc + khong lech voi generator)
	python3 scripts/gen_postman.py --check

# ---------- docker ----------
.PHONY: docker-build
docker-build: ## Build docker image
	docker build -t $(DOCKER_IMG) .

.PHONY: dev-up
dev-up: ## [DEV] Chay source trong docker, hot reload khi sua code (air)
	docker compose -f docker-compose.dev.yml up -d --build
	docker compose -f docker-compose.dev.yml ps

.PHONY: dev-logs
dev-logs: ## [DEV] Xem log app dev (co log build cua air)
	docker compose -f docker-compose.dev.yml logs -f app

.PHONY: dev-down
dev-down: ## [DEV] Dung stack dev
	docker compose -f docker-compose.dev.yml down

.PHONY: dev-down-v
dev-down-v: ## [DEV] Dung stack dev + xoa volume (mat du lieu + cache)
	docker compose -f docker-compose.dev.yml down -v

.PHONY: dev-sh
dev-sh: ## [DEV] Vao shell container app (chay go test, make proto... ben trong)
	docker compose -f docker-compose.dev.yml exec app sh

.PHONY: dev-test
dev-test: ## [DEV] Chay unit test ben trong container
	docker compose -f docker-compose.dev.yml exec app go test ./... -count=1

.PHONY: dev-migrate
dev-migrate: ## [DEV] Chay lai migration trong stack dev
	docker compose -f docker-compose.dev.yml run --rm migrate

.PHONY: dev-psql
dev-psql: ## [DEV] Mo psql cua stack dev
	docker compose -f docker-compose.dev.yml exec postgres psql -U chat -d chat

.PHONY: up
up: ## Chay toan bo stack (postgres + redis + app) bang docker compose
	docker compose up -d --build
	docker compose ps

.PHONY: up-deps
up-deps: ## Chi chay postgres + redis (app chay bang `make run`)
	docker compose up -d postgres redis

.PHONY: prod-up
prod-up: ## [PROD] Chay stack that tren VPS (doc .env, khong mo cong DB ra ngoai)
	@test -f .env || (echo "Chua co .env — copy tu .env.production.example roi dien bi mat"; exit 1)
	docker compose -f docker-compose.prod.yml up -d --build
	docker compose -f docker-compose.prod.yml ps

.PHONY: prod-logs
prod-logs: ## [PROD] Xem log app
	docker compose -f docker-compose.prod.yml logs -f app

.PHONY: prod-down
prod-down: ## [PROD] Dung stack that (giu du lieu)
	docker compose -f docker-compose.prod.yml down

.PHONY: prod-migrate
prod-migrate: ## [PROD] Chay migration (tu dong chay khi prod-up, lenh nay de chay lai)
	docker compose -f docker-compose.prod.yml run --rm migrate

.PHONY: prod-backup
prod-backup: ## [PROD] Dump database ra file: make prod-backup OUT=backup.sql
	docker compose -f docker-compose.prod.yml exec -T postgres \
	  pg_dump -U "$${POSTGRES_USER:-chat}" "$${POSTGRES_DB:-chat}" > $(or $(OUT),backup-$(shell date +%Y%m%d-%H%M%S).sql)

.PHONY: down
down: ## Dung stack
	docker compose down

.PHONY: down-v
down-v: ## Dung stack + xoa volume (mat du lieu)
	docker compose down -v

.PHONY: logs
logs: ## Xem log app
	docker compose logs -f app

.PHONY: log-file
log-file: ## Theo doi LOG_FILE o local: make log-file [FILE=./logs/app.log]
	@f="$(or $(FILE),./logs/app.log)"; \
	test -f "$$f" || { echo "Chua co $$f. Dat LOG_FILE=./logs/app.log trong .env roi chay lai app."; exit 1; }; \
	tail -f "$$f"

.PHONY: log-grep
log-grep: ## Tim trong LOG_FILE: make log-grep Q=groups/refresh [FILE=./logs/app.log]
	@test -n "$(Q)" || { echo 'Dung: make log-grep Q=groups/refresh'; exit 1; }
	@f="$(or $(FILE),./logs/app.log)"; \
	test -f "$$f" || { echo "Chua co $$f."; exit 1; }; \
	grep --color=always -- "$(Q)" "$$f" | tail -50

.PHONY: sql-on
sql-on: ## Bat in cau SQL ra console docker (sua .env roi nap lai app dev)
	@./scripts/env-set.sh POSTGRES_LOG_SQL true
	@# Phai truyen thang gia tri: Makefile co `include .env` + `export` nen compose
	@# se uu tien bien da export TU LUC make khoi dong, tuc gia tri CU.
	@POSTGRES_LOG_SQL=true docker compose -f docker-compose.dev.yml up -d --force-recreate --wait app >/dev/null
	@echo "SQL log: BAT. Xem bang: make dev-logs"

.PHONY: sql-off
sql-off: ## Tat in cau SQL, console tro lai sach
	@./scripts/env-set.sh POSTGRES_LOG_SQL false
	@# Phai truyen thang gia tri: Makefile co `include .env` + `export` nen compose
	@# se uu tien bien da export TU LUC make khoi dong, tuc gia tri CU.
	@POSTGRES_LOG_SQL=false docker compose -f docker-compose.dev.yml up -d --force-recreate --wait app >/dev/null
	@echo "SQL log: TAT."

.PHONY: redis-cli
redis-cli: ## Mo redis-cli (tu dung REDIS_PASSWORD trong .env neu co)
	docker compose exec redis sh -c 'redis-cli $${REDIS_PASSWORD:+-a "$$REDIS_PASSWORD"}'

.PHONY: psql
psql: ## Mo psql trong container postgres
	docker compose exec postgres psql -U chat -d chat

# ---------- tien ich ----------
.PHONY: clean
clean: ## Xoa binary + file coverage + tmp cua air
	rm -rf $(BIN_DIR) tmp coverage.out coverage.html
