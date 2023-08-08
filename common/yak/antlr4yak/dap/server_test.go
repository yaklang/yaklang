package dap

import (
	"net"
	"testing"
	"time"
)

func startDAPServer(t *testing.T) (server *DAPServer, forceStop chan struct{}) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	stopChan := make(chan struct{})
	forceStop = make(chan struct{})

	server = NewDAPServer(&DAPServerConfig{
		listener: l,
		stopped:  stopChan,
	})

	server.Start()
	time.Sleep(100 * time.Millisecond)

	go func() {
		select {
		case <-stopChan:
			t.Log("server stop by call stop function")
		case <-forceStop:
			t.Log("server stop by force")
		}
		server.Stop()
	}()
	return
}

func verifyServerStopped(t *testing.T, server *DAPServer) {
	t.Helper()
	if server.listener != nil {
		if server.listener.Close() == nil {
			t.Error("server should have closed listener after shutdown")
		}
	}
	verifySessionStopped(t, server.session)
}

func verifySessionStopped(t *testing.T, session *DebugSession) {
	t.Helper()
	if session == nil {
		return
	}
	if session.conn == nil {
		t.Error("session must always have a connection")
	}
	verifyConnStopped(t, session.conn)
}

func verifyConnStopped(t *testing.T, conn net.Conn) {
	t.Helper()
	if conn.Close() == nil {
		t.Error("client connection should be closed after shutdown")
	}
}

func TestStopNoCilent(t *testing.T) {
	for name, triggerStop := range map[string]func(s *DAPServer, forceStop chan struct{}){
		"force":          func(s *DAPServer, forceStop chan struct{}) { close(forceStop) },
		"listener_close": func(s *DAPServer, forceStop chan struct{}) { s.listener.Close() },
	} {
		t.Run(name, func(t *testing.T) {
			server, forceStop := startDAPServer(t)

			triggerStop(server, forceStop)

			time.Sleep(100 * time.Millisecond)
			verifyServerStopped(t, server)
		})
	}
}

func TestStopNoTarget(t *testing.T) {
	for name, triggerStop := range map[string]func(c *TestClient, forceStop chan struct{}){
		"force":              func(c *TestClient, forceStop chan struct{}) { close(forceStop) },
		"client_close":       func(c *TestClient, forceStop chan struct{}) { c.Close() },
		"disconnect_request": func(c *TestClient, forceStop chan struct{}) { c.DisconnectRequest() },
	} {
		t.Run(name, func(t *testing.T) {
			server, forceStop := startDAPServer(t)
			client := NewTestClient(server.listener.Addr().String())
			defer client.Close()

			client.InitializeRequest()
			client.ExpectInitializeResponseAndCapabilities(t)

			triggerStop(client, forceStop)

			time.Sleep(100 * time.Millisecond)
			verifyServerStopped(t, server)
		})
	}
}
