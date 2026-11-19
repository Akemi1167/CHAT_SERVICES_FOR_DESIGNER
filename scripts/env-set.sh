#!/usr/bin/env sh
# Doi gia tri 1 bien trong .env, giu nguyen comment cuoi dong.
# Dung: ./scripts/env-set.sh POSTGRES_LOG_SQL true [file]
set -e

key="$1"
val="$2"
file="${3:-.env}"

if [ -z "$key" ] || [ -z "$val" ]; then
  echo "Dung: $0 <KEY> <VALUE> [file]" >&2
  exit 1
fi
if [ ! -f "$file" ]; then
  echo "Khong thay $file" >&2
  exit 1
fi

# Chua co bien thi them vao cuoi, co roi thi thay gia tri.
if ! grep -q "^$key=" "$file"; then
  printf '%s=%s\n' "$key" "$val" >> "$file"
  echo "$key=$val (them moi vao $file)"
  exit 0
fi

tmp=$(mktemp)
awk -v k="$key" -v v="$val" '
  index($0, k "=") == 1 {
    c = index($0, "#")
    # KHONG dem khoang trang sau gia tri: docker compose giu lai lam gia tri bi ban.
    if (c > 0) printf "%s=%s  %s\n", k, v, substr($0, c)
    else       printf "%s=%s\n", k, v
    next
  }
  { print }
' "$file" > "$tmp" && mv "$tmp" "$file"

echo "$key=$val"
