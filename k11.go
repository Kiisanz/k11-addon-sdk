package k11

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/Kiisanz/k11-addon-sdk/addonapi"
	"github.com/Kiisanz/k11-addon-sdk/server"
)

type Addon struct {
	manifest          addonapi.Manifest
	executors         map[string]addonapi.Executor
	invokers          map[string]func(context.Context, interface{}) (interface{}, error)
	capabilitiesCheck func() map[string]map[string]string
}

type AddonOption func(*addonapi.Manifest)

func Name(name string) AddonOption        { return func(m *addonapi.Manifest) { m.Name = name } }
func Version(version string) AddonOption  { return func(m *addonapi.Manifest) { m.Version = version } }
func Description(desc string) AddonOption { return func(m *addonapi.Manifest) { m.Description = desc } }
func ApiVersion(version string) AddonOption {
	return func(m *addonapi.Manifest) { m.ApiVersion = version }
}
func EditorSchemaVersion(ver int) AddonOption {
	return func(m *addonapi.Manifest) { m.EditorSchemaVersion = ver }
}

func NewAddon(id string, opts ...AddonOption) *Addon {
	m := addonapi.Manifest{
		ID:                  id,
		ApiVersion:          "v1",
		EditorSchemaVersion: 1,
		Nodes:               make([]addonapi.NodeDefinition, 0),
	}
	for _, opt := range opts {
		opt(&m)
	}
	return &Addon{
		manifest:  m,
		executors: make(map[string]addonapi.Executor),
		invokers:  make(map[string]func(context.Context, interface{}) (interface{}, error)),
	}
}

// ---------------------------------------------------------
// Invokers & Capabilities
// ---------------------------------------------------------

func (a *Addon) RegisterInvoker(resource string, handler func(ctx context.Context, params interface{}) (interface{}, error)) {
	a.invokers[resource] = handler
}

func (a *Addon) SetCapabilitiesCheck(handler func() map[string]map[string]string) {
	a.capabilitiesCheck = handler
}

func Capabilities(caps ...string) AddonOption {
	return func(m *addonapi.Manifest) { m.Capabilities = append(m.Capabilities, caps...) }
}

func (a *Addon) ServeStream(in io.Reader, out io.Writer) error {
	if err := a.manifest.Validate(); err != nil {
		return err
	}

	srv := server.NewServer(a.manifest)

	for id, executor := range a.executors {
		srv.RegisterExecutor(id, executor)
	}
	for method, invoker := range a.invokers {
		srv.RegisterInvoker(method, invoker)
	}
	srv.CapabilitiesCheck = a.capabilitiesCheck

	srv.ServeStream(in, out)
	return nil
}

