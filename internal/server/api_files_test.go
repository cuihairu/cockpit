package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/audit"
)

// filesReq 构造 POST 到 /api/agents/{id}/files/{action} 的请求
func filesReq(action, agentID, body string) *http.Request {
	return httptest.NewRequest(http.MethodPost,
		"/api/agents/"+agentID+"/files/"+action, strings.NewReader(body))
}

func TestFilesListForwards(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	var gotParams map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		gotParams = params
		return map[string]interface{}{
			"dir": "/etc/nginx",
			"entries": []map[string]interface{}{
				{"name": "nginx.conf", "size": 513, "mode": "0644", "mtime": 1726000000,
					"isDir": false, "isSymlink": false, "target": ""},
			},
		}, ""
	})

	rec := httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("list", "a1", `{"dir":"/etc/nginx"}`), "a1/files/list")
	if rec.Code != http.StatusOK {
		t.Fatalf("list code = %d, body: %s", rec.Code, rec.Body.String())
	}
	if gotMethod != "file.list" || gotParams["dir"] != "/etc/nginx" {
		t.Fatalf("forwarded = %s %v", gotMethod, gotParams)
	}
	var out struct {
		Entries []map[string]interface{} `json:"entries"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Entries) != 1 || out.Entries[0]["name"] != "nginx.conf" {
		t.Fatalf("entries = %v", out.Entries)
	}
}

func TestFilesWriteValidatesAndForwards(t *testing.T) {
	s := newBackupTestServer(t)
	var gotParams map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotParams = params
		return map[string]interface{}{"size": 5}, ""
	})

	// 非法路径 → 400 且不下发
	for _, path := range []string{"rel/path", "/", "", "../x"} {
		rec := httptest.NewRecorder()
		body, _ := json.Marshal(map[string]interface{}{"path": path, "data": "aGk=", "truncate": true})
		s.handleAgentFilesAPI(rec, filesReq("write", "a1", string(body)), "a1/files/write")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q: code = %d, want 400", path, rec.Code)
		}
	}
	if gotParams != nil {
		t.Fatal("should not dispatch on invalid path")
	}

	// 合法 → 转发
	rec := httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("write", "a1",
		`{"path":"/tmp/x/a.txt","data":"aGk=","truncate":true}`), "a1/files/write")
	if rec.Code != http.StatusOK {
		t.Fatalf("write code = %d, body: %s", rec.Code, rec.Body.String())
	}
	if gotParams["path"] != "/tmp/x/a.txt" || gotParams["truncate"] != true {
		t.Fatalf("params = %v", gotParams)
	}
	// server 转发原始 base64（agent 侧解码校验大小）
	if gotParams["data"] != "aGk=" {
		t.Fatalf("data = %v", gotParams["data"])
	}
}

func TestFilesRenameNameValidation(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", nil)

	for _, name := range []string{"a/b", "..", ".", "", "a\\b"} {
		rec := httptest.NewRecorder()
		body, _ := json.Marshal(map[string]string{"path": "/tmp/x/a.txt", "name": name})
		s.handleAgentFilesAPI(rec, filesReq("rename", "a1", string(body)), "a1/files/rename")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name %q: code = %d, want 400", name, rec.Code)
		}
	}
}

func TestFilesAgentOffline(t *testing.T) {
	s := newBackupTestServer(t) // 不注册任何 agent

	rec := httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("list", "ghost", `{"dir":"/tmp"}`), "ghost/files/list")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("offline code = %d, want 503", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.handleAgentFilesAPI(rec,
		httptest.NewRequest(http.MethodGet, "/api/agents/ghost/files/download?path=/tmp/x", nil),
		"ghost/files/download")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("download offline code = %d, want 503", rec.Code)
	}
}

func TestFilesDownload(t *testing.T) {
	s := newBackupTestServer(t)
	// 1.5 块：覆盖满块与尾块
	content := make([]byte, filesDownloadChunk+100)
	for i := range content {
		content[i] = byte(i % 251)
	}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		if method != "file.read" {
			return nil, "unexpected method " + method
		}
		if params["path"] != "/var/log/big.log" {
			return nil, "bad path"
		}
		offset := int64(params["offset"].(float64))
		length := int64(params["length"].(float64))
		if offset >= int64(len(content)) {
			return map[string]interface{}{"data": "", "size": len(content), "eof": true}, ""
		}
		end := offset + length
		if end > int64(len(content)) {
			end = int64(len(content))
		}
		return map[string]interface{}{
			"data": base64.StdEncoding.EncodeToString(content[offset:end]),
			"size": len(content),
			"eof":  end >= int64(len(content)),
		}, ""
	})

	rec := httptest.NewRecorder()
	u := "/api/agents/a1/files/download?path=" + url.QueryEscape("/var/log/big.log")
	s.handleAgentFilesAPI(rec, httptest.NewRequest(http.MethodGet, u, nil), "a1/files/download")
	if rec.Code != http.StatusOK {
		t.Fatalf("download code = %d, body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, content) {
		t.Fatalf("downloaded %d bytes, want %d (mismatch)", len(got), len(content))
	}
	if cl := rec.Header().Get("Content-Length"); cl != strconv.Itoa(len(content)) {
		t.Errorf("Content-Length = %q, want %d", cl, len(content))
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "big.log") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	// 非法路径拒绝
	rec = httptest.NewRecorder()
	s.handleAgentFilesAPI(rec,
		httptest.NewRequest(http.MethodGet, "/api/agents/a1/files/download?path=rel", nil),
		"a1/files/download")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad path code = %d, want 400", rec.Code)
	}
}

// TestFilesSearchValidatesAndForwards 文本搜索端点（file-manager-design.md D12）：
// 校验（相对 dir / 空 query / 超长 query 400）+ 转发参数透传 + 浏览不审计
func TestFilesSearchValidatesAndForwards(t *testing.T) {
	s := newBackupTestServer(t)
	var gotMethod string
	var gotParams map[string]interface{}
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		gotMethod = method
		gotParams = params
		return map[string]interface{}{
			"matches": []map[string]interface{}{
				{"path": "nginx.conf", "line": 1, "text": "server_name example.com;"},
			},
			"truncated": false, "scanned": 12, "skipped": 2,
		}, ""
	})

	// 相对 dir → 400
	rec := httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("search", "a1", `{"dir":"rel/path","query":"x"}`), "a1/files/search")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("relative dir: code = %d", rec.Code)
	}

	// 空 query → 400
	rec = httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("search", "a1", `{"dir":"/etc","query":""}`), "a1/files/search")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty query: code = %d", rec.Code)
	}

	// 超长 query → 400
	longQuery := strings.Repeat("q", 257)
	rec = httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("search", "a1", `{"dir":"/etc","query":"`+longQuery+`"}`), "a1/files/search")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("long query: code = %d", rec.Code)
	}

	// 正常转发：caseSensitive 透传
	rec = httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("search", "a1", `{"dir":"/etc/nginx","query":"server_name","caseSensitive":true}`), "a1/files/search")
	if rec.Code != http.StatusOK {
		t.Fatalf("search code = %d, body: %s", rec.Code, rec.Body.String())
	}
	if gotMethod != "file.search" {
		t.Fatalf("method = %s", gotMethod)
	}
	if gotParams["dir"] != "/etc/nginx" || gotParams["query"] != "server_name" || gotParams["caseSensitive"] != true {
		t.Fatalf("params = %v", gotParams)
	}
	var out struct {
		Matches []map[string]interface{} `json:"matches"`
		Scanned int                      `json:"scanned"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Matches) != 1 || out.Matches[0]["path"] != "nginx.conf" || out.Scanned != 12 {
		t.Fatalf("out = %v", out)
	}
	// 浏览性质不审计（D9 延伸）：无 file_search 审计记录
	logs, _, _ := s.db.GetAuditLogs(0, 100, nil)
	for _, l := range logs {
		if strings.Contains(l.Action, "search") {
			t.Fatalf("search must not be audited, got %s", l.Action)
		}
	}
}

