//go:build linux

package scannode

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/proto"
)

func TestResilienceManagedInputTerminalBeforeAsyncBind(t *testing.T) {
	for _, kind := range []string{"cancel", "close"} {
		t.Run(kind, func(t *testing.T) {
			manager := newAISessionRuntimeManager(&recordingAISessionRuntimeDriver{})
			bind := validAISessionBindCommand()
			var err error
			if kind == "cancel" {
				_, err = manager.Cancel(validAISessionCancelCommand())
			} else {
				_, err = manager.Close(validAISessionCloseCommand())
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Bind(context.Background(), bind, nil, aiSessionRuntimeBindOptions{}); !errors.Is(err, errAISessionBindFenced) {
				t.Fatalf("late bind escaped %s: %v", kind, err)
			}
			// A delayed failure from an older bind cannot replace the terminal fence.
			ref := aiSessionRefFromBindCommand(bind)
			manager.RetireAfterBindFailure(ref)
			if _, err := manager.Bind(context.Background(), bind, nil, aiSessionRuntimeBindOptions{}); !errors.Is(err, errAISessionBindFenced) {
				t.Fatalf("failure overwrote terminal fence: %v", err)
			}
		})
	}
}

func TestResilienceManagedInputConsumerLifetime(t *testing.T) {
	t.Run("consumer stops during preparation", func(t *testing.T) {
		command := managedInputBindFixture(t, "synthetic_inventory", "hello")
		started := make(chan struct{})
		options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
		}))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		options.PreparationContext = ctx
		driver := &recordingAISessionRuntimeDriver{}
		manager := newAISessionRuntimeManager(driver)
		done := make(chan error, 1)
		go func() { _, err := manager.Bind(context.Background(), command, nil, options); done <- err }()
		<-started
		cancel()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("lost consumer installed a runtime")
			}
		case <-time.After(time.Second):
			t.Fatal("download outlived consumer")
		}
		if len(driver.bindings) != 0 {
			t.Fatal("consumer loss reached the engine")
		}
		manager.mu.Lock()
		pending := len(manager.bindings)
		manager.mu.Unlock()
		if pending != 0 {
			t.Fatal("consumer loss retained a bind reservation")
		}
	})
	t.Run("installed runtime survives consumer stop", func(t *testing.T) {
		command := managedInputBindFixture(t, "synthetic_inventory", "hello")
		options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hello")) }))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		options.PreparationContext = ctx
		driver := &recordingAISessionRuntimeDriver{}
		manager := newAISessionRuntimeManager(driver)
		if _, err := manager.Bind(context.Background(), command, nil, options); err != nil {
			t.Fatal(err)
		}
		workspace := driver.bindings[0].InputWorkspace
		defer workspace.Cleanup()
		cancel()
		if _, err := workspace.Read(context.Background(), command.InputManifest.Resources[0].RelativePath, 0, 5); err != nil {
			t.Fatalf("consumer stop damaged installed workspace: %v", err)
		}
		if _, err := os.Stat(workspace.RootForDiagnostics()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestResilienceManagedInputOldTerminalCannotDowngradeFence(t *testing.T) {
	manager := newAISessionRuntimeManager(&recordingAISessionRuntimeDriver{})
	cancel := validAISessionCancelCommand()
	cancel.Session.BindEpoch = 3
	if _, err := manager.Cancel(cancel); err != nil {
		t.Fatal(err)
	}
	old := proto.Clone(cancel).(*aiv1.CancelAISessionCommand)
	old.Metadata.CommandId = "old-cancel"
	old.Session.BindEpoch = 1
	if _, err := manager.Cancel(old); !errors.Is(err, errAISessionBindFenced) {
		t.Fatalf("stale cancel=%v", err)
	}
	close := validAISessionCloseCommand()
	close.Session.BindEpoch = 2
	if _, err := manager.Close(close); !errors.Is(err, errAISessionBindFenced) {
		t.Fatalf("stale close=%v", err)
	}
	manager.mu.Lock()
	epoch := manager.terminalTombstones[cancel.Session.SessionId].epoch
	manager.mu.Unlock()
	if epoch != 3 {
		t.Fatalf("terminal fence moved backwards to %d", epoch)
	}
}

func TestResilienceManagedInputCloseAcknowledgementSurvivesTerminalReordering(t *testing.T) {
	manager := newAISessionRuntimeManager(&recordingAISessionRuntimeDriver{})
	closeCommand := validAISessionCloseCommand()
	if result, err := manager.Close(closeCommand); err != nil || !result.acknowledge {
		t.Fatalf("first close acknowledgement=%v err=%v", result.acknowledge, err)
	}
	if _, err := manager.Cancel(validAISessionCancelCommand()); err != nil {
		t.Fatal(err)
	}
	if result, err := manager.Close(closeCommand); err != nil || !result.acknowledge {
		t.Fatalf("redelivered close acknowledgement=%v err=%v", result.acknowledge, err)
	}
	next := proto.Clone(closeCommand).(*aiv1.CloseAISessionCommand)
	next.Metadata.CommandId = "close-next-epoch"
	next.Session.BindEpoch++
	if result, err := manager.Close(next); err != nil || !result.acknowledge {
		t.Fatalf("newer close acknowledgement=%v err=%v", result.acknowledge, err)
	}
}

func TestAISessionBindAckIntervalHonorsBrokerPolicy(t *testing.T) {
	for _, tc := range []struct {
		policy nats.ConsumerConfig
		want   time.Duration
	}{
		{nats.ConsumerConfig{AckWait: 30 * time.Second}, 10 * time.Second},
		{nats.ConsumerConfig{AckWait: 300 * time.Millisecond}, 100 * time.Millisecond},
		{nats.ConsumerConfig{AckWait: time.Minute, BackOff: []time.Duration{600 * time.Millisecond, time.Minute}}, 200 * time.Millisecond},
	} {
		if got := aiSessionBindAckInterval(tc.policy); got != tc.want {
			t.Fatalf("ack interval=%v want=%v", got, tc.want)
		}
	}
}