func (a *Addon) Serve() {
	if err := a.ServeStream(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "addon error: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------
// Node Registration
// ---------------------------------------------------------

type Node struct {
	Name        string
	Category    string
	Description string
	Color       string
}

type RegisterOption func(*addonapi.NodeDefinition)

func Inputs(inputs ...InputBuilder) RegisterOption {
	return func(def *addonapi.NodeDefinition) {
		if def.Inputs == nil {
			def.Inputs = make(map[string]addonapi.NodeInput)
		}
		if def.Editor == nil {
			def.Editor = &addonapi.NodeEditor{}
		}
		for _, ib := range inputs {
			inputDef, prop := ib.Build()
			def.Inputs[prop.Key] = inputDef
			def.Editor.Properties = append(def.Editor.Properties, prop)
		}
	}
}

func Outputs(outputs ...OutputBuilder) RegisterOption {
	return func(def *addonapi.NodeDefinition) {
		if def.Outputs == nil {
			def.Outputs = make(map[string]addonapi.NodeOutput)
		}
		for _, ob := range outputs {
			k, v := ob.BuildOutput()
			def.Outputs[k] = v
		}
	}
}

func Register(a *Addon, id string, meta Node, handler any, opts ...RegisterOption) {
	def := addonapi.NodeDefinition{
		ID:          id,
		Name:        meta.Name,
		Category:    meta.Category,
		Description: meta.Description,
		Color:       meta.Color,
	}

	for _, opt := range opts {
		opt(&def)
	}

	executor, err := wrapHandler(handler)
	if err != nil {
		panic(fmt.Errorf("node %q registration failed: %w", id, err))
	}

	a.manifest.Nodes = append(a.manifest.Nodes, def)
	a.executors[id] = executor
}

// ---------------------------------------------------------
// Context
// ---------------------------------------------------------

type Context interface {
	context.Context
	Log(msg string)
	Progress(pct float64)
	Emit(event string, payload map[string]any)
}

type addonContext struct {
	context.Context
	emitter addonapi.EventEmitter
}

func (c *addonContext) Log(msg string) {
	c.emitter.Emit(addonapi.RunEvent{Type: "log", Message: &msg})
}
func (c *addonContext) Progress(pct float64) {
	c.emitter.Emit(addonapi.RunEvent{Type: "progress", State: map[string]any{"percentage": pct}})
}
func (c *addonContext) Emit(event string, payload map[string]any) {
	c.emitter.Emit(addonapi.RunEvent{Type: event, State: payload})
}

// ---------------------------------------------------------
// Handler Reflection
// ---------------------------------------------------------

type wrappedHandler struct {
	fn reflect.Value
}

func wrapHandler(handler any) (addonapi.Executor, error) {
	if ext, ok := handler.(addonapi.Executor); ok {
		return ext, nil
	}

	t := reflect.TypeOf(handler)
	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("handler must be a function")
	}

	if t.NumIn() != 2 {
		return nil, fmt.Errorf("handler must have exactly 2 parameters, got %d", t.NumIn())
	}

	// Param 1 must be a struct or pointer to struct
	in0 := t.In(0)
	actualIn0 := in0
	if in0.Kind() == reflect.Ptr {
		actualIn0 = in0.Elem()
	}
	if actualIn0.Kind() != reflect.Struct {
		return nil, fmt.Errorf("first parameter must be a struct or pointer to struct")
	}

	// Param 2 must implement Context
	ctxType := reflect.TypeOf((*Context)(nil)).Elem()
	if !t.In(1).Implements(ctxType) {
		return nil, fmt.Errorf("second parameter must implement k11.Context")
	}

	if t.NumOut() != 2 {
		return nil, fmt.Errorf("handler must have exactly 2 return values, got %d", t.NumOut())
	}

	// Return 2 must implement error
	errType := reflect.TypeOf((*error)(nil)).Elem()
	if !t.Out(1).Implements(errType) {
		return nil, fmt.Errorf("second return value must implement error")
	}

	return &wrappedHandler{fn: reflect.ValueOf(handler)}, nil
}

func (w *wrappedHandler) Execute(ctx context.Context, input map[string]any, emitter addonapi.EventEmitter) (outMap map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during execution: %v", r)
		}
	}()

	fnType := w.fn.Type()

	// Create input struct instance
	inType := fnType.In(0)
	isPtr := inType.Kind() == reflect.Ptr
	var inVal reflect.Value
	if isPtr {
		inVal = reflect.New(inType.Elem())
	} else {
		inVal = reflect.New(inType)
	}

	// Unmarshal input map to struct via JSON (easiest way to handle struct tags)
	b, errMarsh := json.Marshal(input)
	if errMarsh != nil {
		return nil, fmt.Errorf("failed to marshal input map: %w", errMarsh)
	}
	if errUnmarsh := json.Unmarshal(b, inVal.Interface()); errUnmarsh != nil {
		return nil, fmt.Errorf("failed to decode input: %w", errUnmarsh)
	}

	if !isPtr {
		inVal = inVal.Elem()
	}

	// Prepare context
	kCtx := &addonContext{Context: ctx, emitter: emitter}
	ctxVal := reflect.ValueOf(kCtx)

	// Call function
	results := w.fn.Call([]reflect.Value{inVal, ctxVal})

	// Check error
	if !results[1].IsNil() {
		return nil, results[1].Interface().(error)
	}

	// Marshal output struct to map
	outVal := results[0].Interface()
	bOut, errMarshOut := json.Marshal(outVal)
	if errMarshOut != nil {
		return nil, fmt.Errorf("failed to marshal output struct: %w", errMarshOut)
	}
	if errUnmarshOut := json.Unmarshal(bOut, &outMap); errUnmarshOut != nil {
		return nil, fmt.Errorf("failed to decode output: %w", errUnmarshOut)
	}

	return outMap, nil
}

func Kind(kind string) AddonOption {
	return func(m *addonapi.Manifest) { m.Kind = kind }
}
