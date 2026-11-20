#!/usr/bin/env bash
# Tra nhanh: tu duong dan URL -> noi khai bao route -> handler -> service.
#
#   ./scripts/find-route.sh /api/client/v1/chat/auth/me
#   make route URL=auth/me
#
# Dung URL= chu khong phai PATH= : PATH la bien moi truong cua shell, make se lay
# gia tri cua no thay vi doi so cua ban.
set -uo pipefail

RAW="${1:-}"
if [ -z "$RAW" ]; then
  echo "Dung: $0 <duong-dan hoac tu khoa>   (vd: /api/v1/messages/:id/replies)" >&2
  exit 1
fi

# Bo base_url / query / bien Postman, chi giu phan duong dan.
PATH_ONLY="${RAW%%\?*}"
PATH_ONLY="${PATH_ONLY#*://*/}"
PATH_ONLY="${PATH_ONLY#\{\{base_url\}\}}"

# Lay 2 doan cuoi lam tu khoa (vd auth/me, messages/:id/replies -> replies).
KEY="$(printf '%s' "$PATH_ONLY" | awk -F/ '{ if (NF>=2) printf "%s/%s", $(NF-1), $NF; else printf "%s", $NF }')"
LAST="$(printf '%s' "$PATH_ONLY" | awk -F/ '{print $NF}')"

# Route co tham so duoc khai bao dang ":formId", ":id"... nen doan giua cua URL
# that (vd "create_task") khong he xuat hien trong code. Dung them mot mau noi
# long: doan ap cuoi coi nhu tham so, vd forms/create_task/submit -> forms/[^/]*/submit
LOOSE=""
if [ "$(printf '%s' "$PATH_ONLY" | awk -F/ '{print NF}')" -ge 3 ]; then
  LOOSE="$(printf '%s' "$PATH_ONLY" | awk -F/ '{printf "%s/[^\"]*/%s", $(NF-2), $NF}')"
fi

PAT="\"(/)?${KEY}\"|\"(/)?${LAST}\""
[ -n "$LOOSE" ] && PAT="$PAT|\"(/)?${LOOSE}\""

echo "Tim: $PATH_ONLY   (tu khoa: \"$KEY\", \"$LAST\"${LOOSE:+, \"$LOOSE\"})"
echo

echo "1) Khai bao route"
grep -rn --include=*.go -E "$PAT" \
  internal/transport/httpserver/router.go \
  internal/transport/httpserver/clientapi \
  internal/transport/httpserver/botapi 2>/dev/null | sed 's/^/   /' || true
echo

echo "2) Handler (ham xu ly)"
HANDLERS=$(grep -rhn --include=*.go -E "$PAT" \
  internal/transport/httpserver 2>/dev/null \
  | grep -oE "h\.[A-Z][A-Za-z]*|[a-zA-Z]+H\.[A-Z][A-Za-z]*" | sed 's/.*\.//' | sort -u)
for h in $HANDLERS; do
  grep -rn --include=*.go "func (h \*[A-Za-z]*Handler) ${h}(\|func (h \*Handler) ${h}(" internal/transport | sed 's/^/   /'
done
echo

echo "3) Service duoc goi (nghiep vu)"
for h in $HANDLERS; do
  grep -rn --include=*.go -A 25 "func (h \*[A-Za-z]*Handler) ${h}(\|func (h \*Handler) ${h}(" internal/transport 2>/dev/null \
    | grep -oE "h\.(msgs|convs|cards|bots|tasks|notifs|sync|store)\.[A-Za-z]+" | sort -u | sed 's/^/   /'
done
echo
echo "Tang tiep theo: service -> repository (internal/repository) -> SQL (migrations/)."
