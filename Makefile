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

# ---------- postman ----------
.PHONY: postman
postman: ## Sinh lai Postman collection (file v2.1 de import + ban v3 cho CLI)
	python3 scripts/gen_postman.py

.PHONY: postman-run
postman-run: ## Chay collection (tru folder CMS): make postman-run TOKEN=<jwt> [USER2=<uuid>]
	@test -n "$(TOKEN)" || (echo "Thieu TOKEN. Vi du: make postman-run TOKEN=$$(make -s token)"; exit 1)
	postman collection run postman/chat-service.postman_collection.json \
	  -e postman/chat-service.local.postman_environment.json \
	  --env-var "token=$(TOKEN)" --env-var "user_id_2=$(or $(USER2),)" \
	  -i "00 · Bắt đầu" -i "01 · Hội thoại" -i "02 · Tin nhắn" -i "03 · Tổ & gợi ý người phụ trách" \
	  -i "04 · Bot @bot & gửi mẫu" -i "05 · Task (yêu cầu / nhận việc)" -i "06 · Phiếu sửa" \
	  -i "07 · Lỗi & quyền (xem cách API phản hồi)" -i "99 · Dọn dẹp (chạy cuối, tuỳ ý)"

.PHONY: postman-run-cms
postman-run-cms: ## Chay 2 folder CMS (can CMS_ENABLED=true): make postman-run-cms TOKEN=<cms jwt> KEY=<service key> CHAT_ID=wg_...
	@test -n "$(TOKEN)" || (echo "Thieu TOKEN (access token do CMS phat)"; exit 1)
	@test -n "$(KEY)" || (echo "Thieu KEY (= CHAT_SERVICE_KEY)"; exit 1)
	postman collection run postman/chat-service.postman_collection.json \
	  -e postman/chat-service.local.postman_environment.json \
	  --env-var "base_url=$(or $(BASE_URL),http://localhost:8080)" \
	  --env-var "token=$(TOKEN)" --env-var "cms_token=$(TOKEN)" \
	  --env-var "service_key=$(KEY)" --env-var "chat_id=$(CHAT_ID)" \
	  -i "08 · CMS → Chat (Bot API)" -i "09 · API cho FE (hợp đồng CMS)"

.PHONY: postman-lint
postman-lint: ## Lint ban v3 (gom ca request WebSocket/gRPC)
	postman collection lint "postman/collections/chat-service"
	postman environment lint "postman/environments/Chat Service Local.environment.yaml"

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

.PHONY: psql
psql: ## Mo psql trong container postgres
	docker compose exec postgres psql -U chat -d chat

# ---------- tien ich ----------
.PHONY: clean
clean: ## Xoa binary + file coverage + tmp cua air
	rm -rf $(BIN_DIR) tmp coverage.out coverage.html
