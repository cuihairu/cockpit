package rpc

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// ============ File Provider ============
//
// Agent 主机远程文件管理：浏览 / 分块读 / 写（覆盖与追加）/ 建目录 / 删除 /
// 改名。全文件系统可见（Agent 本有终端，等价能力，见 file-manager-design.md D1），
// 安全靠路径校验与 symlink 拒绝（D7）。全部同步 RPC——单块 ≤1MB 的分块模型
// 把每次调用压在 30s 超时内，大文件由 server 循环分块拉取（下载流转发）。

const (
	// fileReadChunkLimit file.read 单块字节上限（server 侧用 256KB 分块拉）
	fileReadChunkLimit = 1024 * 1024
	// fileWriteChunkLimit file.write 单块字节上限（base64 解码后）
	fileWriteChunkLimit = 1024 * 1024
)

// FileReadChunkLimit / FileWriteChunkLimit 供 capability metadata 与 server 校验复用
func FileReadChunkLimit() int  { return fileReadChunkLimit }
func FileWriteChunkLimit() int { return fileWriteChunkLimit }

// fileNameRe rename 目标：纯文件名，不含路径分隔符
var fileNameRe = regexp.MustCompile(`^[^/\x00]{1,255}$`)

// FileProvider 远程文件管理 Provider（无状态，全平台注册）
type FileProvider struct{}

func NewFileProvider() *FileProvider { return &FileProvider{} }

func (p *FileProvider) Type() string { return "file" }

func (p *FileProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "list":
		return p.List(paramString(params, "dir"))
	case "read":
		return p.Read(params)
	case "write":
		return p.Write(params)
	case "mkdir":
		return p.Mkdir(paramString(params, "path"))
	case "delete":
		return p.Delete(paramString(params, "path"))
	case "rename":
		return p.Rename(paramString(params, "path"), paramString(params, "name"))
	default:
		return nil, fmt.Errorf("unknown file action: %s", action)
	}
}

// cleanAbsPath 路径安全校验：Clean 后必须是绝对路径且不为根目录。
// 拒绝相对路径与 `..` 形态（Clean 已消解内部 `..`，残留者必然非绝对）。
func cleanAbsPath(path, what string) (string, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("%s must be an absolute path: %q", what, path)
	}
	if cleaned == string(filepath.Separator) {
		return "", fmt.Errorf("%s must not be the filesystem root", what)
	}
	return cleaned, nil
}

// lstatEntry 取条目信息，symlink 返回链接目标（不跟随）
func lstatEntry(path string) (os.FileInfo, bool, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false, "", err
	}
	isSymlink := info.Mode()&os.ModeSymlink != 0
	target := ""
	if isSymlink {
		if t, err := os.Readlink(path); err == nil {
			target = t
		}
	}
	return info, isSymlink, target, nil
}

// List 列目录直属条目（不递归），目录优先 + 名称排序
func (p *FileProvider) List(dir string) (interface{}, error) {
	cleaned, err := cleanAbsPath(dir, "dir")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(cleaned)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	out := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		full := filepath.Join(cleaned, e.Name())
		info, isSymlink, target, err := lstatEntry(full)
		if err != nil {
			continue // 竞态删除等瞬时错误跳过
		}
		isDir := info.IsDir()
		out = append(out, map[string]interface{}{
			"name":      e.Name(),
			"size":      info.Size(),
			"mode":      fmt.Sprintf("%04o", uint32(info.Mode().Perm())),
			"mtime":     info.ModTime().Unix(),
			"isDir":     isDir,
			"isSymlink": isSymlink,
			"target":    target,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i]["isDir"] != out[j]["isDir"] {
			return out[i]["isDir"].(bool) // 目录在前
		}
		return out[i]["name"].(string) < out[j]["name"].(string)
	})
	return map[string]interface{}{
		"dir":     cleaned,
		"entries": out,
	}, nil
}

