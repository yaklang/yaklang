package asyncexecmanager_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
)

// This resource needs no Yak engine, ReAct loop or tool types. It owns its
// observations and implements its own inspector; the manager only owns its run.
type exampleResource struct {
	mu     sync.Mutex
	stdout string
}

func (r *exampleResource) Execute(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	r.stdout = "work finished"
	r.mu.Unlock()
	return nil
}

func (r *exampleResource) Inspector() asyncexecmanager.Inspector { return r }

func (r *exampleResource) Inspect(ctx context.Context) (asyncexecmanager.Observation, error) {
	if err := ctx.Err(); err != nil {
		return asyncexecmanager.Observation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return asyncexecmanager.Observation{Stdout: r.stdout}, nil
}

func ExampleManager() {
	ctx := context.Background()
	manager, err := asyncexecmanager.New(ctx, asyncexecmanager.Config{})
	if err != nil {
		panic(err)
	}
	defer manager.Close()
	receipt, err := manager.Start(asyncexecmanager.Request{
		ID: "example", Resource: &exampleResource{},
	})
	if err != nil {
		panic(err)
	}
	final, err := manager.Wait(ctx, receipt.ID)
	if err != nil {
		panic(err)
	}
	inspector, err := manager.Inspector(receipt.ID)
	if err != nil {
		panic(err)
	}
	observation, err := inspector.Inspect(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(final.ID, final.State, observation.Stdout)
	// Output: example completed work finished
}

func ExampleNewResource() {
	ctx := context.Background()
	manager, err := asyncexecmanager.New(ctx, asyncexecmanager.Config{})
	if err != nil {
		panic(err)
	}
	defer manager.Close()

	// Adapt existing methods without implementing Resource or Inspector.
	work := &exampleResource{}
	resource, err := asyncexecmanager.NewResource(work.Execute, work.Inspect)
	if err != nil {
		panic(err)
	}
	receipt, err := manager.Start(asyncexecmanager.Request{Resource: resource})
	if err != nil {
		panic(err)
	}
	final, err := manager.Wait(ctx, receipt.ID)
	if err != nil {
		panic(err)
	}
	snapshot, err := manager.Inspect(ctx, receipt.ID)
	if err != nil {
		panic(err)
	}
	fmt.Println(final.State, snapshot.Observation.Stdout)
	// Output: completed work finished
}
