#!/usr/bin/env python3
"""docs 站内链接校验。

校验范围（docs 为独立 VitePress 站，根目录 = 英文，/zh/ = 中文）：
  1. 所有页面里的 markdown 链接 ](href)（代码围栏内不算）
  2. frontmatter 里的 link: 值（home 布局的 actions/features、frontmatter sidebar）
  3. docs/.vitepress/config.ts 里 nav/sidebar 的 link 值

站内目标必须可解析：/x 指向 docs/x.md，/zh/x 指向 docs/zh/x.md；
相对路径按页面目录解析；非 .md 目标（图片等资产）要求文件存在。
外链（http/https、mailto:、tel:）与纯锚点不校验。

退出码：死链数为 0 时 0，否则 1。
用法：python3 tool/docs_check_links.py
"""
import os
import posixpath
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DOCS = os.path.join(REPO, "docs")
CONFIG = os.path.join(DOCS, ".vitepress", "config.ts")
IGNORE_DIRS = {"node_modules", ".vitepress", "review", "public"}

FENCE = re.compile(r"^ {0,3}(```|~~~)")
MD_LINK = re.compile(r"\]\(([^()\s]+)(?:\s+\"[^\"]*\")?\)")
FM_LINK = re.compile(r"^\s*link:\s*['\"]?([^'\"\s#]+)")
CFG_LINK = re.compile(r"link:\s*'([^']+)'")
EXTERNAL = ("http://", "https://", "//", "mailto:", "tel:", "javascript:")


def page_map():
    """站点 URL -> 磁盘文件（docs 相对）。"""
    pages = {}
    for root, dirs, names in os.walk(DOCS):
        dirs[:] = [d for d in dirs if d not in IGNORE_DIRS]
        for n in names:
            if n.endswith(".md"):
                rel = os.path.relpath(os.path.join(root, n), DOCS)
                pages[rel[:-3]] = rel
    return pages


PAGES = page_map()
dead = []


def add_dead(src, line_no, href, why):
    dead.append(f"{src}:{line_no}: {href} ({why})")


def check_target(href, src, line_no):
    if href.startswith(EXTERNAL) or href.startswith("#") or href == "":
        return
    body = href.split("#")[0].split("?")[0]
    if body == "":
        return
    if body.startswith("/"):
        stem = body[1:]
        if stem.endswith(".md"):
            stem = stem[:-3]
        if stem not in PAGES:
            add_dead(src, line_no, href, "no such page")
        return
    rel = posixpath.normpath(posixpath.join(posixpath.dirname(src), body))
    if rel.startswith(".."):
        add_dead(src, line_no, href, "escapes docs/")
        return
    if rel.endswith(".md") or rel in PAGES or os.path.exists(os.path.join(DOCS, rel + ".md")):
        # 文档目标：解析为 .md 页
        stem = rel[:-3] if rel.endswith(".md") else rel
        if stem not in PAGES:
            add_dead(src, line_no, href, "no such page")
    else:
        # 资产目标：要求文件存在
        if not os.path.exists(os.path.join(DOCS, rel)):
            add_dead(src, line_no, href, "no such file")


def fm_range(lines):
    """frontmatter 行号区间（1 起，首行为 --- 才算）；无则 (0, 0)。"""
    if not lines or lines[0].rstrip("\n") != "---":
        return 0, 0
    for i in range(1, len(lines)):
        if lines[i].rstrip("\n") == "---":
            return 1, i + 1  # 开区间 (0..i)，两端 --- 之间
    return 1, len(lines)


def check_pages():
    for stem in sorted(PAGES):
        src = PAGES[stem]  # docs 相对路径
        lines = open(os.path.join(DOCS, src), encoding="utf-8").read().split("\n")
        f_start, f_end = fm_range(lines)
        fence = False
        for i, ln in enumerate(lines, 1):
            if FENCE.match(ln):
                fence = not fence
                continue
            if fence:
                continue
            if f_start < i <= f_end:
                m = FM_LINK.match(ln)
                if m:
                    check_target(m.group(1), src, i)
                continue
            for href in MD_LINK.findall(ln):
                check_target(href, src, i)


def check_config():
    if not os.path.exists(CONFIG):
        return
    for i, ln in enumerate(open(CONFIG, encoding="utf-8"), 1):
        for href in CFG_LINK.findall(ln):
            check_target(href, ".vitepress/config.ts", i)


check_pages()
check_config()
for d in dead:
    print(d)
print(f"dead_links={len(dead)}")
sys.exit(1 if dead else 0)
