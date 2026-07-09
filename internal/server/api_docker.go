package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

const dockerAPIPrefix = "/api/docker/"

func (s *Server) registerDockerAPI(mux *http.ServeMux) {
	mux.HandleFunc(dockerAPIPrefix, func(w http.ResponseWriter, r *http.Request) {
		auth.Middleware(s.handleDocker)(w, r)
	})
}

func (s *Server) handleDocker(w http.ResponseWriter, r *http.Request) {
	agentID, method, params, err := parseDockerRequest(r)
	if err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	agent, ok := s.registry.Get(agentID)
	if !ok {
		s.handleError(w, r, http.StatusNotFound, "Agent not found")
		return
	}
	if !agent.HasCapability("docker-api") && !agent.HasCapability("docker") {
		s.handleError(w, r, http.StatusNotFound, "Docker capability not available")
		return
	}

	resp, err := s.CallAgent(agentID, method, params)
	if err != nil {
		status := http.StatusBadGateway
		if err == ErrAgentNotFound {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "timeout") {
			status = http.StatusGatewayTimeout
		}
		s.handleError(w, r, status, err.Error())
		return
	}

	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "Invalid agent response")
		return
	}
	if rpcResp.Status == "error" {
		s.handleError(w, r, http.StatusBadGateway, rpcResp.Error)
		return
	}

	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

func parseDockerRequest(r *http.Request) (string, string, map[string]interface{}, error) {
	segments := splitDockerPath(r.URL.Path)
	if len(segments) < 3 || segments[0] != "agents" {
		return "", "", nil, fmt.Errorf("expected /api/docker/agents/{agentId}/...")
	}

	agentID, err := pathSegment(segments[1])
	if err != nil || agentID == "" {
		return "", "", nil, fmt.Errorf("invalid agent id")
	}

	resource := segments[2]
	params, err := requestParams(r)
	if err != nil {
		return "", "", nil, err
	}

	switch resource {
	case "containers":
		method, err := parseDockerContainerRoute(r, segments, params)
		return agentID, method, params, err
	case "images":
		method, err := parseDockerImageRoute(r, segments, params)
		return agentID, method, params, err
	case "volumes":
		method, err := parseDockerVolumeRoute(r, segments, params)
		return agentID, method, params, err
	case "networks":
		if r.Method == http.MethodGet && len(segments) == 3 {
			return agentID, "docker.networks.list", params, nil
		}
	case "system":
		if r.Method == http.MethodGet && len(segments) == 4 && segments[3] == "info" {
			return agentID, "docker.system.info", params, nil
		}
	}

	return "", "", nil, fmt.Errorf("unsupported docker route")
}

func parseDockerContainerRoute(r *http.Request, segments []string, params map[string]interface{}) (string, error) {
	if r.Method == http.MethodGet && len(segments) == 3 {
		setQueryBool(params, r, "all")
		return "docker.containers.list", nil
	}
	if len(segments) < 4 {
		return "", fmt.Errorf("container id required")
	}

	id, err := pathSegment(segments[3])
	if err != nil || id == "" {
		return "", fmt.Errorf("invalid container id")
	}
	params["id"] = id

	if r.Method == http.MethodGet && len(segments) == 4 {
		return "docker.containers.get", nil
	}
	if r.Method == http.MethodDelete && len(segments) == 4 {
		setQueryBool(params, r, "force")
		setQueryBool(params, r, "volumes")
		return "docker.containers.remove", nil
	}
	if len(segments) != 5 {
		return "", fmt.Errorf("unsupported container route")
	}

	switch segments[4] {
	case "start":
		if r.Method == http.MethodPost {
			return "docker.containers.start", nil
		}
	case "stop":
		if r.Method == http.MethodPost {
			setQueryInt(params, r, "timeout")
			return "docker.containers.stop", nil
		}
	case "restart":
		if r.Method == http.MethodPost {
			setQueryInt(params, r, "timeout")
			return "docker.containers.restart", nil
		}
	case "pause":
		if r.Method == http.MethodPost {
			return "docker.containers.pause", nil
		}
	case "unpause":
		if r.Method == http.MethodPost {
			return "docker.containers.unpause", nil
		}
	case "logs":
		if r.Method == http.MethodGet {
			setQueryString(params, r, "tail")
			setQueryString(params, r, "since")
			setQueryBool(params, r, "follow")
			setQueryBool(params, r, "timestamps")
			setQueryBoolDefault(params, r, "stdout", true)
			setQueryBoolDefault(params, r, "stderr", true)
			return "docker.containers.logs", nil
		}
	case "stats":
		if r.Method == http.MethodGet {
			return "docker.containers.stats", nil
		}
	}

	return "", fmt.Errorf("unsupported container action")
}

func parseDockerImageRoute(r *http.Request, segments []string, params map[string]interface{}) (string, error) {
	if r.Method == http.MethodGet && len(segments) == 3 {
		setQueryBool(params, r, "all")
		return "docker.images.list", nil
	}
	if r.Method == http.MethodPost && len(segments) == 4 && segments[3] == "pull" {
		setQueryString(params, r, "ref")
		if params["ref"] == nil || params["ref"] == "" {
			return "", fmt.Errorf("ref required")
		}
		return "docker.images.pull", nil
	}
	if r.Method == http.MethodDelete && len(segments) == 4 {
		id, err := pathSegment(segments[3])
		if err != nil || id == "" {
			return "", fmt.Errorf("invalid image id")
		}
		params["id"] = id
		setQueryBool(params, r, "force")
		setQueryBool(params, r, "prune_children")
		return "docker.images.remove", nil
	}
	return "", fmt.Errorf("unsupported image route")
}

func parseDockerVolumeRoute(r *http.Request, segments []string, params map[string]interface{}) (string, error) {
	if r.Method == http.MethodGet && len(segments) == 3 {
		return "docker.volumes.list", nil
	}
	if r.Method == http.MethodDelete && len(segments) == 4 {
		name, err := pathSegment(segments[3])
		if err != nil || name == "" {
			return "", fmt.Errorf("invalid volume name")
		}
		params["name"] = name
		setQueryBool(params, r, "force")
		return "docker.volumes.remove", nil
	}
	return "", fmt.Errorf("unsupported volume route")
}

func splitDockerPath(path string) []string {
	trimmed := strings.Trim(strings.TrimPrefix(path, dockerAPIPrefix), "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func pathSegment(value string) (string, error) {
	return url.PathUnescape(value)
}

func requestParams(r *http.Request) (map[string]interface{}, error) {
	params := map[string]interface{}{}
	if r.Body == nil || r.Body == http.NoBody {
		return params, nil
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&params); err != nil {
		if err == io.EOF {
			return params, nil
		}
		return nil, fmt.Errorf("invalid request body")
	}
	return params, nil
}

func setQueryString(params map[string]interface{}, r *http.Request, key string) {
	if value := r.URL.Query().Get(key); value != "" {
		params[key] = value
	}
}

func setQueryBool(params map[string]interface{}, r *http.Request, key string) {
	if raw := r.URL.Query().Get(key); raw != "" {
		if value, err := strconv.ParseBool(raw); err == nil {
			params[key] = value
		}
	}
}

func setQueryBoolDefault(params map[string]interface{}, r *http.Request, key string, defaultValue bool) {
	if _, exists := params[key]; exists {
		return
	}
	params[key] = defaultValue
	setQueryBool(params, r, key)
}

func setQueryInt(params map[string]interface{}, r *http.Request, key string) {
	if raw := r.URL.Query().Get(key); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			params[key] = value
		}
	}
}
