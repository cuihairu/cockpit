#!/usr/bin/env bash
# 覆盖率收口检查：vitest --coverage 后解析 lcov，
# 未覆盖行与未覆盖分支必须全部登记在 known_uncoverable.txt（附理由），否则失败。
# 用法：web/tool/coverage_check.sh [--skip-test]（已有 lcov 时跳过跑测试）
set -euo pipefail
export LC_ALL=C  # comm 依赖字节序，sort 默认 locale 排序与其不一致
cd "$(dirname "$0")/.."

if [[ "${1:-}" != "--skip-test" ]]; then
  pnpm vitest run --coverage.enabled=true --coverage.reporter=lcov --coverage.include='src/**/*.{ts,tsx}'
fi

LCOV=coverage/lcov.info
KNOWN=tool/known_uncoverable.txt
[[ -f "$LCOV" ]] || { echo "缺少 $LCOV（先跑 vitest --coverage）" >&2; exit 1; }
[[ -f "$KNOWN" ]] || { echo "缺少 $KNOWN" >&2; exit 1; }

# lcov 新鲜度：覆盖结论必须基于本轮源码。沿用上轮残留 lcov（未重跑
# vitest --coverage）等于把上一版覆盖算成现在——同 Go 端 2026-10-11
# 假绿（本地残留 coverage.out 校验新仓库，COV_RC=0 假绿推送后 CI 红）。
# 只比本 lcov 覆盖到的文件（SF 里没有的文件不算过期）。
stale_src=$(comm -12 \
  <(find src -type f \( -name '*.ts' -o -name '*.tsx' \) -newer "$LCOV" -printf '%p\n' 2>/dev/null | sort -u) \
  <(awk -F: '/^SF:/ { print $2 }' "$LCOV" | sort -u))
if [[ -n "$stale_src" ]]; then
  echo "失败：lcov 早于以下源文件的改动，覆盖结论不可信（沿用上轮残留 lcov）" >&2
  printf '%s\n' "$stale_src" | sed 's/^/  /' >&2
  echo "修复：重跑 pnpm vitest run --coverage.enabled=true --coverage.reporter=lcov --coverage.include='src/**/*.{ts,tsx}' 后再校验" >&2
  exit 1
fi

# 未覆盖行（DA:0）与未覆盖分支（BRDA taken 为 0 或 '-'）→ "path:line" 列表
missed=$(awk -F'[,:]' '
  /^SF:/ { file=$2 }
  /^DA:/ { if ($3 == 0) print file ":" $2 }
  /^BRDA:/ { if ($5 == 0 || $5 == "-") print file ":" $2 }
' "$LCOV" | sed 's#^src/##' | sort -u)

registered=$(grep -v '^\s*#' "$KNOWN" | grep -v '^\s*$' \
  | awk '{print $1}' | sed 's#^src/##' | sort -u)

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
brf=$(awk -F: '/^BRF:/ { s += $2 } END { print s + 0 }' "$LCOV")
brh=$(awk -F: '/^BRH:/ { s += $2 } END { print s + 0 }' "$LCOV")
printf '行覆盖：%d/%d（%.2f%%），分支覆盖：%d/%d（%.2f%%）\n' \
  "$lh" "$lf" "$(awk "BEGIN { printf \"%.2f\", 100 * $lh / $lf }")" \
  "$brh" "$brf" "$(awk "BEGIN { if ($brf == 0) print 100; else printf \"%.2f\", 100 * $brh / $brf }")"
printf '未覆盖行/分支点：%d，已登记：%d\n' "$total_missed" "$total_known"

if [[ -n "$stale" ]]; then
  echo '提示：以下登记行实际已覆盖，可从登记表移除：'
  printf '%s\n' "$stale" | sed 's/^/  /'
fi

if [[ -n "$unregistered" ]]; then
  echo '失败：存在未登记的未覆盖行/分支：' >&2
  printf '%s\n' "$unregistered" | sed 's/^/  /' >&2
  exit 1
fi

echo '通过：全部未覆盖行/分支均已登记 KNOWN_UNCOVERABLE（有效覆盖 100%）'
