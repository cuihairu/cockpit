package openwrt

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client OpenWrt ubus RPC client
type Client struct {
	endpoint string
	username string
	password string
	timeout  time.Duration
	client   *http.Client
}

// Config OpenWrt configuration
type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	Timeout     time.Duration
	InsecureTLS bool
}

// NewClient creates OpenWrt client
func NewClient(cfg Config) *Client {
	scheme := "https"
	if cfg.Port == 80 || cfg.Port == 0 {
		scheme = "http"
	}

	endpoint := fmt.Sprintf("%s://%s:%d/ubus", scheme, cfg.Host, cfg.Port)
	if cfg.Port == 0 {
		if scheme == "https" {
			endpoint = fmt.Sprintf("https://%s/ubus", cfg.Host)
		} else {
			endpoint = fmt.Sprintf("http://%s/ubus", cfg.Host)
		}
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		endpoint: endpoint,
		username: cfg.Username,
		password: cfg.Password,
		timeout:  timeout,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: cfg.InsecureTLS,
				},
			},
		},
	}
}

// RPCRequest ubus RPC request
type RPCRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// RPCResponse ubus RPC response
type RPCResponse struct {
	Jsonrpc string    `json:"jsonrpc"`
	ID      int       `json:"id"`
	Result  RPCResult `json:"result"`
	Error   *RPCError `json:"error,omitempty"`
}

// RPCResult RPC result
type RPCResult struct {
	Status string        `json:"status"`
	Data   []interface{} `json:"data"`
}

// RPCError RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// call calls ubus RPC
func (c *Client) call(namespace string, procedure string, params ...interface{}) ([]byte, error) {
	// Login first to get session
	session, err := c.login()
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	// Build RPC request
	rpcReq := RPCRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "call",
		Params: []interface{}{
			session,
			namespace,
			procedure,
			params,
		},
	}

	jsonData, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP error: %d - %s", resp.StatusCode, string(body))
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
	}

	// Extract data
	if len(rpcResp.Result.Data) > 0 {
		return json.Marshal(rpcResp.Result.Data[0])
	}

	return body, nil
}

// login gets session token
func (c *Client) login() (string, error) {
	loginReq := RPCRequest{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "call",
		Params: []interface{}{
			"00000000000000000000000000000000",
			"session",
			"login",
			[]interface{}{
				map[string]string{
					"username": c.username,
					"password": c.password,
				},
			},
		},
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return "", fmt.Errorf("marshal login request: %w", err)
	}

	req, err := http.NewRequest("POST", c.endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do login request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read login response: %w", err)
	}

	var loginResp RPCResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return "", fmt.Errorf("unmarshal login response: %w", err)
	}

	if loginResp.Error != nil {
		return "", fmt.Errorf("login error: %s", loginResp.Error.Message)
	}

	// Extract session
	if len(loginResp.Result.Data) > 0 {
		if data, ok := loginResp.Result.Data[0].(map[string]interface{}); ok {
			if ubusRPCSession, ok := data["ubus_rpc_session"].(string); ok {
				return ubusRPCSession, nil
			}
		}
	}

	return "", fmt.Errorf("session not found in response")
}

// SystemInfo system information
type SystemInfo struct {
	Uptime    int64      `json:"uptime"`
	Load      []float64  `json:"load"`
	Memory    MemoryInfo `json:"memory"`
	Swap      MemoryInfo `json:"swap"`
	Localtime int64      `json:"localtime"`
}

// MemoryInfo memory information
type MemoryInfo struct {
	Total     int64 `json:"total"`
	Free      int64 `json:"free"`
	Available int64 `json:"available"`
	Buffered  int64 `json:"buffered"`
	Cached    int64 `json:"cached"`
}

// GetSystemInfo gets system information
func (c *Client) GetSystemInfo() (*SystemInfo, error) {
	body, err := c.call("system", "info")
	if err != nil {
		return nil, err
	}

	var info SystemInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("parse system info: %w", err)
	}

	return &info, nil
}

// Interface network interface
type Interface struct {
	Name      string   `json:"interface"`
	Up        bool     `json:"up"`
	Enabled   bool     `json:"enabled"`
	Static    bool     `json:"static"`
	Device    string   `json:"l3_device"`
	Autostart bool     `json:"autostart"`
	Metrics   Metrics  `json:"metrics"`
	IPv4      []string `json:"ipv4-address"`
	IPv6      []string `json:"ipv6-address"`
	DNS       []string `json:"dns-server"`
}

// Metrics interface metrics
type Metrics struct {
	BytesReceived   int64 `json:"rx_bytes"`
	BytesSent       int64 `json:"tx_bytes"`
	PacketsReceived int64 `json:"rx_packets"`
	PacketsSent     int64 `json:"tx_packets"`
}

// ListInterfaces lists network interfaces
func (c *Client) ListInterfaces() ([]Interface, error) {
	body, err := c.call("network.interface", "dump")
	if err != nil {
		return nil, err
	}

	var resp struct {
		Interfaces []Interface `json:"interface"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse interfaces: %w", err)
	}

	return resp.Interfaces, nil
}

// GetInterface gets single interface info
func (c *Client) GetInterface(name string) (*Interface, error) {
	body, err := c.call("network.interface", "status", map[string]string{
		"interface": name,
	})
	if err != nil {
		return nil, err
	}

	var iface Interface
	if err := json.Unmarshal(body, &iface); err != nil {
		return nil, fmt.Errorf("parse interface: %w", err)
	}

	return &iface, nil
}

// Route route
type Route struct {
	Target  string `json:"target"`
	Mask    int    `json:"mask"`
	Gateway string `json:"nexthop"`
	Device  string `json:"device"`
	Source  string `json:"source"`
	Metric  int    `json:"metric"`
	Table   int    `json:"table"`
	Type    string `json:"type"`
	OnLink  bool   `json:"onlink"`
}

// ListRoutes lists routes
func (c *Client) ListRoutes() ([]Route, error) {
	body, err := c.call("network.route", "dump")
	if err != nil {
		return nil, err
	}

	var resp struct {
		Routes []Route `json:"route"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse routes: %w", err)
	}

	return resp.Routes, nil
}

