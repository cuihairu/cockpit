package auth

// 惰性初始化（ensureDefaults/sync.Once）行为测试——子进程模式。
//
// 为什么用子进程：「import 期零副作用」与「sync.Once 首次触发」都是
// 进程生命周期的性质；测试二进制启动时本包 import 早已完成、多数用例
// 也已消耗 once，进程内无法复现这两个时机。子进程复用同一二进制
// （go test -race 下编译为 race-instrumented），helper 模式里的竞态会
// 打到自己的 stderr，父进程据输出判成败——-race 全量跑即自动覆盖本文件。
//
// 治理对象（立项见 todo「JWT_SECRET 警告噪音」）：旧 init() 让任何
// import 本包的二进制（含不发 token 的 `cockpit sync` CLI）启动即打
// `WARNING: JWT_SECRET not set`。现要求：不用 auth 就没有那行；
// 用到时才打，且密钥语义/校验强度与改前逐条一致（不放宽）。

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

const lazyHelperEnv = "COCKPIT_TEST_LAZY_HELPER"

const noSecretWarning = "WARNING: JWT_SECRET not set"

// runLazyHelper 在「未设 JWT_SECRET」的子进程里执行 mode 对应的动作。
func runLazyHelper(t *testing.T, mode string) (string, error) {
	t.Helper()
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "JWT_SECRET=") {
			continue
		}
		env = append(env, kv)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLazyInitHelper")
	cmd.Env = append(env, lazyHelperEnv+"="+mode)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestLazyInitHelper 子进程入口。父测试只匹配本用例名，非 helper
// 模式下它平凡通过。
func TestLazyInitHelper(t *testing.T) {
	switch os.Getenv(lazyHelperEnv) {
	case "":
		return
	case "importonly":
		// 只 import 不触碰 token 功能——对应 sync CLI 启动路径。
		// 走到这里没有任何断言：正确性由父进程「输出无 WARNING」判定。
		// HELPER_OK 兼作哨兵：证明 helper 模式真的执行过（-run 没匹配
		// 上时子进程 0 用例也退出 0、输出干净，会造成「无 WARNING」假绿）。
		fmt.Println("HELPER_OK importonly")
		return
	case "firstuse":
		tok, err := GenerateToken("u1", "alice", "admin")
		if err != nil {
			fmt.Println("HELPER_ERR firstuse generate:", err)
			return
		}
		claims, err := ValidateToken(tok)
		if err != nil || claims.Username != "alice" {
			fmt.Println("HELPER_ERR firstuse validate")
			return
		}
		// 篡改 token 必须被拒（校验强度不因惰性化放宽）。
		// 篡改必须落在 payload 段：HS256 段尾字符在 base64url 里可能
		// 因尾部填充位（末字符低位不参与字节还原）解出同一签名——
		// 改末字符「验签合法通过」不是校验被绕，而是内容根本没变，
		// 首版就因此在负载下间歇假阴过一次。换 payload 一个字符后
		// 签名与内容必然不匹配。
		first := strings.IndexByte(tok, '.')
		second := strings.Index(tok[first+1:], ".")
		if first < 0 || second < 0 {
			fmt.Println("HELPER_ERR firstuse malformed token")
			return
		}
		p := first + 1 + second/2
		b := []byte(tok)
		if b[p] == 'x' {
			b[p] = 'y'
		} else {
			b[p] = 'x'
		}
		if _, err := ValidateToken(string(b)); err == nil {
			fmt.Println("HELPER_ERR firstuse tampered accepted")
			return
		}
		fmt.Println("HELPER_OK firstuse")
	case "concurrent":
		const n = 30
		var wg sync.WaitGroup
		var mu sync.Mutex
		tokens := make([]string, 0, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tok, err := GenerateToken("u1", "alice", "admin")
				if err != nil {
					fmt.Println("HELPER_ERR concurrent generate:", err)
					return
				}
				mu.Lock()
				tokens = append(tokens, tok)
				mu.Unlock()
			}()
		}
		wg.Wait()
		// once 语义：所有 token 由同一密钥签发 → 任一 token 可被任意
		// 后续调用校验；出现多个密钥（首触发竞态）时本循环必报错。
		for _, tok := range tokens {
			if _, err := ValidateToken(tok); err != nil {
				fmt.Println("HELPER_ERR concurrent validate:", err)
				return
			}
		}
		fmt.Printf("HELPER_OK concurrent tokens=%d\n", len(tokens))
	case "service":
		// auth 包直接收空 Options.Secret 时的构造期 eager 语义（警告 +
		// 随机密钥，改前后一致）+ 签发/校验往返可用。注意真实 server
		// 走不到这条：config.Normalize 把空 jwt.secret 预填 "change-me"
		// （internal/config/config.go），所以改前 server 启动日志里那行
		// WARNING 也是 init() 噪音而非实例行为——惰性化后随噪音一起消失，
		// 实例校验路径前后零差异（PRODUCTION 强制校验只查 TOTP/storage
		// key 不改 JWT 缺省值，同样是改前既有边界）。
		svc := NewService(nil, Options{})
		tok, err := svc.GenerateToken("u1", "alice", "admin")
		if err != nil {
			fmt.Println("HELPER_ERR service generate:", err)
			return
		}
		if _, err := svc.ValidateToken(tok); err != nil {
			fmt.Println("HELPER_ERR service validate:", err)
			return
		}
		fmt.Println("HELPER_OK service")
	}
}

func TestLazyInitImportHasNoSideEffects(t *testing.T) {
	out, err := runLazyHelper(t, "importonly")
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "HELPER_OK importonly") {
		t.Fatalf("helper mode did not run (false-green guard):\n%s", out)
	}
	if strings.Contains(out, noSecretWarning) {
		t.Errorf("import-only process must not warn (sync CLI noise regression):\n%s", out)
	}
}

func TestLazyInitFirstUseWarnsOnce(t *testing.T) {
	out, err := runLazyHelper(t, "firstuse")
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "HELPER_ERR") {
		t.Fatalf("helper reported errors:\n%s", out)
	}
	if !strings.Contains(out, "HELPER_OK firstuse") {
		t.Fatalf("helper mode did not run (false-green guard):\n%s", out)
	}
	if n := strings.Count(out, noSecretWarning); n != 1 {
		t.Errorf("warning count = %d, want exactly 1 (lazy first-use timing, same as old init once): %s", n, out)
	}
}

func TestLazyInitConcurrentFirstUse(t *testing.T) {
	out, err := runLazyHelper(t, "concurrent")
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "HELPER_ERR") {
		t.Fatalf("helper reported errors:\n%s", out)
	}
	if !strings.Contains(out, "HELPER_OK concurrent tokens=30") {
		t.Fatalf("helper did not complete round trip:\n%s", out)
	}
	// -race 下本用例的子进程同样是 race-instrumented 二进制，
	// DATA RACE 会出现在 out 里（CombinedOutput 捕获）。
	if strings.Contains(out, "DATA RACE") {
		t.Errorf("data race in concurrent first use:\n%s", out)
	}
}

func TestLazyInitServicePathUnchanged(t *testing.T) {
	out, err := runLazyHelper(t, "service")
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "HELPER_ERR") {
		t.Fatalf("helper reported errors:\n%s", out)
	}
	if !strings.Contains(out, "HELPER_OK service") {
		t.Fatalf("helper mode did not run (false-green guard):\n%s", out)
	}
	// 服务端语义对照（改前即如此）：NewService 构造时 eager 解析并警告
	if !strings.Contains(out, noSecretWarning) {
		t.Errorf("NewService with empty secret must warn at construction (server startup semantics unchanged):\n%s", out)
	}
}
