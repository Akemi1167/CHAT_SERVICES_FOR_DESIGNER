#!/usr/bin/env bash
# Kiem tra tung duong noi giua Chat va CMS. Chay sau khi dien .env:
#   make cms-check
# Doc bien tu .env (hoac bien moi truong dang co).
set -uo pipefail

# Doc .env an toan: bo qua dong comment/rong, tu them dau nhay neu thieu.
if [ -f .env ]; then
  while IFS= read -r line; do
    case "$line" in ''|\#*) continue;; *=*) ;; *) continue;; esac
    key="${line%%=*}"; val="${line#*=}"
    val="${val%\"}"; val="${val#\"}"
    export "$key=$val"
  done < .env
fi

CHAT_BASE="${CHAT_BASE:-http://localhost:8080}"
OK=0; FAIL=0
pass() { printf "  \033[32m✓\033[0m %s\n" "$1"; OK=$((OK+1)); }
fail() { printf "  \033[31m✗\033[0m %s\n" "$1"; [ -n "${2:-}" ] && printf "      %s\n" "$2"; FAIL=$((FAIL+1)); }
info() { printf "  · %s\n" "$1"; }
section() { printf "\n\033[1m%s\033[0m\n" "$1"; }

section "0. Cau hinh dang doc duoc"
info "CMS_ENABLED            = ${CMS_ENABLED:-(trong)}"
info "CMS_WORKSPACE_BASE_URL = ${CMS_WORKSPACE_BASE_URL:-(trong)}"
info "CMS_WEBHOOK_URL        = ${CMS_WEBHOOK_URL:-(trong)}"
if [ -n "${CHAT_SERVICE_KEY:-}" ]; then
  info "CHAT_SERVICE_KEY       = (da dien, ${#CHAT_SERVICE_KEY} ky tu, khong in ra)"
else
  info "CHAT_SERVICE_KEY       = (TRONG)"
fi
info "Chat base              = $CHAT_BASE"

if [ -z "${CHAT_SERVICE_KEY:-}" ] || [ -z "${CMS_WORKSPACE_BASE_URL:-}" ]; then
  fail "Thieu CHAT_SERVICE_KEY hoac CMS_WORKSPACE_BASE_URL trong .env" \
       "Xin doi CMS 2 gia tri nay roi dien vao .env (xem docs/deploy.md#cutover-bật-tích-hợp-cms)."
  exit 1
fi

JWKS_URL="${CMS_JWKS_URL:-${CMS_WORKSPACE_BASE_URL}/.well-known/jwks.json}"

section "1. JWKS cua CMS (Chat dung de kiem token nguoi dung)"
JWKS=$(curl -s -m 10 -w '\n%{http_code}' "$JWKS_URL" 2>/dev/null)
CODE=$(printf '%s' "$JWKS" | tail -1); BODY=$(printf '%s' "$JWKS" | sed '$d')
if [ "$CODE" = "200" ]; then
  KIDS=$(printf '%s' "$BODY" | python3 -c "
import sys, json
keys = json.load(sys.stdin).get('keys', [])
print(', '.join('%s(%s)' % (k.get('kid'), k.get('alg')) for k in keys))" 2>/dev/null)
  [ -n "$KIDS" ] && pass "JWKS doc duoc: $KIDS" || fail "JWKS tra 200 nhung khong co khoa nao" "$BODY"
else
  fail "JWKS khong goi duoc ($JWKS_URL -> HTTP ${CODE:-loi mang})" \
       "Kiem tra CMS_WORKSPACE_BASE_URL / mang / firewall."
fi

section "2. Workspace API (Chat doc nhom + user, dung service key)"
RES=$(curl -s -m 10 -w '\n%{http_code}' -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" "${CMS_WORKSPACE_BASE_URL}/groups")
CODE=$(printf '%s' "$RES" | tail -1); BODY=$(printf '%s' "$RES" | sed '$d')
case "$CODE" in
  200) N=$(printf '%s' "$BODY" | python3 -c "
import sys, json
print(len(json.load(sys.stdin).get('element', {}).get('groups', [])))" 2>/dev/null || echo '?')
       pass "GET /groups OK — CMS tra $N nhom"
       printf '%s' "$BODY" | python3 -c "
import sys, json
for g in json.load(sys.stdin).get('element', {}).get('groups', [])[:3]:
    print('      - %-20s %-28s (%d thanh vien)' % (g.get('chatId'), g.get('name'), len(g.get('members', []))))" 2>/dev/null ;;
  401) fail "GET /groups -> 401: CHAT_SERVICE_KEY khong khop voi CMS" "Doi chieu lai khoa voi doi CMS." ;;
  503) fail "GET /groups -> 503: CMS chua cau hinh CHAT_SERVICE_KEY ben phia ho" ;;
  *)   fail "GET /groups -> HTTP ${CODE:-loi mang}" "$(printf '%.200s' "$BODY")" ;;
