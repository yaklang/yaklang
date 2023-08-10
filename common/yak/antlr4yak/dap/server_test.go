package dap

import (
	"net"
	"testing"
	"time"

	"github.com/google/go-dap"
	"github.com/yaklang/yaklang/common/log"
)

var (
	StopOnEntry = true
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func runTest(t *testing.T, name string, generateFunc GernerateFuncTyp, test func(s *DAPServer, c *Client, program string)) {
	serverStopped := make(chan struct{})

	server, _ := startDAPServer(t, serverStopped)
	client := NewTestClient(server.listener.Addr().String())
	defer client.Close()

	tc, removeFunc := generateFunc()
	defer removeFunc()

	test(server, client, tc)
	<-serverStopped
}

func startDAPServer(t *testing.T, serverStopped chan struct{}) (server *DAPServer, forceStop chan struct{}) {
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
	// Give server time to start listening for clients
	time.Sleep(100 * time.Millisecond)

	go func() {
		defer func() {
			if serverStopped != nil {
				close(serverStopped)
			}
		}()
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

func checkErrorMessageId(er *dap.ErrorMessage, id int) bool {
	return er != nil && er.Id == id
}

func checkErrorMessageFormat(er *dap.ErrorMessage, fmt string) bool {
	return er != nil && er.Format == fmt
}

func TestStopNoCilent(t *testing.T) {
	for name, triggerStop := range map[string]func(s *DAPServer, forceStop chan struct{}){
		"force":          func(s *DAPServer, forceStop chan struct{}) { close(forceStop) },
		"listener_close": func(s *DAPServer, forceStop chan struct{}) { s.listener.Close() },
	} {
		t.Run(name, func(t *testing.T) {
			serverStopped := make(chan struct{})
			server, forceStop := startDAPServer(t, serverStopped)

			triggerStop(server, forceStop)
			<-serverStopped
			verifyServerStopped(t, server)
		})
	}
}

func TestStopNoTarget(t *testing.T) {
	for name, triggerStop := range map[string]func(c *Client, forceStop chan struct{}){
		"force":              func(c *Client, forceStop chan struct{}) { close(forceStop) },
		"client_close":       func(c *Client, forceStop chan struct{}) { c.Close() },
		"disconnect_request": func(c *Client, forceStop chan struct{}) { c.DisconnectRequest() },
	} {
		t.Run(name, func(t *testing.T) {
			serverStopped := make(chan struct{})
			server, forceStop := startDAPServer(t, serverStopped)
			client := NewTestClient(server.listener.Addr().String())
			defer client.Close()

			client.InitializeRequest()
			client.ExpectInitializeResponseAndCapabilities(t)

			triggerStop(client, forceStop)
			<-serverStopped
			verifyServerStopped(t, server)
		})
	}
}

func TestStopWithTarget(t *testing.T) {
	for name, triggerStop := range map[string]func(c *Client, forceStop chan struct{}){
		"force":                  func(c *Client, forceStop chan struct{}) { close(forceStop) },
		"client_close":           func(c *Client, forceStop chan struct{}) { c.Close() },
		"disconnect_before_exit": func(c *Client, forceStop chan struct{}) { c.DisconnectRequest() },
		"disconnect_after_exit": func(c *Client, forceStop chan struct{}) {
			c.ContinueRequest(1)
			c.ExpectContinueResponse(t)
			c.DisconnectRequest()
			c.ExpectOutputEventDetaching(t)
			c.ExpectDisconnectResponse(t)
			c.ExpectTerminateEvent(t)
		},
	} {
		t.Run(name, func(t *testing.T) {
			serverStopped := make(chan struct{})
			server, forceStop := startDAPServer(t, serverStopped)
			client := NewTestClient(server.listener.Addr().String())
			defer client.Close()

			client.InitializeRequest()
			client.ExpectInitializeResponseAndCapabilities(t)
			tc, removeFunc := GenerateSimpleYakTestCase()
			defer removeFunc()

			client.LaunchRequest("debug", tc, StopOnEntry)
			client.ExpectInitializedEvent(t)
			client.ExpectLaunchResponse(t)

			triggerStop(client, forceStop)
			<-serverStopped
			verifyServerStopped(t, server)
		})
	}
}

func TestForceStopWhileStopping(t *testing.T) {
	serverStopped := make(chan struct{})
	server, forceStop := startDAPServer(t, serverStopped)
	client := NewTestClient(server.listener.Addr().String())

	client.InitializeRequest()
	client.ExpectInitializeResponseAndCapabilities(t)
	tc, removeFunc := GenerateSimpleYakTestCase()
	defer removeFunc()

	client.LaunchRequest("exec", tc, StopOnEntry)
	client.ExpectInitializedEvent(t)
	client.Close()
	time.Sleep(time.Microsecond)
	close(forceStop)
	<-serverStopped
	verifyServerStopped(t, server)
}

func TestLaunchStopOnEntry(t *testing.T) {
	runTest(t, "stopOnEntry", GenerateSimpleYakTestCase, func(server *DAPServer, client *Client, program string) {
		// 1 >> initialize, << initialize
		client.InitializeRequest()
		initResp := client.ExpectInitializeResponseAndCapabilities(t)
		if initResp.Seq != 0 || initResp.RequestSeq != 1 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=1", initResp)
		}

		// 2 >> launch,  << initialized, << launch
		client.LaunchRequest("exec", program, StopOnEntry)
		initEvent := client.ExpectInitializedEvent(t)
		if initEvent.Seq != 0 {
			t.Errorf("\ngot %#v\nwant Seq=0", initEvent)
		}
		launchResp := client.ExpectLaunchResponse(t)
		if launchResp.Seq != 0 || launchResp.RequestSeq != 2 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=2", launchResp)
		}

		// 3 >> setBreakpoints, << setBreakpoints
		client.SetBreakpointsRequest(program, nil)
		sbpResp := client.ExpectSetBreakpointsResponse(t)
		if sbpResp.Seq != 0 || sbpResp.RequestSeq != 3 || len(sbpResp.Body.Breakpoints) != 0 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=3, len(Breakpoints)=0", sbpResp)
		}

		// 4 >> setExceptionBreakpoints, << setExceptionBreakpoints
		client.SetExceptionBreakpointsRequest()
		sebpResp := client.ExpectSetExceptionBreakpointsResponse(t)
		if sebpResp.Seq != 0 || sebpResp.RequestSeq != 4 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=4", sebpResp)
		}
		// 5 >> configurationDone, << stopped, << configurationDone
		client.ConfigurationDoneRequest()
		stopEvent := client.ExpectStoppedEvent(t)
		if stopEvent.Seq != 0 ||
			stopEvent.Body.Reason != "entry" ||
			stopEvent.Body.ThreadId != 0 ||
			!stopEvent.Body.AllThreadsStopped {
			t.Errorf("\ngot %#v\nwant Seq=0, Body={Reason=\"entry\", ThreadId=0, AllThreadsStopped=true}", stopEvent)
		}
		client.ExpectOutputEventHelpInfo(t)
		cdResp := client.ExpectConfigurationDoneResponse(t)
		if cdResp.Seq != 0 || cdResp.RequestSeq != 5 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=5", cdResp)
		}

		// 一开始stopOnEntry,所以要continue,由于continue后会直接执行结束,所以会收到terminated事件
		// 6 >> continue, << continue, << terminated
		client.ContinueRequest(1)
		cResp := client.ExpectContinueResponse(t)
		if cResp.Seq != 0 || cResp.RequestSeq != 6 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=6", cResp)
		}
		termEvent := client.ExpectTerminatedEvent(t)
		if termEvent.Seq != 0 {
			t.Errorf("\ngot %#v\nwant Seq=0", termEvent)
		}

		// 7 >> threads, << threads
		client.ThreadsRequest()
		tResp := client.ExpectThreadsResponse(t)
		if tResp.Seq != 0 || tResp.RequestSeq != 7 || len(tResp.Body.Threads) != 1 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=6 len(Threads)=1", tResp)
		}
		if len(tResp.Body.Threads) < 1 || tResp.Body.Threads[0].Id != 0 || tResp.Body.Threads[0].Name != "[Yak 0] global code" {
			t.Errorf("\ngot %#v\nwant Id=0, Name=\"[Yak 0] global code\"", tResp)
		}

		// 8 >> stackTrace, << error
		client.StackTraceRequest(1, 0, 20)
		steResp := client.ExpectInvisibleErrorResponse(t)
		if steResp.Seq != 0 || steResp.RequestSeq != 8 || steResp.Success || !checkErrorMessageFormat(steResp.Body.Error, "Unable to produce stack trace: Can't found Goroutine 1 stack trace") {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=8 Format=\"Unable to produce stack trace: Can't found Goroutine 1 stack trace\"", steResp)
		}

		// 9 >> stackTrace, << stackTrace
		client.StackTraceRequest(0, 0, 20)
		stResp := client.ExpectStackTraceResponse(t)
		if stResp.Seq != 0 || stResp.RequestSeq != 9 || !stResp.Success || stResp.Body.TotalFrames != 1 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=8 len(StackTraces)=1 ", stResp)
		}

		// 10 >> evaluate, << error
		client.EvaluateRequest("{", 0, "repl")
		erResp := client.ExpectInvisibleErrorResponse(t)
		_ = erResp
		if erResp.Seq != 0 || erResp.RequestSeq != 10 || !checkErrorMessageId(erResp.Body.Error, UnableToEvaluateExpression) {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=10 Id=%d", erResp, UnableToEvaluateExpression)
		}

		// 11 >> evaluate, << evaluate
		client.EvaluateRequest("1+1", 0 /*no frame specified*/, "repl")
		evResp := client.ExpectEvaluateResponse(t)
		if evResp.Seq != 0 || evResp.RequestSeq != 11 || evResp.Body.Result != "2" {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=10 Result=2", evResp)
		}

		// 12 >> continue, << continue, << terminated
		client.ContinueRequest(1)
		contResp := client.ExpectContinueResponse(t)
		if contResp.Seq != 0 || contResp.RequestSeq != 12 || !contResp.Body.AllThreadsContinued {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=12 Body.AllThreadsContinued=true", contResp)
		}
		termEvent := client.ExpectTerminatedEvent(t)
		if termEvent.Seq != 0 {
			t.Errorf("\ngot %#v\nwant Seq=0", termEvent)
		}

		// 13 >> disconnect, << disconnect
		client.DisconnectRequest()
		oed := client.ExpectOutputEventDetaching(t)
		if oed.Seq != 0 || oed.Body.Category != "console" {
			t.Errorf("\ngot %#v\nwant Seq=0 Category='console'", oed)
		}
		dResp := client.ExpectDisconnectResponse(t)
		if dResp.Seq != 0 || dResp.RequestSeq != 13 {
			t.Errorf("\ngot %#v\nwant Seq=0, RequestSeq=13", dResp)
		}
		client.ExpectTerminatedEvent(t)
	})
}
