#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Chặn 2 job Jenkins chạy song song (@2) đụng độ Docker Swarm ("update out of sequence")
exec 9>/tmp/deploy_chat_services_for_designer.lock
if ! flock -n 9; then
  echo "⏳ Đang có một tiến trình deploy khác chạy, chờ đến lượt..."
  flock 9
fi

if [ ! -f .env ]; then
  echo "Thieu file .env!" >&2
  exit 1
fi

echo "1. Loading environment variables from .env..."
set -a
# shellcheck disable=SC1091
source <(sed -E 's/[[:space:]]+#.*$//' .env | grep -vE '^[[:space:]]*(#|$)')
set +a

echo "2. Pulling latest code..."
STASHED=0
if ! git diff --quiet; then
  git stash push -m "deploy.sh auto-stash"
  STASHED=1
fi
git pull --ff-only || true
if [ "$STASHED" -eq 1 ]; then
  git stash pop || true
fi

TAG="$(git rev-parse --short HEAD 2>/dev/null || date +%Y%m%d%H%M%S)"
IMAGE="chat_services_for_designer_app:${TAG}"
export TAG

echo "3. Building Docker image ${IMAGE}..."
docker build -t "$IMAGE" -t chat_services_for_designer_app:latest .
IMAGE_ID="$(docker image inspect -f '{{.Id}}' "$IMAGE")"

echo "4. Ensuring overlay networks exist..."
if ! docker network inspect chat_designer_network >/dev/null 2>&1; then
  docker network create -d overlay --attachable chat_designer_network
fi
if ! docker network inspect designer_bot_network >/dev/null 2>&1; then
  docker network create -d overlay --attachable designer_bot_network
fi

echo "5. Deploying Docker Swarm stack (chat_services_for_designer)..."
for attempt in 1 2 3; do
  if docker stack deploy --resolve-image never -c docker.swarm.yml chat_services_for_designer; then
    break
  fi
  echo "Swarm đang bận (update out of sequence), thử lại lần ${attempt}/3 sau 3s..."
  sleep 3
done

echo "6. Waiting for Postgres to be ready and running database migrations..."
PG_CONTAINER=""
for _ in $(seq 1 30); do
  PG_CONTAINER="$(docker ps -q -f name=chat_services_for_designer_postgres | head -n1)"
  if [ -n "$PG_CONTAINER" ] && docker exec "$PG_CONTAINER" pg_isready -U "${POSTGRES_USER:-chat}" -d "${POSTGRES_DB:-chat}" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

if [ -z "$PG_CONTAINER" ]; then
  echo "Postgres container chua san sang!" >&2
  exit 1
fi

docker run --rm \
  --network chat_designer_network \
  -v "${SCRIPT_DIR}/migrations:/migrations:ro" \
  migrate/migrate:v4.17.1 \
  -path=/migrations \
  -database="postgres://${POSTGRES_USER:-chat}:${POSTGRES_PASSWORD:-chat}@postgres:5432/${POSTGRES_DB:-chat}?sslmode=disable" \
  up || true

echo "7. Zero-downtime rolling update (order: start-first) & forcing new image ${IMAGE} (${IMAGE_ID})..."
docker service update \
  --force \
  --image "$IMAGE_ID" \
  --update-parallelism 1 \
  --update-delay 5s \
  --update-order start-first \
  --update-failure-action rollback \
  chat_services_for_designer_app

echo "8. Verifying service status and healthcheck..."
docker service ps chat_services_for_designer_app --filter "desired-state=running"
curl -fsS "http://127.0.0.1:${HTTP_PORT:-8080}/healthz" && echo ""
curl -fsS "http://127.0.0.1:${HTTP_PORT:-8080}/readyz" && echo ""

echo "Deploy CHAT_SERVICES_FOR_DESIGNER completed successfully (Zero Downtime)!"
