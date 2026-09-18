package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 日志实时尾随（见 logs-design.md F4/F5）：
//
//	POST /api/agents/{id}/logs/follow   body 同 query 参数，NDJSON 流式响应
//
// 数据面 agent 经 proxy_data（proxyId="logs:<followId>"）推行，这里写入 HTTP 流；
// 终止（agent 进程退出 / 上限 / stop）经 proxy_close 转为 eof 帧。客户端断开
// （fetch abort）由 r.Context().Done() 感知，补发 logs.follow.stop。
// 浏览类不记审计（F8，同查询纪律 D9）。

const (
	// logsFollowMaxGlobal / logsFollowMaxPerAgent server 侧 follow 上限（F5）
	logsFollowMaxGlobal   = 8
	logsFollowMaxPerAgent = 2
	logsFollowChanBuffer  = 256 // 行缓冲：吸收突发输出，满则丢帧（不堵 agent WS 读循环）
	logsFollowProxyPrefix = "logs:"
)

// logsFollower 一个进行中的尾随流（与 terminalSessions 同款注册表模式）
type logsFollower struct {
	agentID string
	ch      chan string // agent 推来的行（含换行）
	done    chan string // 终止 reason（buffered 1，close 路径不阻塞）
	once    sync.Once
}

var (
	logsFollowersMu     sync.Mutex
	logsFollowers       = map[string]*logsFollower{}
	logsFollowsPerAgent = map[string]int{}
)

// registerLogsFollower 上限检查 + 注册（必须在 CallAgent follow.start 之前，
// 否则 start 返回后 agent 的回填行早于注册到达会丢帧）
func registerLogsFollower(followID, agentID string) (*logsFollower, bool) {
	logsFollowersMu.Lock()
	defer logsFollowersMu.Unlock()
	if len(logsFollowers) >= logsFollowMaxGlobal || logsFollowsPerAgent[agentID] >= logsFollowMaxPerAgent {
		return nil, false
	}
	f := &logsFollower{
		agentID: agentID,
		ch:      make(chan string, logsFollowChanBuffer),
		done:    make(chan string, 1),
	}
	logsFollowers[followID] = f
	logsFollowsPerAgent[agentID]++
	return f, true
}

// removeLogsFollower 注销并返回 follower（不存在返回 nil，幂等）
func removeLogsFollower(followID string) *logsFollower {
	logsFollowersMu.Lock()
	defer logsFollowersMu.Unlock()
	f := logsFollowers[followID]
	if f != nil {
		delete(logsFollowers, followID)
		logsFollowsPerAgent[f.agentID]--
		if logsFollowsPerAgent[f.agentID] <= 0 {
			delete(logsFollowsPerAgent, f.agentID)
		}
	}
	return f
}

// HandleLogsFollowData server.go proxy_data 分派入口（proxyId="logs:<followId>"）
func (s *Server) HandleLogsFollowData(proxyID string, data []byte) {
	followID := strings.TrimPrefix(proxyID, logsFollowProxyPrefix)
	logsFollowersMu.Lock()
	f := logsFollowers[followID]
	logsFollowersMu.Unlock()
	if f == nil {
		return // 流已断开/不存在：静默丢弃，agent 侧上限会自行收尾
	}
	select {
	case f.ch <- string(data):
	default:
		// 背压丢弃：堵住会拖死 agent 消息读循环，256 行缓冲内个人场景够用
	}
}

// HandleLogsFollowClose server.go proxy_close 分派入口：终止原因转 eof 帧
func (s *Server) HandleLogsFollowClose(proxyID, reason string) {
	followID := strings.TrimPrefix(proxyID, logsFollowProxyPrefix)
	f := removeLogsFollower(followID)
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

// handleAgentLogsFollow POST /api/agents/{id}/logs/follow（F4/F5/F6）
func (s *Server) handleAgentLogsFollow(w http.ResponseWriter, r *http.Request, agentID string) {
	var payload logsQueryPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "Invalid request body")
		return
	}
	// 校验与 query 完全同规则（F6）；since_minutes 对跟随无意义，忽略
	if err := validateLogsQuery(&payload); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	followID := protocol.GenerateIDWithPrefix("logf")
	f, ok := registerLogsFollower(followID, agentID)
	if !ok {
		s.handleError(w, r, http.StatusTooManyRequests, "too many active log follows")
		return
	}

	stopAgent := func() {
		_, _ = s.CallAgent(agentID, "logs.follow.stop", map[string]interface{}{"followId": followID})
	}

	resp, err := s.CallAgent(agentID, "logs.follow.start", map[string]interface{}{
		"followId": followID,
		"type":     payload.Type,
		"source":   payload.Source,
		"tail":     payload.Tail,
		"grep":     payload.Grep,
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
		removeLogsFollower(followID)
		if _, isFollowErr := err.(*agentFollowError); isFollowErr {
			s.handleError(w, r, http.StatusBadGateway, err.Error())
		} else {
			s.handleError(w, r, http.StatusBadGateway, "failed to reach agent: "+err.Error())
		}
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		removeLogsFollower(followID)
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
		// 客户端断开或 eof 收尾：确保 agent 侧进程停掉并清表
		if removeLogsFollower(followID) != nil {
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
		case line := <-f.ch:
			if !writeFrame(map[string]interface{}{"data": line}) {
				return
			}
		case reason := <-f.done:
			// 先排干已缓冲的数据帧再收尾：回填行与退出 close 可能同时
			// 就绪（select 随机选中 done），不排干会丢尾部数据
			for {
				select {
				case line := <-f.ch:
					if !writeFrame(map[string]interface{}{"data": line}) {
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

// agentFollowError agent 拒绝 follow（区别于连不上，透传原错误信息）
type agentFollowError struct{ msg string }

func (e *agentFollowError) Error() string { return e.msg }