func TestFilesWriteAuditTruncateSplit(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"size": 1024}, ""
	})

	// truncate=true（编辑保存/分块首块）→ 记审计
	rec := httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("write", "a1",
		`{"path":"/tmp/x/big.bin","data":"aGk=","truncate":true}`), "a1/files/write")
	if rec.Code != http.StatusOK {
		t.Fatalf("first chunk code = %d", rec.Code)
	}
	_, total, err := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionFileWrite})
	if err != nil || total != 1 {
		t.Fatalf("truncate write should audit once: total=%d err=%v", total, err)
	}

	// truncate=false（分块续块）→ 不记审计（file-manager-design M3/D18）
	rec = httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("write", "a1",
		`{"path":"/tmp/x/big.bin","data":"aGk=","truncate":false}`), "a1/files/write")
	if rec.Code != http.StatusOK {
		t.Fatalf("append chunk code = %d", rec.Code)
	}
	_, total, err = s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionFileWrite})
	if err != nil || total != 1 {
		t.Fatalf("append chunk should not audit: total=%d err=%v", total, err)
	}

	// truncate 缺省（既有调用方）→ 视为 true 照记
	rec = httptest.NewRecorder()
	s.handleAgentFilesAPI(rec, filesReq("write", "a1",
		`{"path":"/tmp/x/a.txt","data":"aGk="}`), "a1/files/write")
	if rec.Code != http.StatusOK {
		t.Fatalf("default write code = %d", rec.Code)
	}
	_, total, err = s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionFileWrite})
	if err != nil || total != 2 {
		t.Fatalf("default truncate should audit: total=%d err=%v", total, err)
	}
}
