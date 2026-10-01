#!/usr/bin/env bash
# 覆盖率收口检查：解析 coverage.out，未覆盖语句块必须被
# tool/known_uncoverable.txt 里的某条登记覆盖（逐块比对或落入登记区间），
# 否则失败。
#
# 用法：tool/coverage_check.sh [coverage.out 路径]
#   默认 ./coverage.out（CI 与本地跑 go test -coverprofile 后的产物）
#
# 与 web/tool/coverage_check.sh、mobile 同名脚本同一口径：三端「有效覆盖
# 100%」——未覆盖即须登记，登记须带理由。差别在粒度：Go 覆盖率是语句块级
# （coverprofile 一行 = 一个块，startLine.startCol,endLine.endCol,numStmt,count），
# 本脚本以块的行区间与登记项比对。
#
# 登记项格式（<路径>:<起>,<止> <理由>）：
#   - 止 >= 起 时视为「区间」：块起始行落在区间内即视为已登记（用于成片豁免，
#     如探针 main() 场景编排）
#   - 起 == 止 时视为「单行」：块起始行等于该行
set -euo pipefail
export LC_ALL=C  # comm 依赖字节序
cd "$(dirname "$0")/.."

PROFILE="${1:-coverage.out}"
KNOWN=tool/known_uncoverable.txt
[[ -f "$PROFILE" ]] || { echo "缺少 $PROFILE（先跑 go test -coverprofile=$PROFILE ./...）" >&2; exit 1; }
[[ -f "$KNOWN" ]] || { echo "缺少 $KNOWN" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# 未覆盖块 → "path startLine endLine numStmt"，跳过 numStmt=0 的伪块
# （Go 对无语句块也产行，覆盖率分母为 0，算进去会误报）
# 同一块可能在 profile 中出现多行（外部测试包各自插桩同一源文件、
# 计数值不同）——按块聚合取最大计数：任一行 >0 即视为已覆盖，
# 否则会出现「0 与 >0 并存」的假未覆盖。
awk -v mod='github.com/cuihairu/cockpit/' '
  /^mode:/ { next }
  {
    if (index($0, mod) == 0) next
    loc = substr($0, index($0, mod) + length(mod))
    nstmt = $(NF-1); cnt = $NF
    if (nstmt + 0 == 0) next
    n = split(loc, a, ":")
    path = substr(loc, 1, length(loc) - length(a[n]) - 1)
    split(a[n], r, ",")
    split(r[1], s, "."); split(r[2], e, ".")
    key = path " " s[1] " " e[1]
    if (!(key in seen)) { seen[key] = 1; max[key] = cnt + 0; nst[key] = nstmt }
    else if (cnt + 0 > max[key]) { max[key] = cnt + 0 }
  }
  END {
    for (key in seen) if (max[key] == 0) print key " " nst[key]
  }
' "$PROFILE" | sort -u > "$tmp/missed"

# profile 中出现过的全部文件（含已覆盖）。stale 判定只针对本 profile 覆盖到的
# 文件，避免子集 profile（如 -tags rdp 双包）把无关登记项全报成 stale
awk -v mod='github.com/cuihairu/cockpit/' '
  /^mode:/ { next }
  index($0, mod) == 0 { next }
  {
    loc = substr($0, index($0, mod) + length(mod))
    n = split(loc, a, ":")
    print substr(loc, 1, length(loc) - length(a[n]) - 1)
  }
' "$PROFILE" | sort -u > "$tmp/profilepaths"

# 登记表 → "path start end"（登记项形如 path:start,end）
grep -v '^[[:space:]]*#' "$KNOWN" | grep -v '^[[:space:]]*$' \
  | awk '{
      key = $1
      n = split(key, a, ":")
      rng = a[n]
      split(rng, r, ",")
      path = substr(key, 1, length(key) - length(a[n]) - 1)
      print path, r[1], r[2]
    }' | sort -u -k1,1 -k2,2n > "$tmp/registered"

