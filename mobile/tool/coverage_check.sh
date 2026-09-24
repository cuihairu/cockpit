#!/usr/bin/env bash
# 覆盖率收口检查：flutter test --coverage 后解析 lcov，
# 未覆盖行必须全部登记在 known_uncoverable.txt（附理由），否则失败。
# 用法：mobile/tool/coverage_check.sh [--skip-test]（已有 lcov 时跳过跑测试）
set -euo pipefail
cd "$(dirname "$0")/.."

FLUTTER="${FLUTTER:-$HOME/development/flutter-3.47.5/bin/flutter}"

if [[ "${1:-}" != "--skip-test" ]]; then
  "$FLUTTER" test --coverage
fi

LCOV=coverage/lcov.info
KNOWN=tool/known_uncoverable.txt
[[ -f "$LCOV" ]] || { echo "缺少 $LCOV（先跑 flutter test --coverage）" >&2; exit 1; }
[[ -f "$KNOWN" ]] || { echo "缺少 $KNOWN" >&2; exit 1; }

# 未覆盖行 → "path:line" 列表；路径归一到 lib/ 开头
missed=$(awk -F'[,:]' '
  /^SF:/ { file=$2 }
  /^DA:/ { if ($3 == 0) print file ":" $2 }
' "$LCOV" | sed 's#^lib/##' | sort -u)

registered=$(grep -v '^\s*#' "$KNOWN" | grep -v '^\s*$' \
  | awk '{print $1}' | sed 's#^lib/##' | sort -u)

total_missed=$(printf '%s\n' "$missed" | sed '/^$/d' | wc -l)
total_known=$(printf '%s\n' "$registered" | sed '/^$/d' | wc -l)

# 未登记的缺行 = missed - registered
unregistered=$(comm -23 <(printf '%s\n' "$missed" | sed '/^$/d') \
  <(printf '%s\n' "$registered" | sed '/^$/d'))

# 登记了但实际已覆盖 = registered - missed（登记表过期，提醒清理）
stale=$(comm -13 <(printf '%s\n' "$missed" | sed '/^$/d') \
  <(printf '%s\n' "$registered" | sed '/^$/d'))

lf=$(awk -F: '/^LF:/ { s += $2 } END { print s + 0 }' "$LCOV")
lh=$(awk -F: '/^LH:/ { s += $2 } END { print s + 0 }' "$LCOV")
printf '行覆盖：%d/%d（%.2f%%），未覆盖 %d 行，已登记 %d 行\n' \
  "$lh" "$lf" "$(awk "BEGIN { printf \"%.2f\", 100 * $lh / $lf }")" \
  "$total_missed" "$total_known"

if [[ -n "$stale" ]]; then
  echo '提示：以下登记行实际已覆盖，可从登记表移除：'
  printf '%s\n' "$stale" | sed 's/^/  /'
fi

if [[ -n "$unregistered" ]]; then
  echo '失败：存在未登记的未覆盖行：' >&2
  printf '%s\n' "$unregistered" | sed 's/^/  /' >&2
  exit 1
fi

echo '通过：全部未覆盖行均已登记 KNOWN_UNCOVERABLE（有效覆盖 100%）'
