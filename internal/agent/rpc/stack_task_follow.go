package rpc

import (
	"fmt"
	"sync"
	"time"
)

// ============ Stack 任务日志流式推送（P2 ②） ============
//
// 部署任务日志原先只有 task.get 轮询一条路（web 每 2s 拉全量缓冲）。
// 这里复用日志尾随同款通道（logs-design.md F4 架构）：agent 把 cappedBuffer
// 的增量经 proxy_data（proxyId="stacktask:<followId>"）推行，任务终态后
// proxy_close（reason=终态）收尾。server 端 NDJSON 流式响应，web 在任务
// 运行期打开 follow 替代轮询。

const (
	// stackTaskFollowProxyPrefix follow 数据的 proxyId 前缀
	stackTaskFollowProxyPrefix = "stacktask:"
	// stackTaskFollowMaxActive agent 侧同活 follow 上限（server 侧另有配额）
	stackTaskFollowMaxActive = 8
	// stackTaskFollowTick 增量轮询缓冲的间隔（部署输出是低频行流，300ms 够顺滑）
	stackTaskFollowTick = 300 * time.Millisecond
)

// taskFollowSession 一个进行中的任务日志跟随
type taskFollowSession struct {
	id       string
	taskID   string
	stop     chan struct{}
	stopOnce sync.Once
	once     sync.Once
	// closeFn 幂等收尾：注销会话 + proxy_close（reason）
	closeFn func(reason string)
}

// SetSender 注入数据回推函数（providers.go 接线；不注入则 follow 报错）
func (p *StackProvider) SetSender(fn func(proxyID string, data []byte)) {
	p.mu.Lock()
	p.taskSender = fn
	p.mu.Unlock()
}

// SetCloser 注入收尾函数（proxy_close）
func (p *StackProvider) SetCloser(fn func(proxyID, reason string)) {
	p.mu.Lock()
	p.taskCloser = fn
	p.mu.Unlock()
}

// FollowTask 处理 stack.task.follow：校验任务存在 → 起增量推送 goroutine。
// 任务已结束也允许 follow：先回填全量日志，紧接 proxy_close 终态帧。
func (p *StackProvider) FollowTask(params map[string]interface{}) (interface{}, error) {
	p.mu.Lock()
	sender, closer := p.taskSender, p.taskCloser
	p.mu.Unlock()
	if sender == nil || closer == nil {
		return nil, fmt.Errorf("stack task follow not available")
	}
	taskID := paramString(params, "taskId")
	followID := paramString(params, "followId")
	if taskID == "" || followID == "" || len(followID) > 128 {
		return nil, fmt.Errorf("taskId and followId required")
	}

	p.mu.Lock()
	task, ok := p.tasks[taskID]
	if !ok {
		p.mu.Unlock()
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if len(p.taskFollows) >= stackTaskFollowMaxActive {
		p.mu.Unlock()
		return nil, fmt.Errorf("too many active task follows (max %d)", stackTaskFollowMaxActive)
	}
	sess := &taskFollowSession{id: followID, taskID: taskID, stop: make(chan struct{})}
	sess.closeFn = func(reason string) {
		sess.once.Do(func() {
			p.mu.Lock()
			// 只删自己：同 followId 被新会话覆盖后，旧会话的迟到 close 不误删
			if cur := p.taskFollows[followID]; cur == sess {
				delete(p.taskFollows, followID)
			}
			p.mu.Unlock()
			closer(stackTaskFollowProxyPrefix+followID, reason)
		})
	}
	p.taskFollows[followID] = sess
	p.mu.Unlock()
	go p.pumpTaskLog(sess, task, sender)
	return map[string]interface{}{"followId": followID}, nil
}

// pumpTaskLog 增量推送任务日志：快照 diff 出新增字节推行，任务终态后收尾。
// cappedBuffer 只保留尾部 stackLogLimit 字节——截断发生后快照变短，回退
// offset 重发全量快照（尾部缓冲语义，接受重复段）。
func (p *StackProvider) pumpTaskLog(sess *taskFollowSession, task *stackTask, sender func(string, []byte)) {
	proxyID := stackTaskFollowProxyPrefix + sess.id
	offset := 0
	for {
		p.mu.Lock()
		status := task.Status
		snap := task.Log.String()
		p.mu.Unlock()
		if len(snap) < offset {
			offset = 0
		}
		if len(snap) > offset {
			sender(proxyID, []byte(snap[offset:]))
			offset = len(snap)
		}
		if status != stackTaskRunning {
			sess.closeFn(status)
			return
		}
		select {
		case <-sess.stop:
			sess.closeFn("stopped")
			return
		case <-time.After(stackTaskFollowTick):
		}
	}
}

// StopTaskFollow 处理 stack.task.follow.stop
func (p *StackProvider) StopTaskFollow(params map[string]interface{}) (interface{}, error) {
	followID := paramString(params, "followId")
	if followID == "" {
		return nil, fmt.Errorf("followId required")
	}
	p.mu.Lock()
	sess := p.taskFollows[followID]
	p.mu.Unlock()
	if sess == nil {
		return nil, fmt.Errorf("follow not found: %s", followID)
	}
	// 唤醒 pump 的 stop 分支让它退出（closeFn 只摘表 + 通知 server，
	// 不停 pump——不关 stop 它会空转到任务终态）
	sess.stopOnce.Do(func() { close(sess.stop) })
	sess.closeFn("stopped")
	return map[string]interface{}{"stopped": true}, nil
}
