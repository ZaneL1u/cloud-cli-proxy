-include .env
export

DEV_COMPOSE := docker compose -f deploy/compose/control-plane.dev.yml

.PHONY: dev dev-api dev-web db test test-go test-smoke build clean gateway-image up up-build rebuild-runners rebuild-runners-running rebuild-runners-non-running down logs release

# ── Development ──────────────────────────────────────────────

dev: ## Start backend + frontend (requires PostgreSQL running)
	@echo "Starting control-plane + admin frontend..."
	@echo "  API  → http://$(CONTROL_PLANE_ADDR)"
	@echo "  Web  → http://localhost:5173"
	@echo "  Agent → embedded (in-process)"
	@echo ""
	@mkdir -p .data
	@trap 'kill 0' EXIT; \
		HOST_AGENT_MODE=embedded DATA_DIR=$(CURDIR)/.data go run ./cmd/control-plane & \
		cd web/admin && pnpm dev & \
		wait

dev-api: ## Start backend only (with embedded host-agent)
	@mkdir -p .data
	HOST_AGENT_MODE=embedded DATA_DIR=$(CURDIR)/.data go run ./cmd/control-plane

dev-web: ## Start frontend only
	cd web/admin && pnpm dev

# ── Database ─────────────────────────────────────────────────

db: ## Start PostgreSQL via Docker Compose
	$(DEV_COMPOSE) up -d postgres
	@echo "Waiting for PostgreSQL on port $(POSTGRES_PORT)..."
	@until pg_isready -h 127.0.0.1 -p $(POSTGRES_PORT) -U $(POSTGRES_USER) -d $(POSTGRES_DB) > /dev/null 2>&1 || $(DEV_COMPOSE) exec -T postgres pg_isready -U $(POSTGRES_USER) -d $(POSTGRES_DB) > /dev/null 2>&1; do sleep 1; done
	@echo "PostgreSQL ready."

db-stop: ## Stop PostgreSQL
	$(DEV_COMPOSE) down

db-reset: ## Reset database (destroy volume and restart)
	$(DEV_COMPOSE) down -v
	$(MAKE) db

# ── Testing ──────────────────────────────────────────────────

test: test-go test-smoke ## Run all tests

test-go: ## Run Go tests
	go test ./... -count=1

test-smoke: ## Run BATS bootstrap smoke tests
	npx bats tests/smoke/

# ── Images ────────────────────────────────────────────────────

MANAGED_USER_TAG := $(shell grep '^image_name:' deploy/docker/managed-user/image.lock | cut -d' ' -f2)

user-image: ## Build managed-user image (cloud desktop)
	docker build -t $(MANAGED_USER_TAG) -f deploy/docker/managed-user/Dockerfile .
	@echo "Built: $(MANAGED_USER_TAG)"

gateway-image: ## Build sing-box + iptables sidecar (required for macOS/Windows host-agent egress)
	docker build -t cloud-cli-proxy-sing-gateway:local -f deploy/docker/sing-box-gateway/Dockerfile .

# ── Build ────────────────────────────────────────────────────

build: ## Build all artifacts
	go build -o bin/control-plane ./cmd/control-plane
	GOOS=linux GOARCH=amd64 go build -o bin/host-agent ./cmd/host-agent
	cd web/admin && pnpm build

build-api: ## Build Go backend only
	go build -o bin/control-plane ./cmd/control-plane

build-web: ## Build frontend only
	cd web/admin && pnpm build

# ── Production ───────────────────────────────────────────────

up: ## Start production stack (prefer prebuilt latest images)
	docker compose pull --policy always
	docker compose up -d

up-build: ## Start production stack from local source build
	docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
	docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate

rebuild-runners: ## Queue rebuild for all managed hosts (running + non-running)
	@ADMIN_USERNAME="$${ADMIN_USERNAME:-admin}" \
	ADMIN_PASSWORD="$${ADMIN_PASSWORD:?Set ADMIN_PASSWORD in .env or env}" \
	bash deploy/scripts/rebuild-host-runners.sh \
		--scope all

rebuild-runners-running: ## Queue rebuild for hosts whose Docker status is running
	@ADMIN_USERNAME="$${ADMIN_USERNAME:-admin}" \
	ADMIN_PASSWORD="$${ADMIN_PASSWORD:?Set ADMIN_PASSWORD in .env or env}" \
	bash deploy/scripts/rebuild-host-runners.sh \
		--scope running

rebuild-runners-non-running: ## Queue rebuild for hosts whose Docker status is non-running
	@ADMIN_USERNAME="$${ADMIN_USERNAME:-admin}" \
	ADMIN_PASSWORD="$${ADMIN_PASSWORD:?Set ADMIN_PASSWORD in .env or env}" \
	bash deploy/scripts/rebuild-host-runners.sh \
		--scope non-running

down: ## Stop production stack
	docker compose down

logs: ## Tail production logs
	docker compose logs -f

release: ## Create and push release tag (usage make release VERSION=1.5.0)
	@test -n "$(VERSION)" || (echo "VERSION is required. Example: make release VERSION=1.5.0" && exit 1)
	@test -z "$$(git status --porcelain)" || (echo "Working tree is dirty. Commit/stash changes before release." && exit 1)
	git tag v$(VERSION)
	git push origin v$(VERSION)

# ── Setup ────────────────────────────────────────────────────

setup: ## First-time setup: install deps, copy .env
	cd web/admin && pnpm install
	@test -f .env || cp .env.example .env
	@echo "Done. Edit .env if needed, then run: make db && make dev"
	@echo "提示：在 macOS/Windows 上调试主机出口代理（sidecar 网关）前请先执行一次: make gateway-image"

# ── Utilities ────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf bin/ web/admin/dist/

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' Makefile | sed 's/:.*## /: /' | awk 'BEGIN {FS = ": "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
