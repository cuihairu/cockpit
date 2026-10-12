package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 部署任务日志流式推送（stack-deploy-design.md P2 ②，架构同 logs-design.md F4）：
//
//	POST /api/stacks/agents/{id}/tasks/{taskId}/follow   NDJSON 流式响应
//
// 数据面 agent 经 proxy_data（proxyId="stacktask:<followId>"）推行增量日志，
// 这里写入 HTTP 流；任务终态经 proxy_close 转为 eof 帧（reason=终态）。
// 客户端断开由 r.Context().Done() 感知，补发 stack.task.follow.stop。
// 浏览类不记审计（同日志尾随纪律 F8）。

const (
	// stackTaskFollowMaxGlobal / stackTaskFollowMaxPerAgent server 侧配额
	stackTaskFollowMaxGlobal   = 8
	stackTaskFollowMaxPerAgent = 4
	stackTaskFollowChanBuffer  = 256 // 帧缓冲：满则丢帧（不堵 agent WS 读循环）
	stackTaskFollowProxyPrefix = "stacktask:"
)

// stackTaskFollower 一个进行中的任务日志流（与 logsFollower 同款注册表模式）
type stackTaskFollower struct {
	agentID string
	ch      chan string // agent 推来的日志增量
	done    chan string // 终止 reason（终态或 stopped，buffered 1）
	once    sync.Once
}

var (
	stackTaskFollowersMu     sync.Mutex
	stackTaskFollowers       = map[string]*stackTaskFollower{}
	stackTaskFollowsPerAgent = map[string]int{}
)

// registerStackTaskFollower 上限检查 + 注册（必须在 CallAgent follow 之前，
// 否则 start 返回后 agent 的回填快照早于注册到达会丢帧）
func registerStackTaskFollower(followID, agentID string) (*stackTaskFollower, bool) {
	stackTaskFollowersMu.Lock()
	defer stackTaskFollowersMu.Unlock()
	if len(stackTaskFollowers) >= stackTaskFollowMaxGlobal ||
		stackTaskFollowsPerAgent[agentID] >= stackTaskFollowMaxPerAgent {
		return nil, false
	}
	f := &stackTaskFollower{
		agentID: agentID,
		ch:      make(chan string, stackTaskFollowChanBuffer),
		done:    make(chan string, 1),
	}
	stackTaskFollowers[followID] = f
	stackTaskFollowsPerAgent[agentID]++
	return f, true
}

// removeStackTaskFollower 注销并返回 follower（不存在返回 nil，幂等）
func removeStackTaskFollower(followID string) *stackTaskFollower {
	stackTaskFollowersMu.Lock()
	defer stackTaskFollowersMu.Unlock()
	f := stackTaskFollowers[followID]
	if f != nil {
		delete(stackTaskFollowers, followID)
		stackTaskFollowsPerAgent[f.agentID]--
		if stackTaskFollowsPerAgent[f.agentID] <= 0 {
			delete(stackTaskFollowsPerAgent, f.agentID)
		}
	}
	return f
}

// HandleStackTaskFollowData server.go proxy_data 分派入口（proxyId="stacktask:<id>"）
func (s *Server) HandleStackTaskFollowData(proxyID string, data []byte) {
	followID := strings.TrimPrefix(proxyID, stackTaskFollowProxyPrefix)
	stackTaskFollowersMu.Lock()
	f := stackTaskFollowers[followID]
	stackTaskFollowersMu.Unlock()
	if f == nil {
		return // 流已断开/不存在：静默丢弃，agent 侧上限会自行收尾
	}
	select {
	case f.ch <- string(data):
	default:
		// 背压丢弃：堵住会拖死 agent 消息读循环，256 帧缓冲内个人场景够用
	}
}

// HandleStackTaskFollowClose server.go proxy_close 分派入口：终止原因转 eof 帧
func (s *Server) HandleStackTaskFollowClose(proxyID, reason string) {
	followID := strings.TrimPrefix(proxyID, stackTaskFollowProxyPrefix)
	f := removeStackTaskFollower(followID)
	if f == nil {
		return
	}
	f.once.Do(func() {
		select {
		case f.done <- reason:
		default:
		}
	})
}

// handleStackTaskFollow POST /api/stacks/agents/{agentId}/tasks/{taskId}/follow
func (s *Server) handleStackTaskFollow(w http.ResponseWriter, r *http.Request, agentID, taskID string) {
	followID := protocol.GenerateIDWithPrefix("stackf")
	f, ok := registerStackTaskFollower(followID, agentID)
	if !ok {
		s.handleError(w, r, http.StatusTooManyRequests, "too many active task follows")
		return
	}

	// stop 即发即忘：收尾路径（客户端断开/写失败/不支持流式）不该等 agent
	// 应答——agent 无应答时 CallAgent 30s 超时会拖住 handler 返回
	stopAgent := func() {
		go func() {
			_, _ = s.CallAgent(agentID, "stack.task.follow.stop", map[string]interface{}{"followId": followID})
		}()
	}

	resp, err := s.CallAgent(agentID, "stack.task.follow", map[string]interface{}{
		"taskId":   taskID,
		"followId": followID,
	})
	if err == nil {
		var rpcResp protocol.RPCResponsePayload
		rpcResp, err = protocol.DecodeRPCResponse(resp)
		if err == nil && rpcResp.Status == "error" {
			msg := rpcResp.Error
			if msg == "" {
				msg = "agent rejected the follow"
			}
			err = &agentFollowError{msg}
		}
	}
	if err != nil {
		removeStackTaskFollower(followID)
		if _, isFollowErr := err.(*agentFollowError); isFollowErr {
			// agent 侧拒绝：任务不存在映射 404，其余透传
			if strings.Contains(err.Error(), "not found") {
				s.handleError(w, r, http.StatusNotFound, err.Error())
			} else {
				s.handleError(w, r, http.StatusBadGateway, err.Error())
			}
		} else {
			s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		}
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		removeStackTaskFollower(followID)
		stopAgent()
		s.handleError(w, r, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	defer func() {
		// 客户端断开或 eof 收尾：确保 agent 侧会话停掉并清表
		if removeStackTaskFollower(followID) != nil {
			stopAgent()
		}
	}()

	enc := json.NewEncoder(w)
	writeFrame := func(frame map[string]interface{}) bool {
		if err := enc.Encode(frame); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	for {
		select {
		case chunk := <-f.ch:
			if !writeFrame(map[string]interface{}{"data": chunk}) {
				return
			}
		case reason := <-f.done:
			// 先排干已缓冲的数据帧再收尾：回填快照与终态 close 可能同时
			// 就绪（select 随机选中 done），不排干会丢尾部日志
			for {
				select {
				case chunk := <-f.ch:
					if !writeFrame(map[string]interface{}{"data": chunk}) {
						return
					}
				default:
					_ = writeFrame(map[string]interface{}{"eof": true, "reason": reason})
					return
				}
			}
		case <-r.Context().Done():
			return // defer 补发 follow.stop
		}
	}
}
