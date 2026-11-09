// 文件系统与归档校验层（自 main.go 按职责拆分）：源树/归档统一形态
// treeEntry、逐条目比对 diffEntries、Zip Slip 构造样例。
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------- 文件系统/归档层 ----------

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// treeEntry 归档条目与源树统一形态（kind: dir/reg/sym）
type treeEntry struct {
	Kind string
	Size int64
	SHA  string
	Link string
}

// buildTree 以 root 为根收树；prefix 拼在每个相对键前（源树传 basename 与
// tar 顶层目录对齐，恢复树传空串——解包后相对键已是 src/... 形态）
func buildTree(root, prefix string) (map[string]treeEntry, error) {
	out := make(map[string]treeEntry)
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			if prefix == "" {
				return nil // 恢复树根目录本身不入键
			}
			out[prefix] = entryFor(info, p)
			return nil
		}
		key := rel
		if prefix != "" {
			key = prefix + "/" + rel
		}
		out[key] = entryFor(info, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func entryFor(info os.FileInfo, p string) treeEntry {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		link, _ := os.Readlink(p)
		return treeEntry{Kind: "sym", Link: link}
	case info.IsDir():
		return treeEntry{Kind: "dir"}
	case info.Mode().IsRegular():
		sha, _ := sha256File(p)
		return treeEntry{Kind: "reg", Size: info.Size(), SHA: sha}
	default:
		return treeEntry{Kind: "other"}
	}
}

// extractArchive 解析 tar.gz 字节为统一形态（key 去尾斜杠）
func extractArchive(data []byte) (map[string]treeEntry, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := make(map[string]treeEntry)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		key := strings.TrimSuffix(hdr.Name, "/")
		switch hdr.Typeflag {
		case tar.TypeDir:
			out[key] = treeEntry{Kind: "dir"}
		case tar.TypeSymlink:
			out[key] = treeEntry{Kind: "sym", Link: hdr.Linkname}
		case tar.TypeReg, tar.TypeRegA:
			h := sha256.New()
			size, err := io.Copy(h, tr)
			if err != nil {
				return nil, fmt.Errorf("entry %s: %w", hdr.Name, err)
			}
			out[key] = treeEntry{Kind: "reg", Size: size, SHA: hex.EncodeToString(h.Sum(nil))}
		default:
			out[key] = treeEntry{Kind: "other"}
		}
	}
	return out, nil
}

// diffEntries want/got 逐键比对，返回问题清单（空=一致）
func diffEntries(want, got map[string]treeEntry) []string {
	var probs []string
	for _, k := range sortedKeys(want) {
		w := want[k]
		g, ok := got[k]
		if !ok {
			probs = append(probs, "missing: "+k)
			continue
		}
		switch {
		case w.Kind != g.Kind:
			probs = append(probs, fmt.Sprintf("kind %s: want %s got %s", k, w.Kind, g.Kind))
		case w.Kind == "reg" && (w.SHA != g.SHA || w.Size != g.Size):
			probs = append(probs, fmt.Sprintf("content %s: want sha=%s/%d got sha=%s/%d", k, w.SHA, w.Size, g.SHA, g.Size))
		case w.Kind == "sym" && w.Link != g.Link:
			probs = append(probs, fmt.Sprintf("link %s: want %q got %q", k, w.Link, g.Link))
		}
	}
	for _, k := range sortedKeys(got) {
		if _, ok := want[k]; !ok {
			probs = append(probs, "extra: "+k)
		}
	}
	return probs
}

func sortedKeys(m map[string]treeEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// evilEscapeEntries Zip Slip 构造样例的越界条目（相对 destDir 逃逸）
var evilEscapeEntries = []string{"../zipslip-escape-1", "sub/../../zipslip-escape-2"}

const evilOKEntry = "ok.txt"

// writeEvilTar 构造恶意归档：1 个正常条目 + 2 个越界条目
func writeEvilTar(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	write := func(name, content string) error {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := tw.Write([]byte(content))
		return err
	}
	for _, e := range evilEscapeEntries {
		if err := write(e, "pwned"); err != nil {
			f.Close()
			return err
		}
	}
	if err := write(evilOKEntry, "safe-content"); err != nil {
		f.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// escapeTarget 恶意条目若未被拦截会落盘的位置（与 unpack 的 Join+Clean 同式）
func escapeTarget(destDir, name string) string {
	return filepath.Clean(filepath.Join(destDir, name))
}
