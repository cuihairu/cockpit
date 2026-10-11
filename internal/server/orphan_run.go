package server

import (
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// server 重启孤儿清扫（审批门前置）：Job 派发（dispatchJobAsync）与 workflow
// 推进（advanceWorkflow）都是进程内 goroutine，重启即丢——遗留的 pending/
// running Job 行与 running WorkflowRun 行不再有推进者，继续等待只会永远悬挂。
// 启动期（HTTP 未开始接流）一次性收敛为 failed 终态，与 backup 的
// recoverOrphanBackupRuns 同款口径：逐行条件 UPDATE，绝不改写已落终态的行。

// orphanErrMsg 孤儿记录落账错误摘要（进 Job.error；run 无错误列只落状态）。
const orphanErrMsg = "server restarted during execution"

// recoverOrphanRuns 把重启遗留的非终态 Job（pending/running）与 running
// WorkflowRun 收敛为 failed。Job 面与 run 面相互独立：一面查询失败只记日志
// 继续，另一面照常清扫（一面不可用不该拖住另一面的孤儿收敛）。
func (s *Server) recoverOrphanRuns() {
	swept := 0
	// 先 running 后 pending：pending 行含「建行后派发 goroutine 未及翻转」
	// 的窗口态，重启后一律无推进者
	for _, status := range []string{storage.JobStatusRunning, storage.JobStatusPending} {
		jobs, err := s.db.ListJobsFiltered(status, "", "", "", 10000)
		if err != nil {
			log.Printf("recover orphan jobs (%s): %v", status, err)
			continue
		}
		for _, job := range jobs {
			if _, err := s.db.FailJob(job.ID, status, time.Now(), orphanErrMsg); err != nil {
				log.Printf("recover job %s: %v", job.ID, err)
				continue
			}
			swept++
		}
	}

	runs, err := s.db.RunningWorkflowRuns()
	if err != nil {
		log.Printf("recover orphan workflow runs: %v", err)
		return
	}
	for _, run := range runs {
		if _, err := s.db.FinishWorkflowRun(run.ID, storage.WorkflowRunStatusFailed, time.Now()); err != nil {
			log.Printf("recover workflow run %s: %v", run.ID, err)
			continue
		}
		swept++
	}

	if swept > 0 {
		log.Printf("Recovered %d orphan job/run records", swept)
	}
}