// Read 分块读普通文件（symlink 拒绝，D7）。与 backup.read 同构。
func (p *FileProvider) Read(params map[string]interface{}) (interface{}, error) {
	path, err := cleanAbsPath(paramString(params, "path"), "path")
	if err != nil {
		return nil, err
	}
	offset := int64(paramFloat(params, "offset"))
	length := int64(paramFloat(params, "length"))
	if length <= 0 || length > fileReadChunkLimit {
		length = fileReadChunkLimit
	}
	info, isSymlink, _, err := lstatEntry(path)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if isSymlink {
		return nil, fmt.Errorf("refusing to read symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	total := info.Size()
	if offset >= total {
		return map[string]interface{}{"data": "", "size": total, "eof": true}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, fmt.Errorf("seek: %w", err)
	}
	if remaining := total - offset; length > remaining {
		length = remaining
	}
	buf := make([]byte, length)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, fmt.Errorf("read: %w", err)
	}
	return map[string]interface{}{
		"data": base64.StdEncoding.EncodeToString(buf[:n]),
		"size": total,
		"eof":  offset+int64(n) >= total,
	}, nil
}

// Write 写文件：truncate=true 覆盖/新建，false 追加（上传分块续传，文件须已存在）。
// symlink 拒绝（防经链接写穿）；返回写入后的文件总大小。
func (p *FileProvider) Write(params map[string]interface{}) (interface{}, error) {
	path, err := cleanAbsPath(paramString(params, "path"), "path")
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(paramString(params, "data"))
	if err != nil {
		return nil, fmt.Errorf("data must be base64")
	}
	if len(data) > fileWriteChunkLimit {
		return nil, fmt.Errorf("chunk too large: %d > %d bytes", len(data), fileWriteChunkLimit)
	}
	truncate, _ := params["truncate"].(bool)

	if info, isSymlink, _, err := lstatEntry(path); err == nil {
		if isSymlink {
			return nil, fmt.Errorf("refusing to write through symlink: %s", path)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("path is a directory: %s", path)
		}
	} else if os.IsNotExist(err) {
		// 追加模式要求文件已存在：分块上传首块建文件，后续块若发现
		// 目标丢失（被删/被移走），继续追加会产出损坏的文件碎片
		if !truncate {
			return nil, fmt.Errorf("append target does not exist: %s", path)
		}
	} else {
		return nil, fmt.Errorf("stat: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create parent dir: %w", err)
	}
	flags := os.O_CREATE | os.O_WRONLY
	if truncate {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	return map[string]interface{}{"size": info.Size()}, nil
}

// Mkdir 新建目录（支持多级）；已存在报错，防误把已有目录当新建
func (p *FileProvider) Mkdir(path string) (interface{}, error) {
	cleaned, err := cleanAbsPath(path, "path")
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(cleaned); err == nil {
		return nil, fmt.Errorf("path already exists: %s", cleaned)
	}
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	return map[string]interface{}{}, nil
}

// Delete 删除文件或递归删目录；symlink 只删链接本身（Remove 不跟随）
func (p *FileProvider) Delete(path string) (interface{}, error) {
	cleaned, err := cleanAbsPath(path, "path")
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(cleaned); err != nil {
		return nil, fmt.Errorf("remove: %w", err)
	}
	return map[string]interface{}{}, nil
}

// Rename 同目录内改名：name 只允许纯文件名，目标已存在拒绝（不静默覆盖）
func (p *FileProvider) Rename(path, name string) (interface{}, error) {
	cleaned, err := cleanAbsPath(path, "path")
	if err != nil {
		return nil, err
	}
	if !fileNameRe.MatchString(name) || name == "." || name == ".." {
		return nil, fmt.Errorf("invalid name: %q", name)
	}
	target := filepath.Join(filepath.Dir(cleaned), name)
	if _, err := os.Lstat(target); err == nil {
		return nil, fmt.Errorf("target already exists: %s", name)
	}
	if err := os.Rename(cleaned, target); err != nil {
		return nil, fmt.Errorf("rename: %w", err)
	}
	return map[string]interface{}{"path": target}, nil
}
