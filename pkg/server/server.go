package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Kiisanz/k11-addon-sdk/pkg/addonapi"
	"github.com/Kiisanz/k11-addon-sdk/pkg/ipc"
	"sync"
	"time"
)

type Server struct {
	CapabilitiesCheck func() map[string]map[string]string
	manifest          addonapi.Manifest
	executors         map[string]addonapi.Executor
	invokers          map[string]func(context.Context, interface{}) (interface{}, error)

	activeContexts map[string]context.CancelFunc
	cancellations  map[string]time.Time
	activeMu       sync.Mutex
}

func NewServer(manifest addonapi.Manifest) *Server {
	if err := manifest.Validate(); err != nil {
		panic(fmt.Sprintf("Invalid addon manifest: %v", err))
	}
	return &Server{
		manifest:       manifest,
		executors:      make(map[string]addonapi.Executor),
		invokers:       make(map[string]func(context.Context, interface{}) (interface{}, error)),
		activeContexts: make(map[string]context.CancelFunc),
		cancellations:  make(map[string]time.Time),
	}
}

func (s *Server) RegisterExecutor(kind string, executor addonapi.Executor) {
	s.executors[kind] = executor
}

func (s *Server) RegisterInvoker(method string, handler func(context.Context, interface{}) (interface{}, error)) {
	s.invokers[method] = handler
}

type executeParams struct {
	Kind        string                 `json:"kind"`
	Input       map[string]interface{} `json:"input"`
	RunID       string                 `json:"runId"`
	ExecutionID string                 `json:"executionId"`
}

type emitParams struct {
	RunID       string            `json:"runId"`
	ExecutionID string            `json:"executionId"`
	Event       addonapi.RunEvent `json:"event"`
}

type remoteEmitter struct {
	conn        *ipc.Connection
	runID       string
	executionID string
}

func (r *remoteEmitter) Emit(event addonapi.RunEvent) {
	r.conn.Notify("workflow.emitEvent", emitParams{
		RunID:       r.runID,
		ExecutionID: r.executionID,
		Event:       event,
	})
}

func (s *Server) ServeStream(in io.Reader, out io.Writer) {
	conn := ipc.NewConnection(in, out)

	conn.Handlers["addon.capabilities"] = func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		if s.CapabilitiesCheck != nil {
			return s.CapabilitiesCheck(), nil
		}
		caps := make(map[string]map[string]string)
		for _, cap := range s.manifest.Capabilities {
			caps[cap] = map[string]string{"status": "available"}
		}
		return caps, nil
	}

	conn.Handlers["addon.manifest"] = func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return s.manifest, nil
	}

	conn.Handlers["addon.invoke"] = func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		var req struct {
			Method string      `json:"method"`
			Params interface{} `json:"params"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		handler, ok := s.invokers[req.Method]
		if !ok {
			return nil, fmt.Errorf("invoker not found: %s", req.Method)
		}
		return handler(ctx, req.Params)
	}

	conn.Handlers["addon.execute"] = func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		var req executeParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}

		executor, ok := s.executors[req.Kind]
		if !ok {
			return nil, fmt.Errorf("executor not found: %s", req.Kind)
		}

		emitter := &remoteEmitter{
			conn:        conn,
			runID:       req.RunID,
			executionID: req.ExecutionID,
		}

		execCtx, cancel := context.WithCancel(context.Background())
		s.activeMu.Lock()
		if _, canceled := s.cancellations[req.ExecutionID]; canceled {
			delete(s.cancellations, req.ExecutionID)
			s.activeMu.Unlock()
			cancel()
			return nil, context.Canceled
		}
		s.activeContexts[req.ExecutionID] = cancel
		s.activeMu.Unlock()

		defer func() {
			s.activeMu.Lock()
			delete(s.activeContexts, req.ExecutionID)
			delete(s.cancellations, req.ExecutionID)
			s.activeMu.Unlock()
			cancel()
		}()

		return executor.Execute(execCtx, req.Input, emitter)
	}

	conn.Notifiers["addon.cancel"] = func(params json.RawMessage) {
		var req struct {
			ExecutionID string `json:"executionId"`
		}
		if err := json.Unmarshal(params, &req); err == nil {
			s.activeMu.Lock()
			if cancel, ok := s.activeContexts[req.ExecutionID]; ok {
				cancel()
			} else {
				s.cancellations[req.ExecutionID] = time.Now()
				if len(s.cancellations) > 1000 {
					cutoff := time.Now().Add(-5 * time.Minute)
					for id, t := range s.cancellations {
						if t.Before(cutoff) {
							delete(s.cancellations, id)
						}
					}
					for id := range s.cancellations {
						if len(s.cancellations) <= 1000 {
							break
						}
						delete(s.cancellations, id)
					}
				}
			}
			s.activeMu.Unlock()
		}
	}

	// Blocks until EOF
	conn.Serve()
}
