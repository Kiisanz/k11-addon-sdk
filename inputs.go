package k11

import "github.com/Kiisanz/k11-addon-sdk/pkg/addonapi"

type InputBuilder interface {
	Build() (addonapi.NodeInput, addonapi.EditorProperty)
}

type PropOption func(*addonapi.NodeInput, *addonapi.EditorProperty)

func Label(label string) PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		prop.Label = label
	}
}

func Required() PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		in.Required = true
	}
}

func Default(val any) PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		in.Default = val
	}
}

func Options(opts ...string) PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		for _, o := range opts {
			prop.Options = append(prop.Options, addonapi.SelectOption{Label: o, Value: o})
		}
	}
}

func ShapeAny() PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		if prop.Props == nil {
			prop.Props = make(map[string]any)
		}
		prop.Props["shape"] = "any"
	}
}

func ShapeObject() PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		if prop.Props == nil {
			prop.Props = make(map[string]any)
		}
		prop.Props["shape"] = "object"
	}
}

func DependsOn(deps ...string) PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		prop.DependsOn = append(prop.DependsOn, deps...)
	}
}

func Resource(resource string) PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		prop.Resource = resource
	}
}

type basicInput struct {
	key     string
	inType  string
	control addonapi.EditorControl
	opts    []PropOption
}

func (b *basicInput) Build() (addonapi.NodeInput, addonapi.EditorProperty) {
	in := addonapi.NodeInput{Type: b.inType}
	prop := addonapi.EditorProperty{
		Key:     b.key,
		Control: b.control,
	}
	for _, opt := range b.opts {
		opt(&in, &prop)
	}
	return in, prop
}

func Text(key string, opts ...PropOption) InputBuilder {
	return &basicInput{key: key, inType: "string", control: addonapi.ControlText, opts: opts}
}

func Select(key string, opts ...PropOption) InputBuilder {
	return &basicInput{key: key, inType: "string", control: addonapi.ControlSelect, opts: opts}
}

func JSON(key string, opts ...PropOption) InputBuilder {
	return &basicInput{key: key, inType: "any", control: addonapi.ControlJson, opts: opts}
}

func ResourceSelect(key string, opts ...PropOption) InputBuilder {
	return &basicInput{key: key, inType: "string", control: addonapi.ControlResourceSelect, opts: opts}
}

type OutputBuilder interface {
	BuildOutput() (string, addonapi.NodeOutput)
}

func ShapeArray() PropOption {
	return func(in *addonapi.NodeInput, prop *addonapi.EditorProperty) {
		if prop.Props == nil {
			prop.Props = make(map[string]any)
		}
		prop.Props["shape"] = "array"
	}
}

func Number(key string, opts ...PropOption) InputBuilder {
	return &basicInput{key: key, inType: "number", control: addonapi.ControlNumber, opts: opts}
}

func Textarea(key string, opts ...PropOption) InputBuilder {
	return &basicInput{key: key, inType: "string", control: addonapi.ControlTextarea, opts: opts}
}

type basicOutput struct {
	key string
	typ string
}

func (b *basicOutput) BuildOutput() (string, addonapi.NodeOutput) {
	return b.key, addonapi.NodeOutput{Type: b.typ}
}

func RecordOutput(key string) OutputBuilder { return &basicOutput{key: key, typ: "record"} }
func NumberOut(key string) OutputBuilder    { return &basicOutput{key: key, typ: "number"} }
func StringOut(key string) OutputBuilder    { return &basicOutput{key: key, typ: "string"} }
func AnyOut(key string) OutputBuilder       { return &basicOutput{key: key, typ: "any"} }

func ElementPicker(key string, opts ...PropOption) InputBuilder {
	return &basicInput{key: key, inType: "AndroidElement", control: addonapi.ControlElementPicker, opts: opts}
}

func Enum(key string, opts ...PropOption) InputBuilder {
	return &basicInput{key: key, inType: "enum", control: addonapi.ControlSelect, opts: opts}
}

func AndroidElementOut(key string) OutputBuilder {
	return &basicOutput{key: key, typ: "AndroidElement"}
}
func AndroidElementArrayOut(key string) OutputBuilder {
	return &basicOutput{key: key, typ: "AndroidElement[]"}
}
