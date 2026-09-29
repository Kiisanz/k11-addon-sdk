package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *string         `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *string         `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Connection struct {
	closed    bool
	r         *bufio.Reader
	w         io.Writer
	mu        sync.Mutex // protects writing
	nextID    int64
	pending   map[string]chan Response
	pendingMu sync.Mutex
	Handlers  map[string]func(ctx context.Context, params json.RawMessage) (interface{}, error)
	Notifiers map[string]func(params json.RawMessage)
}

func NewConnection(r io.Reader, w io.Writer) *Connection {
	return &Connection{
		r:         bufio.NewReader(r),
		w:         w,
		pending:   make(map[string]chan Response),
		Handlers:  make(map[string]func(ctx context.Context, params json.RawMessage) (interface{}, error)),
		Notifiers: make(map[string]func(params json.RawMessage)),
	}
}

func (c *Connection) cleanup(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	for id, ch := range c.pending {
		ch <- Response{Error: &ResponseError{Code: -32603, Message: "connection closed: " + err.Error()}}
		delete(c.pending, id)
	}
}

func (c *Connection) Serve() error {
	var finalErr error
	defer func() {
		if finalErr != nil {
			c.cleanup(finalErr)
		} else {
			c.cleanup(fmt.Errorf("connection closed"))
		}
	}()
	for {
		length := -1
		for {
			line, err := c.r.ReadString('\n')
			if err != nil {
				c.cleanup(err)
				if err == io.EOF {
					return nil
				}
				return err
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if strings.HasPrefix(line, "Content-Length: ") {
				length, _ = strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			}
		}

		if length < 0 {
			err := fmt.Errorf("missing Content-Length")
			c.cleanup(err)
			return err
		}
		if length > 10*1024*1024 { // 10MB limit
			err := fmt.Errorf("message too large")

			c.cleanup(err)
			return err
		}

		body := make([]byte, length)
		if _, err := io.ReadFull(c.r, body); err != nil {
			return err
		}

		// It could be a Request/Notification or a Response
		var base struct {
			JSONRPC string  `json:"jsonrpc"`
			ID      *string `json:"id,omitempty"`
			Method  *string `json:"method,omitempty"`
		}
		if err := json.Unmarshal(body, &base); err != nil {
			continue
		}

		if base.Method != nil {
			// Request or Notification
			var req Request
			json.Unmarshal(body, &req)

			if req.ID == nil {
				// Notification (FIFO synchronous)
				if handler, ok := c.Notifiers[req.Method]; ok {
					handler(req.Params)
				}
			} else {
				// Request (Async to unblock read loop)
				go c.handleRequest(req)
			}
		} else {
			// Response (FIFO synchronous)
			var res Response
			json.Unmarshal(body, &res)
			if res.ID != nil {
				c.pendingMu.Lock()
				ch, ok := c.pending[*res.ID]
				if ok {
					delete(c.pending, *res.ID)
				}
				c.pendingMu.Unlock()
				if ok {
					ch <- res
				}
			}
		}
	}
}

func (c *Connection) handleRequest(req Request) {
	if req.ID == nil {
		// Notification
		if handler, ok := c.Notifiers[req.Method]; ok {
			handler(req.Params)
		}
		return
	}

	// Request
	var res Response
	res.JSONRPC = "2.0"
	res.ID = req.ID

	if handler, ok := c.Handlers[req.Method]; ok {
		result, err := handler(context.Background(), req.Params)
		if err != nil {
			res.Error = &ResponseError{Code: -32000, Message: err.Error()}
		} else {
			if result == nil {
				res.Result = []byte("null")
			} else {
				b, _ := json.Marshal(result)
				res.Result = b
			}
		}
	} else {
		res.Error = &ResponseError{Code: -32601, Message: "Method not found"}
	}

	c.send(res)
}

func (c *Connection) send(msg interface{}) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(b))
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.w.Write([]byte(header)); err != nil {
		return err
	}
	_, err = c.w.Write(b)
	return err
}

func (c *Connection) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := strconv.FormatInt(atomic.AddInt64(&c.nextID, 1), 10)

	var rawParams json.RawMessage
	if params != nil {
		b, _ := json.Marshal(params)
		rawParams = b
	}

	req := Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  rawParams,
	}

	ch := make(chan Response, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.send(req); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case res := <-ch:
		if res.Error != nil {
			return nil, fmt.Errorf("%s", res.Error.Message)
		}
		return res.Result, nil
	}
}

func (c *Connection) Notify(method string, params interface{}) error {
	var rawParams json.RawMessage
	if params != nil {
		b, _ := json.Marshal(params)
		rawParams = b
	}
	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}
	return c.send(req)
}
