package alert

// cov_io_gaps_test.go 覆盖 createAlertIfNotExists 的两个库错误分支：
// 关库后查重失败（记日志继续）与创建失败（记日志返回）。

import (
	"testing"
)

func TestCovCreateAlertDBErrors(t *testing.T) {
	db := testDB(t)
	g := NewGenerator(db, nil, nil)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// 关库：HasUnreadAlert 报错 → 仅记日志；CreateAlert 报错 → 记日志返回
	g.createAlertIfNotExists("info", "cov-title", "msg", "res-1", "host")
}