// FirewallZone firewall zone
type FirewallZone struct {
	Name    string   `json:"name"`
	Network []string `json:"network"`
	Input   string   `json:"input"`
	Output  string   `json:"output"`
	Forward string   `json:"forward"`
	Masq    bool     `json:"masq"`
}

// FirewallRule firewall rule
type FirewallRule struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Family string `json:"family"`
	Proto  string `json:"proto"`
	Src    string `json:"src"`
	Dst    string `json:"dest"`
	Target string `json:"target"`
}

// FirewallRedirect firewall redirect
type FirewallRedirect struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Src         string `json:"src"`
	SrcIP       string `json:"src_ip"`
	SrcPort     string `json:"src_port"`
	Dst         string `json:"dest"`
	DstIP       string `json:"dest_ip"`
	DstPort     string `json:"dest_port"`
	Target      string `json:"target"`
	Reflection  bool   `json:"reflection"`
}

// GetFirewallZones gets firewall zones
func (c *Client) GetFirewallZones() ([]FirewallZone, error) {
	body, err := c.call("firewall", "get_zones")
	if err != nil {
		return nil, err
	}

	var zones []FirewallZone
	if err := json.Unmarshal(body, &zones); err != nil {
		return nil, fmt.Errorf("parse zones: %w", err)
	}

	return zones, nil
}

// GetFirewallRules gets firewall rules
func (c *Client) GetFirewallRules() ([]FirewallRule, error) {
	body, err := c.call("firewall", "get_rules")
	if err != nil {
		return nil, err
	}

	var rules []FirewallRule
	if err := json.Unmarshal(body, &rules); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}

	return rules, nil
}

// GetFirewallRedirects gets firewall redirects
func (c *Client) GetFirewallRedirects() ([]FirewallRedirect, error) {
	body, err := c.call("firewall", "get_redirects")
	if err != nil {
		return nil, err
	}

	var redirects []FirewallRedirect
	if err := json.Unmarshal(body, &redirects); err != nil {
		return nil, fmt.Errorf("parse redirects: %w", err)
	}

	return redirects, nil
}

// WirelessRadio wireless radio config
type WirelessRadio struct {
	Name      string `json:"name"`
	Channel   int    `json:"channel"`
	Frequency int    `json:"frequency"`
	HWMode    string `json:"hwmode"`
	HTMode    string `json:"htmode"`
}

// WirelessSSID wireless network
type WirelessSSID struct {
	SSID       string `json:"ssid"`
	Encryption string `json:"encryption"`
	Disabled   bool   `json:"disabled"`
	Network    string `json:"network"`
	Ifname     string `json:"ifname"`
	Device     string `json:"device"`
}

// GetWirelessStatus gets wireless status
func (c *Client) GetWirelessStatus() ([]struct {
	Radios []WirelessRadio `json:"radios"`
	SSIDs  []WirelessSSID  `json:"interfaces"`
}, error) {
	body, err := c.call("network.wireless", "status")
	if err != nil {
		return nil, err
	}

	var result []struct {
		Radios []WirelessRadio `json:"radios"`
		SSIDs  []WirelessSSID  `json:"interfaces"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse wireless status: %w", err)
	}

	return result, nil
}

// DHCLease DHCP lease
type DHCLease struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Expires  int64  `json:"valid_until"`
}

// GetDHCPLoads gets DHCP leases
func (c *Client) GetDHCPLoads() ([]DHCLease, error) {
	body, err := c.call("uci", "get", map[string]string{
		"config": "dhcp",
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		DHCP []struct {
			Leasesfile string `json:"leasefile"`
		} `json:"dhcp"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse dhcp config: %w", err)
	}

	// Read lease file (needs file interface)
	// Return empty list for now, real implementation needs file access
	return []DHCLease{}, nil
}

// ReadFile reads file
func (c *Client) ReadFile(path string) (string, error) {
	body, err := c.call("file", "read", map[string]string{
		"path": path,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse file data: %w", err)
	}

	return result.Data, nil
}

// WriteFile writes file
func (c *Client) WriteFile(path, data string, mode string) error {
	_, err := c.call("file", "write", map[string]interface{}{
		"path": path,
		"data": data,
		"mode": mode,
	})
	return err
}

// Reboot reboots device
func (c *Client) Reboot() error {
	_, err := c.call("system", "reboot")
	return err
}

// LEDState LED state
type LEDState struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// GetLEDState gets LED state
func (c *Client) GetLEDState(name string) (*LEDState, error) {
	body, err := c.call("led", "get", map[string]string{
		"name": name,
	})
	if err != nil {
		return nil, err
	}

	var state LEDState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, fmt.Errorf("parse LED state: %w", err)
	}

	return &state, nil
}

// SetLEDState sets LED state
func (c *Client) SetLEDState(name, state string) error {
	_, err := c.call("led", "set", map[string]string{
		"name":  name,
		"state": state,
	})
	return err
}
