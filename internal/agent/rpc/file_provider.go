package rpc

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
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
	case "chmod":
		return p.Chmod(params)
	case "chown":
		return p.Chown(params)
	case "search":
		return p.Search(params)
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
		uid, gid := fileStatOwner(full)
		out = append(out, map[string]interface{}{
			"name":      e.Name(),
			"size":      info.Size(),
			"mode":      fmt.Sprintf("%04o", uint32(info.Mode().Perm())),
			"mtime":     info.ModTime().Unix(),
			"uid":       uid,
			"gid":       gid,
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
	// mode 可选：0/缺省 = 0644（既有行为）；白名单只放私钥 0600 与常规 0644，
	// 防误设 0777 之类（ACME 部署用它写私钥，见 acme-design.md D14）
	perm := os.FileMode(0o644)
	explicit := false
	if raw, ok := params["mode"]; ok && raw != nil && raw != float64(0) {
		mode, ok := raw.(float64)
		if !ok {
			return nil, fmt.Errorf("mode must be a number")
		}
		switch os.FileMode(mode) {
		case 0o600, 0o644:
			perm = os.FileMode(mode)
			explicit = mode == 0o600
		default:
			return nil, fmt.Errorf("mode must be 0600 or 0644")
		}
	}
	flags := os.O_CREATE | os.O_WRONLY
	if truncate {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, perm)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	// 已存在文件的权限不受 OpenFile perm 影响：显式 0600（私钥部署覆盖写）
	// 需要补 chmod 落准；缺省路径不 chmod，保持既有行为（设备文件等特殊目标）
	if explicit {
		if err := f.Chmod(perm); err != nil {
			f.Close()
			return nil, fmt.Errorf("chmod: %w", err)
		}
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	info, err := fileStat(f)
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

// ============ 权限编辑（见 file-manager-design.md M4，D20-D21） ============

// fileMaxID uid/gid 上限（uint32 语义）
const fileMaxID = 1<<32 - 1

// chmodChownTarget 校验路径并拒绝 symlink（D7 延伸：对链接 chmod/chown
// 会作用到目标，给「不跟随」纪律开例外）
func chmodChownTarget(path string) (string, os.FileInfo, error) {
	cleaned, err := cleanAbsPath(path, "path")
	if err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(cleaned)
	if err != nil {
		return "", nil, fmt.Errorf("path not found: %s", cleaned)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("refusing to operate on a symlink: %s", cleaned)
	}
	return cleaned, info, nil
}

// numParam 取数字参数（JSON number 必为 float64），要求非负整数
func numParam(params map[string]interface{}, key string, max float64) (float64, error) {
	raw, ok := params[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	v, ok := raw.(float64)
	if !ok || v != float64(int64(v)) || v < 0 || v > max {
		return 0, fmt.Errorf("%s must be an integer in [0, %d]", key, int64(max))
	}
	return v, nil
}

// Chmod 修改权限位（0-0o777，不碰 setuid/sticky）
func (p *FileProvider) Chmod(params map[string]interface{}) (interface{}, error) {
	cleaned, _, err := chmodChownTarget(paramString(params, "path"))
	if err != nil {
		return nil, err
	}
	mode, err := numParam(params, "mode", 0o777)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(cleaned, os.FileMode(mode)); err != nil {
		return nil, fmt.Errorf("chmod: %w", err)
	}
	return map[string]interface{}{"mode": fmt.Sprintf("%04o", int(mode))}, nil
}

// Chown 修改属主 uid/gid（不做用户名解析——跨机用户名空间不可靠）
func (p *FileProvider) Chown(params map[string]interface{}) (interface{}, error) {
	cleaned, _, err := chmodChownTarget(paramString(params, "path"))
	if err != nil {
		return nil, err
	}
	uid, err := numParam(params, "uid", fileMaxID)
	if err != nil {
		return nil, err
	}
	gid, err := numParam(params, "gid", fileMaxID)
	if err != nil {
		return nil, err
	}
	if err := os.Chown(cleaned, int(uid), int(gid)); err != nil {
		return nil, fmt.Errorf("chown: %w", err)
	}
	return map[string]interface{}{"uid": int(uid), "gid": int(gid)}, nil
}

// ============ 文本搜索（见 file-manager-design.md M2，D10-D11） ============

const (
	// fileSearchMaxDepth 递归深度上限
	fileSearchMaxDepth = 8
	// fileSearchMaxFiles 扫描文件总数上限（防大目录树）
	fileSearchMaxFiles = 5000
	// fileSearchMaxMatches 命中条数上限（达到即停）
	fileSearchMaxMatches = 200
	// fileSearchMaxFileSize 单文件参与搜索的大小上限（与编辑器 read 上限一致）
	fileSearchMaxFileSize = 1024 * 1024
	// fileSearchMaxQuery 关键词长度上限
	fileSearchMaxQuery = 256
	// fileSearchLineMaxChars 命中行文本截断长度
	fileSearchLineMaxChars = 200
)

// fileSearchTimeout 单次搜索总超时（var：测试注入短超时以覆盖 deadline 分支）
var fileSearchTimeout = 15 * time.Second

// Search 目录内递归文本搜索：纯文本 contains（D10，防 ReDoS 与 logs grep 同纪律）、
// 默认大小写不敏感；跳过 symlink/二进制/大文件（D11），各上限置 truncated。
// 命中即停返回已得结果——搜索是浏览性质操作，部分结果同样可用。
func (p *FileProvider) Search(params map[string]interface{}) (interface{}, error) {
	dir, err := cleanAbsPath(paramString(params, "dir"), "dir")
	if err != nil {
		return nil, err
	}
	query := paramString(params, "query")
	if query == "" {
		return nil, fmt.Errorf("query required")
	}
	if len(query) > fileSearchMaxQuery {
		return nil, fmt.Errorf("query too long (max %d bytes)", fileSearchMaxQuery)
	}
	caseSensitive, _ := params["caseSensitive"].(bool)
	maxResults := fileSearchMaxMatches
	if n := int(paramFloat(params, "maxResults")); n > 0 && n < fileSearchMaxMatches {
		maxResults = n
	}
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(query)
	}

	type searchState struct {
		matches   []map[string]interface{}
		scanned   int
		skipped   int
		truncated bool
	}
	st := &searchState{matches: []map[string]interface{}{}}
	rootDepth := strings.Count(dir, string(filepath.Separator))
	deadline := time.Now().Add(fileSearchTimeout)

	// walkFn 只返回 nil/SkipAll/SkipDir，WalkDir 结果无需再检查
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 无权限目录等瞬时错误跳过，不中断整体搜索
		}
		if time.Now().After(deadline) {
			st.truncated = true
			return filepath.SkipAll
		}
		// WalkDir 的 path 恒在 dir 之下，Rel 不会失败
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			base := d.Name()
			if rel != "." && (base == ".git" || base == "node_modules") {
				return filepath.SkipDir
			}
			// 深度按根目录的层级数起算，超限整枝跳过
			if rel != "." && strings.Count(path, string(filepath.Separator))-rootDepth > fileSearchMaxDepth {
				st.truncated = true
				return filepath.SkipDir
			}
			return nil
		}
		// 只搜普通文件：symlink（WalkDir Lstat 语义）与设备等一律跳过（D7 延伸）
		if !d.Type().IsRegular() {
			return nil
		}
		st.scanned++
		if st.scanned > fileSearchMaxFiles {
			st.truncated = true
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil || info.Size() > fileSearchMaxFileSize {
			st.skipped++
			return nil
		}
		matches, scannedLine := searchInFile(path, needle, caseSensitive, maxResults-len(st.matches))
		for _, m := range matches {
			m["path"] = rel // 相对 dir 呈现
		}
		st.matches = append(st.matches, matches...)
		if scannedLine {
			st.skipped++
		}
		if len(st.matches) >= maxResults {
			st.truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	return map[string]interface{}{
		"matches":   st.matches,
		"truncated": st.truncated,
		"scanned":   st.scanned,
		"skipped":   st.skipped,
	}, nil
}

// searchInFile 单文件行扫描，返回命中（相对信息由调用方补）与是否二进制跳过
func searchInFile(path, needle string, caseSensitive bool, budget int) (matches []map[string]interface{}, binarySkipped bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	// 二进制探测：首 512 字节含 NUL 即跳过（git 同款启发式）
	head := make([]byte, 512)
	n, _ := f.Read(head)
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return nil, true
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, false
	}

	matches = nil
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), fileSearchMaxFileSize)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		hay := line
		if !caseSensitive {
			hay = strings.ToLower(line)
		}
		if !strings.Contains(hay, needle) {
			continue
		}
		text := line
		if len(text) > fileSearchLineMaxChars {
			text = text[:fileSearchLineMaxChars]
		}
		matches = append(matches, map[string]interface{}{
			"path": path,
			"line": lineNo,
			"text": text,
		})
		if len(matches) >= budget {
			break
		}
	}
	return matches, false
}
