package ipc

import (
	"context"
	"encoding/json"

	"io"
	"sync"
	"testing"
)

type pipeConn struct {
	io.ReadCloser
	io.WriteCloser
}

func (p *pipeConn) Close() error {
	p.ReadCloser.Close()
	p.WriteCloser.Close()
	return nil
}

func TestIPC_EventOrdering(t *testing.T) {
	rClient, wServer := io.Pipe()
	rServer, wClient := io.Pipe()

	clientConn := NewConnection(rClient, wClient)
	serverConn := NewConnection(rServer, wServer)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		clientConn.Serve()
	}()

	go func() {
		defer wg.Done()
		serverConn.Serve()
	}()

	eventsReceived := 0
	var mu sync.Mutex

	clientConn.Notifiers["workflow.emitEvent"] = func(params json.RawMessage) {
		mu.Lock()
		defer mu.Unlock()
		eventsReceived++
	}

	serverConn.Handlers["addon.execute"] = func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		serverConn.Notify("workflow.emitEvent", map[string]string{"foo": "bar"})
		return "success", nil
	}

	for i := 0; i < 1000; i++ {
		_, err := clientConn.Call(context.Background(), "addon.execute", nil)
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		mu.Lock()
		if eventsReceived != i+1 {
			t.Fatalf("FIFO ordering violation: expected %d events, got %d before response unblocked", i+1, eventsReceived)
		}
		mu.Unlock()
	}

	rClient.Close()
	wClient.Close()
	rServer.Close()
	wServer.Close()
}