esac

section "3. Webhook Chat -> CMS (ky HMAC)"
if [ -z "${CMS_WEBHOOK_URL:-}" ]; then
  fail "Thieu CMS_WEBHOOK_URL"
else
  TS=$(date +%s)
  EV="evt_check_$(date +%s%N)"
  BODY_W="{\"schema\":\"abc.event/v1\",\"eventId\":\"$EV\",\"type\":\"user.updated\",\"createdAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"user\":{\"id\":\"check\"}}"
  SIG=$(printf '%s' "$TS.$BODY_W" | openssl dgst -sha256 -hmac "$CHAT_SERVICE_KEY" | awk '{print $2}')
  RES=$(curl -s -m 10 -w '\n%{http_code}|%{content_type}' -X POST "$CMS_WEBHOOK_URL" \
        -H 'Content-Type: application/json' -H "X-Abc-Timestamp: $TS" -H "X-Abc-Signature: $SIG" -d "$BODY_W")
  META=$(printf '%s' "$RES" | tail -1); OUT=$(printf '%s' "$RES" | sed '$d')
  CODE="${META%%|*}"; CTYPE="${META#*|}"
  # 200 + text/html = trang SPA tra ve, KHONG phai webhook that.
  if [ "$CODE" = "200" ] && [ "${CTYPE#*html}" != "$CTYPE" ]; then
    fail "Webhook tra 200 nhung la trang HTML (SPA), khong phai endpoint webhook" \
         "Duong dan nay khong phai webhook. Xin doi CMS webhookPath dung cua bot."
    CODE="html"
  fi
  case "$CODE" in
    html) ;;
    200) pass "CMS nhan event va chap nhan chu ky — tra: $(printf '%.80s' "$OUT")" ;;
    401) fail "Webhook -> 401: chu ky khong hop le" "Thuong do CHAT_SERVICE_KEY khac nhau 2 ben, hoac lech gio >5 phut (gio may: $(date -u +%H:%M:%SZ))." ;;
    404) if [ "${OUT#*Unknown webhook path}" != "$OUT" ]; then
           fail "Webhook -> 404 \"Unknown webhook path\": dung may chu, SAI doan path cua bot" \
                "Base /webhook/... la dung. Xin doi CMS gia tri webhookPath cua bot Web (CMS -> Bot -> Cai dat)."
         else
           fail "Webhook -> 404: sai duong dan webhook" "Xin doi CMS webhookPath dung cua bot."
         fi ;;
    *)   fail "Webhook -> HTTP ${CODE:-loi mang}" "$(printf '%.200s' "$OUT")" ;;
  esac
fi

section "4. Chat da bat tich hop chua"
H=$(curl -s -m 5 "$CHAT_BASE/healthz" -o /dev/null -w '%{http_code}')
if [ "$H" = "200" ]; then pass "Chat song ($CHAT_BASE)"; else fail "Chat khong tra loi ($CHAT_BASE -> ${H:-loi mang})" "Chay: make dev-up"; fi

BOT=$(curl -s -m 5 -X POST "$CHAT_BASE/bot/v1/users/resolve" -H 'Content-Type: application/json' -d '{"items":[]}' -o /dev/null -w '%{http_code}')
case "$BOT" in
  401) pass "Bot API da bat (tra 401 khi thieu service key — dung)" ;;
  404) fail "Bot API chua bat" "Dat CMS_ENABLED=true trong .env roi khoi dong lai Chat." ;;
  *)   info "Bot API tra HTTP $BOT" ;;
esac

BOTK=$(curl -s -m 5 -X POST "$CHAT_BASE/bot/v1/users/resolve" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
       -H 'Content-Type: application/json' -d '{"items":[]}' -o /dev/null -w '%{http_code}')
[ "$BOTK" = "200" ] && pass "Bot API chap nhan CHAT_SERVICE_KEY cua minh" \
  || info "Bot API voi service key tra HTTP $BOTK (200 la dung)"

section "Ket qua: $OK muc OK, $FAIL muc loi"
if [ "$FAIL" -gt 0 ]; then
  echo "  Xem docs/deploy.md#cutover-bật-tích-hợp-cms de biet can xin doi CMS thong tin gi."
  exit 1
fi
echo "  San sang noi. Buoc tiep: dat CMS_ENABLED=true, TASK_ENGINE_ENABLED=false, khoi dong lai."
