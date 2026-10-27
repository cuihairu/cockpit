package server

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/oob"
	"github.com/cuihairu/cockpit/internal/oob/amt"
	"github.com/cuihairu/cockpit/internal/oob/dash"
	"github.com/cuihairu/cockpit/internal/oob/redfish"
	"github.com/cuihairu/cockpit/internal/oob/wsman"
)

type discoveryProvider struct {
	name string
	info oob.DeviceInfo
	err  error
}

func (p discoveryProvider) Name() string { return p.name }
func (p discoveryProvider) Probe(ctx context.Context, target oob.Target) (oob.DeviceInfo, error) {
	<-ctx.Done()
	return oob.DeviceInfo{}, ctx.Err()
}

func discoveryServer(t *testing.T, providers ...oob.Provider) *Server {
	t.Helper()
	server := newTestServerWithDB(t)
	registry := oob.NewRegistry()
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			t.Fatalf("register provider: %v", err)
		}
	}
	server.oobProviders = registry
	return server
}

func callDiscovery(server *Server, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/oob/discover", strings.NewReader(body))
	token, _ := server.auth.GenerateToken("1", "admin", "admin")
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	server.handleOobDiscover(recorder, request)
	return recorder
}

func TestOobDiscoverValidation(t *testing.T) {
	server := newTestServerWithDB(t)
	cases := []struct {
		name string
		body string
		want int
	}{
		{"bad json", `not-json`, http.StatusBadRequest},
		{"missing address", `{}`, http.StatusBadRequest},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			recorder := callDiscovery(server, test.body)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}

	if result := oobDiscoveryToResult(nil, oob.DeviceView{Capabilities: map[oob.Capability]oob.CapabilityView{}}, nil, nil); result.Capabilities == nil {
		t.Fatal("empty discovery result must serialize as an object")
	}
}

func TestOobDiscoverMergesReachableProviders(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/redfish/v1":
			w.WriteHeader(http.StatusUnauthorized)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/wsman"):
			w.Write([]byte(xml.Header + `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><IdentifyResponse><ProductName>Intel(r) Active Management Technology</ProductName></IdentifyResponse></s:Body></s:Envelope>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(testServer.Close)

	sharedClient := testServer.Client()
	providers := []oob.Provider{
		amt.New(amt.Options{Wsman: wsman.Options{Client: sharedClient}}),
		redfish.New(redfish.Options{Client: sharedClient}),
	}
	server := discoveryServer(t, providers...)
	recorder := callDiscovery(server, `{"address":"`+testServer.URL+`"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"attempted":["amt","redfish"]`) {
		t.Fatalf("attempted paths missing: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"reachable_undetermined":true,"attempted":["amt","redfish"]`) {
		t.Fatalf("reachable-undetermined evidence missing: %s", recorder.Body.String())
	}

	logs, _, err := server.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobDiscover})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Username != "unknown" {
		t.Fatalf("audit = %+v", logs)
	}
	if !strings.Contains(logs[0].Details, `"attempted":["amt","redfish"]`) {
		t.Fatalf("audit attempted paths = %s", logs[0].Details)
	}
}

func TestOobDiscoverMergesCapabilities(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/redfish/v1":
			w.Write([]byte(`{"RedfishVersion":"1.16.0","Systems":{"@odata.id":"/redfish/v1/Systems"},"Managers":{"@odata.id":"/redfish/v1/Managers"}}`))
		case "/redfish/v1/Systems":
			w.Write([]byte(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`))
		case "/redfish/v1/Systems/1":
			w.Write([]byte(`{"Actions":{"#ComputerSystem.Reset":{}}}`))
		case "/redfish/v1/Managers":
			w.Write([]byte(`{"Members":[{"@odata.id":"/redfish/v1/Managers/1"}]}`))
		case "/redfish/v1/Managers/1":
			w.Write([]byte(`{"Model":"CovBMC"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(testServer.Close)

	server := discoveryServer(t, redfish.New(redfish.Options{Client: testServer.Client()}))
	recorder := callDiscovery(server, `{"address":"`+testServer.URL+`","hints":{"line":"server"}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"power":{"state":{"supported":true,"enabled":true,"reachable":true},"source":"redfish"}`) {
		t.Fatalf("power negotiation missing: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"inventory":{"state":{"supported":true,"enabled":true,"reachable":true},"source":"redfish"}`) {
		t.Fatalf("inventory negotiation missing: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"errors":[{"provider":"redfish"`) {
		t.Fatalf("unexpected redfish discovery errors: %s", recorder.Body.String())
	}

	logs, _, err := server.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobDiscover})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Username != "unknown" {
		t.Fatalf("audit = %+v", logs)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/oob/discover", strings.NewReader(`{"address":"`+testServer.URL+`"}`))
	request.Header.Set("Authorization", "Bearer "+func() string {
		token, _ := server.auth.GenerateToken("1", "admin", "admin")
		return token
	}())
	mux := http.NewServeMux()
	server.registerOobAPI(mux)
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("authenticated discovery status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	logs, _, err = server.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionOobDiscover})
	if err != nil || logs[0].Username != "admin" {
		t.Fatalf("authenticated audit = %+v, %v", logs, err)
	}
}

func TestOobDiscoverHonorsCanceledRequest(t *testing.T) {
	provider := discoveryProvider{name: "blocked"}
	server := discoveryServer(t, provider)
	request := httptest.NewRequest(http.MethodPost, "/api/oob/discover", strings.NewReader(`{"address":"10.0.0.8"}`))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	request = request.WithContext(canceled)
	token, _ := server.auth.GenerateToken("1", "admin", "admin")
	request.Header.Set("Authorization", "Bearer "+token)

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.handleOobDiscover(recorder, request)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("discovery ignored canceled request")
	}
	if !strings.Contains(recorder.Body.String(), `"error_text":"context canceled"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestOobDiscoverDefaults(t *testing.T) {
	if got := defaultOOBProbeTimeout; got != 5*time.Second {
		t.Fatalf("default timeout = %v", got)
	}
	registry := newOOBRegistry()
	names := providerNames(registry)
	if len(names) != 3 || names[0] != amt.Name || names[1] != dash.Name || names[2] != redfish.Name {
		t.Fatalf("registry names = %v", names)
	}

	timeoutCases := []struct {
		seconds float64
		want    time.Duration
	}{
		{0, defaultOOBProbeTimeout},
		{-1, defaultOOBProbeTimeout},
		{31, defaultOOBProbeTimeout},
		{2.5, 2500 * time.Millisecond},
	}
	if err := registry.Register(discoveryProvider{name: "amt"}); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate provider registration = %v", err)
	}
	for _, test := range timeoutCases {
		req := oobDiscoverRequest{Timeout: &test.seconds}
		if got := req.timeout(); got != test.want {
			t.Fatalf("timeout(%v) = %v, want %v", test.seconds, got, test.want)
		}
	}
}
