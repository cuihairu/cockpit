package server

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

type guacTestTCPAddrConn struct {
	net.Conn
}

func (c guacTestTCPAddrConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1")}
}

type guacTestMissingLocalAddrConn struct {
	net.Conn
}

func (c guacTestMissingLocalAddrConn) LocalAddr() net.Addr { return &net.TCPAddr{} }

func TestGuacamoleRejectsRelayWithoutConcreteLocalIP(t *testing.T) {
	s := covRemoteSetup(t)
	clientSide, serverSide := net.Pipe()
	t.Cleanup(func() {
		_ = clientSide.Close()
		_ = serverSide.Close()
	})
	old := dialGuacd
	dialGuacd = func(string, time.Duration) (net.Conn, error) {
		return guacTestMissingLocalAddrConn{Conn: clientSide}, nil
	}
	t.Cleanup(func() { dialGuacd = old })
	t.Setenv("GUACD_ADDR", "stub:4822")

	ticket := covTicket(t, s, map[string]string{
		"agent_id": "agent-route", "host": "127.0.0.1", "port": "5900", "protocol": "vnc",
	})
	_, rec := covDirectWS(t, s.handleGuacamoleWebSocket, "/api/remote/guacamole", ticket, false)
	if rec.code != http.StatusBadGateway {
		t.Fatalf("missing relay route status = %d, want 502", rec.code)
	}
}

func TestRelayAddressForGuacdDefaultsToConnectionRoute(t *testing.T) {
	t.Setenv("GUAC_RELAY_BIND_ADDR", "")
	t.Setenv("GUAC_RELAY_HOST", "")
	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4822}
	bind, host, err := relayAddressForGuacd(local)
	if err != nil {
		t.Fatal(err)
	}
	if bind != "127.0.0.1" || host != "127.0.0.1" {
		t.Fatalf("relay addresses = (%q, %q), want loopback route", bind, host)
	}
}

func TestRelayAddressForGuacdHonorsSafeOverrides(t *testing.T) {
	t.Setenv("GUAC_RELAY_BIND_ADDR", "127.0.0.1")
	t.Setenv("GUAC_RELAY_HOST", "relay.internal")
	bind, host, err := relayAddressForGuacd(&net.TCPAddr{IP: net.ParseIP("192.0.2.15")})
	if err != nil {
		t.Fatal(err)
	}
	if bind != "127.0.0.1" || host != "relay.internal" {
		t.Fatalf("relay addresses = (%q, %q), want (127.0.0.1, relay.internal)", bind, host)
	}
	relay, addr, err := startGuacRelayForGuacd("agent", "guac:addr-override", "target:22",
		func(string, *protocol.Message) error { return nil }, &net.TCPAddr{IP: net.ParseIP("192.0.2.15")})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	if !strings.HasPrefix(addr, "relay.internal:") {
		t.Fatalf("advertised address = %q, want relay.internal host", addr)
	}
}

func TestRelayAddressForGuacdRejectsUnsafeOrUnknownAddresses(t *testing.T) {
	tests := []struct {
		name  string
		local net.Addr
		bind  string
		host  string
	}{
		{name: "non tcp", local: &net.UnixAddr{Name: "sock", Net: "unix"}},
		{name: "missing ip", local: &net.TCPAddr{}},
		{name: "unspecified local ip", local: &net.TCPAddr{IP: net.IPv4zero}},
		{name: "invalid bind", local: &net.TCPAddr{IP: net.ParseIP("127.0.0.1")}, bind: "0.0.0.0"},
		{name: "invalid advertised host", local: &net.TCPAddr{IP: net.ParseIP("127.0.0.1")}, host: "bad host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GUAC_RELAY_BIND_ADDR", tt.bind)
			t.Setenv("GUAC_RELAY_HOST", tt.host)
			if _, _, err := relayAddressForGuacd(tt.local); err == nil {
				t.Fatal("expected invalid relay address to be rejected")
			}
		})
	}
}

func TestStartGuacRelayForGuacdRejectsAddressWithoutConcreteIP(t *testing.T) {
	t.Setenv("GUAC_RELAY_BIND_ADDR", "")
	t.Setenv("GUAC_RELAY_HOST", "")
	if _, _, err := startGuacRelayForGuacd("agent", "guac:addr-no-ip", "target:22",
		func(string, *protocol.Message) error { return nil }, &net.UnixAddr{Name: "sock", Net: "unix"}); err == nil {
		t.Fatal("expected startGuacRelayForGuacd to reject a local addr without concrete IP")
	}
}

func TestStartGuacRelayAtBindsSpecificIPAndAdvertisesHost(t *testing.T) {
	relay, addr, err := startGuacRelayAt("agent", "guac:addr-host", "target:22",
		func(string, *protocol.Message) error { return nil }, "127.0.0.1", "relay.internal")
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	if !strings.HasPrefix(addr, "relay.internal:") {
		t.Fatalf("advertised address = %q, want relay.internal host", addr)
	}
	if got := relay.ln.Addr().(*net.TCPAddr).IP.String(); got != "127.0.0.1" {
		t.Fatalf("listener bound to %q, want 127.0.0.1", got)
	}
	if _, _, err := startGuacRelayAt("agent", "guac:bad-bind", "target:22", nil, "0.0.0.0", "host"); err == nil {
		t.Fatal("unspecified bind should be rejected")
	}
	if _, _, err := startGuacRelayAt("agent", "guac:bad-host", "target:22", nil, "127.0.0.1", "bad host"); err == nil {
		t.Fatal("invalid advertised host should be rejected")
	}
	if _, _, err := startGuacRelayAt("agent", "guac:non-local", "target:22", nil, "192.0.2.254", "relay.internal"); err == nil {
		t.Fatal("non-local bind address should fail to listen")
	}
}