missed_n=$(wc -l < "$tmp/missed")
known_n=$(wc -l < "$tmp/registered")

# 未登记块：块起始行既不等于任一登记起点，也不落在任一登记区间内
unregistered=$(awk '
  NR == FNR { p[++nr] = $1; s[nr] = $2; e[nr] = $3; next }
  {
    hit = 0
    for (i = 1; i <= nr; i++) {
      if (p[i] != $1) continue
      if (s[i] == e[i]) { if ($2 == s[i]) { hit = 1; break } }
      else if ($2 >= s[i] && $2 <= e[i]) { hit = 1; break }
    }
    if (!hit) print $1 ":" $2 "," $3
  }
' "$tmp/registered" "$tmp/missed")

# 登记了但块内无未覆盖内容 = 过期（含漂移）
# 逐条登记项判定（键 path:起始行，非 path）：同一文件多条登记时，main()
# 区间有未覆盖块不能把同文件的闭包登记误判为「已覆盖」；且仅当文件出现在
# 本 profile 中才判定（子集 profile 不含的文件不报 stale）
stale=$(awk '
  FILENAME == ARGV[1] { p[++nr] = $1; s[nr] = $2; e[nr] = $3; next }
  FILENAME == ARGV[2] { files[$1] = 1; next }
  {
    for (i = 1; i <= nr; i++) {
      if (p[i] != $1) continue
      if (s[i] == e[i]) { if ($2 == s[i]) { covered[$1 ":" s[i]] = 1; break } }
      else if ($2 >= s[i] && $2 <= e[i]) { covered[$1 ":" s[i]] = 1; break }
    }
  }
  END {
    for (i = 1; i <= nr; i++) {
      if ((p[i] in files) && !((p[i] ":" s[i]) in covered)) print p[i] ":" s[i] "," e[i]
    }
  }
' "$tmp/registered" "$tmp/profilepaths" "$tmp/missed")

# 总语句数 / 已覆盖语句数：同样按块去重聚合（避免多行重复计数），
# 任一行 >0 即该块已覆盖
total_stmts=$(awk -v mod='github.com/cuihairu/cockpit/' '
  /^mode:/ { next }
  index($0, mod) == 0 { next }
  {
    nstmt = $(NF-1)
    if (nstmt + 0 == 0) next
    loc = substr($0, index($0, mod) + length(mod))
    n = split(loc, a, ":")
    path = substr(loc, 1, length(loc) - length(a[n]) - 1)
    split(a[n], r, ",")
    split(r[1], s, ".")
    key = path " " s[1]
    if (!(key in seen)) { seen[key] = 1; nst[key] = nstmt }
    if ($NF + 0 > 0) covered[key] = 1
  }
  END {
    t = 0; c = 0
    for (key in nst) { t += nst[key]; if (key in covered) c += nst[key] }
    printf "%d %d", t, c
  }' "$PROFILE")
read -r total_stmts covered_stmts <<< "$total_stmts"
missed_stmts=$(awk '{s += $4} END {print s+0}' "$tmp/missed")

printf '语句覆盖：%d/%d（%.2f%%）\n' "$covered_stmts" "$total_stmts" \
  "$(awk "BEGIN { printf \"%.2f\", 100 * $covered_stmts / $total_stmts }")"
printf '未覆盖块：%d（%d 语句），登记项：%d\n' "$missed_n" "$missed_stmts" "$known_n"

if [[ -n "$stale" ]]; then
  echo '提示：以下登记路径已无对应未覆盖块（可能已覆盖或区间漂移），请复核：'
  printf '%s\n' "$stale" | sed 's/^/  /'
fi

if [[ -n "$unregistered" ]]; then
  echo '失败：存在未登记的未覆盖语句块：' >&2
  printf '%s\n' "$unregistered" | sed 's/^/  /' >&2
  exit 1
fi

echo '通过：全部未覆盖语句块均已登记 known_uncoverable（有效覆盖 100%）'