package server

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/Kiisanz/k11-addon-sdk/pkg/addonapi"
	"github.com/Kiisanz/k11-addon-sdk/pkg/ipc"
)

type mockExecutor struct {
	executeFunc func(ctx context.Context, input map[string]interface{}, emitter addonapi.EventEmitter) (map[string]interface{}, error)
}

func (m *mockExecutor) Execute(ctx context.Context, input map[string]interface{}, emitter addonapi.EventEmitter) (map[string]interface{}, error) {
	return m.executeFunc(ctx, input, emitter)
}

func TestServer_ExecutionCleanup(t *testing.T) {
	manifest := addonapi.Manifest{ID: "test-addon", Name: "test-addon"}
	srv := NewServer(manifest)

	execCount := 0
	srv.RegisterExecutor("test.kind", &mockExecutor{
		executeFunc: func(ctx context.Context, input map[string]interface{}, emitter addonapi.EventEmitter) (map[string]interface{}, error) {
			execCount++
			return map[string]interface{}{"ok": true}, nil
		},
	})

	rClient, wServer := io.Pipe()
	rServer, wClient := io.Pipe()

	clientConn := ipc.NewConnection(rClient, wClient)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		srv.ServeStream(rServer, wServer)
	}()
	go func() {
		defer wg.Done()
		clientConn.Serve()
	}()

	// 1. Test Successful Execution Cleanup
	_, err := clientConn.Call(context.Background(), "addon.execute", map[string]interface{}{
		"kind":        "test.kind",
		"executionId": "exec-1",
		"runId":       "run-1",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	// activeContexts should be cleaned up immediately after execution
	srv.activeMu.Lock()
	if len(srv.activeContexts) != 0 {
		t.Errorf("activeContexts leaked: %d", len(srv.activeContexts))
	}
	if len(srv.cancellations) != 0 {
		t.Errorf("cancellations leaked: %d", len(srv.cancellations))
	}
	srv.activeMu.Unlock()

	// 2. Test Cancel Before Execute (Tombstone)
	// We simulate the race condition: client sends cancel, then execute arrives later.
	clientConn.Notify("addon.cancel", map[string]string{"executionId": "exec-2"})

	// Wait a tiny bit for the notification to be processed
	_, err = clientConn.Call(context.Background(), "addon.manifest", nil)
	if err != nil {
		t.Fatalf("sync call failed: %v", err)
	}

	// Now try to execute the canceled execution
	srv.RegisterExecutor("test.slow", &mockExecutor{
		executeFunc: func(ctx context.Context, input map[string]interface{}, emitter addonapi.EventEmitter) (map[string]interface{}, error) {
			t.Fatal("Should not be executed since it was canceled before execute")
			return nil, nil
		},
	})

	_, err = clientConn.Call(context.Background(), "addon.execute", map[string]interface{}{
		"kind":        "test.slow",
		"executionId": "exec-2",
		"runId":       "run-1",
	})

	if err == nil {
		t.Fatalf("expected context canceled error, got nil")
	}

	// Ensure the tombstone was cleaned up during the failed execution
	srv.activeMu.Lock()
	if len(srv.cancellations) != 0 {
		t.Errorf("cancellations tombstone leaked after execution: %d", len(srv.cancellations))
	}
	srv.activeMu.Unlock()

	rClient.Close()
	wClient.Close()
	rServer.Close()
	wServer.Close()
	wg.Wait()
}
