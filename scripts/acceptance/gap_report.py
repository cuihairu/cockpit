#!/usr/bin/env python3
"""覆盖率缺口报告（Go 语句级）。

用途：本轮覆盖率收口的判定工具——按「main() 场景编排 vs helpers」两段
拆分未覆盖语句块，helpers 段逐行列出行号（补测目标），main() 段只报总量
（真机编排，按台账口径登记豁免）。

用法：python3 scripts/acceptance/gap_report.py <coverage.out>
"""
import re
import sys
from collections import defaultdict

MOD = "github.com/cuihairu/cockpit/"


def main_go_line(path):
    """返回 main() 起始行（无则 0）。"""
    for i, line in enumerate(open(path), 1):
        if line.startswith("func main()"):
            return i
    return 0


def main():
    if len(sys.argv) != 2:
        print(__doc__)
        return 2
    cov_path = sys.argv[1]
    blocks = defaultdict(lambda: [0, 0])  # (file, func) -> [stmts, uncovered]
    helpers_uncovered = defaultdict(list)
    main_total = 0
    for raw in open(cov_path):
        raw = raw.strip()
        if not raw or raw.startswith("mode:"):
            continue
        loc, nstmt, cnt = raw.rsplit(" ", 2)
        path, rng = loc[len(MOD):].rsplit(":", 1)
        rng = rng.rstrip(":")
        start = int(rng.split(",")[0].split(".")[0])
        end = int(rng.split(",")[1].split(".")[0])
        n, c = int(nstmt), int(cnt)
        ms = main_go_line(path)
        key = (path, "main()" if ms and start >= ms else "helpers")
        blocks[key][0] += n
        if c == 0:
            blocks[key][1] += n
            if key[1] == "helpers":
                helpers_uncovered[path].append((start, end, n))
        else:
            blocks[key][1] += 0

    print(f"{'file':44s} {'段':8s} {'语句':>6s} {'未覆盖':>7s} {'覆盖':>7s}")
    for (path, seg) in sorted(blocks):
        tot, unc = blocks[(path, seg)]
        print(f"{path:44s} {seg:8s} {tot:6d} {unc:7d} {100*(tot-unc)/tot:6.1f}%")
    for path in sorted(helpers_uncovered):
        print(f"\n--- {path} helpers 未覆盖块 ---")
        for start, end, n in helpers_uncovered[path]:
            print(f"  L{start}-{end} ({n} 语句)")
    return 0


if __name__ == "__main__":
    sys.exit(main())