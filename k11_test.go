package k11_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	k11 "github.com/Kiisanz/k11-addon-sdk"
	"github.com/Kiisanz/k11-addon-sdk/ipc"
)

type TestInput struct {
	Data string `json:"data"`
}

type TestOutput struct {
	Result string `json:"result"`
}

func TestAddonHighLevel(t *testing.T) {
	addon := k11.NewAddon("test.addon", k11.Name("Test"), k11.Version("1.0.0"))

	// Test 1-2: NewAddon & Register typed handler
	k11.Register(addon, "test.node",
		k11.Node{Name: "Test Node"},
		func(in *TestInput, ctx k11.Context) (*TestOutput, error) {
			ctx.Log("Processing " + in.Data)
			ctx.Progress(0.5)
			return &TestOutput{Result: "Done " + in.Data}, nil
		},
		k11.Inputs(k11.Text("data", k11.Required())),
	)

	// Test 8: Register Invoker and Capability Check
	addon.RegisterInvoker("test.resource", func(ctx context.Context, params interface{}) (interface{}, error) {
		return "invoked", nil
	})
	addon.SetCapabilitiesCheck(func() map[string]map[string]string {
		return map[string]map[string]string{"foo": {"status": "ok"}}
	})

	// Setup Pipe for communication
	clientIn, serverOut := io.Pipe()
	serverIn, clientOut := io.Pipe()

	// Test 3: Run ServeStream via pipe
	go func() {
		_ = addon.ServeStream(serverIn, serverOut)
	}()

	client := ipc.NewConnection(clientIn, clientOut)
	go client.Serve()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test 4: Call addon.manifest
	manifestRes, err := client.Call(context.Background(), "addon.manifest", nil)
	if err != nil {
		t.Fatalf("addon.manifest failed: %v", err)
	}
	var mMap map[string]interface{}
	json.Unmarshal(manifestRes, &mMap)
	if mMap["id"] != "test.addon" {
		t.Errorf("expected id 'test.addon', got %v", mMap["id"])
	}

	// Test 5 & 6: Call addon.execute and assert input/output and events
	eventCh := make(chan map[string]interface{}, 10)
	client.Notifiers["workflow.emitEvent"] = func(params json.RawMessage) {
		var ev map[string]interface{}
		json.Unmarshal(params, &ev)
		eventMap := ev["event"].(map[string]interface{})
		eventCh <- eventMap
	}

	execParams := map[string]interface{}{
		"kind":  "test.node",
		"runId": "run-123",
		"input": map[string]interface{}{"data": "Hello"},
	}

	execRes, err := client.Call(context.Background(), "addon.execute", execParams)
	if err != nil {
		t.Fatalf("addon.execute failed: %v", err)
	}

	var execMap map[string]interface{}
	json.Unmarshal(execRes, &execMap)
	if execMap["result"] != "Done Hello" {
		t.Errorf("expected 'Done Hello', got %v", execMap["result"])
	}

	// Check events (Log & Progress)
	time.Sleep(100 * time.Millisecond)
	close(eventCh)
	hasLog, hasProgress := false, false
	for e := range eventCh {
		if e["type"] == "log" {
			hasLog = true
		}
		if e["type"] == "progress" {
			hasProgress = true
		}
	}
	if !hasLog || !hasProgress {
		t.Errorf("missing events: log=%v progress=%v", hasLog, hasProgress)
	}

	// Test Invoker
	invRes, err := client.Call(context.Background(), "addon.invoke", map[string]interface{}{"method": "test.resource"})
	if err != nil {
		t.Fatalf("addon.invoke failed: %v", err)
	}
	var invStr string
	json.Unmarshal(invRes, &invStr)
	if invStr != "invoked" {
		t.Errorf("expected 'invoked', got %v", invRes)
	}

	// Clean up
	clientOut.Close()
	serverOut.Close()
}

func TestInvalidHandlerSignature(t *testing.T) {
	// Test 9: Invalid signature does not panic at runtime, instead it panics at registration!
	// Wait, the requirement: "Uji handler signature invalid tidak menyebabkan panic saat runtime."
	// Because it is caught at registration!
	addon := k11.NewAddon("test.addon")

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on invalid handler signature")
		}
	}()

	k11.Register(addon, "test.invalid", k11.Node{Name: "Invalid"}, func(in string) string { return "wrong" })
}
