package addonapi

import (
	"context"
	"fmt"
)

type Manifest struct {
	ApiVersion          string           `json:"apiVersion"`
	EditorSchemaVersion int              `json:"editorSchemaVersion,omitempty"`
	ID                  string           `json:"id"`
	Kind                string           `json:"kind,omitempty"`
	Name                string           `json:"name"`
	Version             string           `json:"version"`
	Description         string           `json:"description,omitempty"`
	Icon                *IconDefinition  `json:"icon,omitempty"`
	Requires            Requirements     `json:"requires"`
	Permissions         []string         `json:"permissions"`
	Capabilities        []string         `json:"capabilities"`
	Nodes               []NodeDefinition `json:"nodes"`
}

type Requirements struct {
	Executables []ExecutableRequirement `json:"executables"`
}
type ExecutableRequirement struct {
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Artifacts []Artifact `json:"artifacts"`
}
type Artifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}
type IconDefinition struct {
	Type string `json:"type"`
	Src  string `json:"src"`
}

type NodeDefinition struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	Category       string                `json:"category"`
	Icon           *IconDefinition       `json:"icon,omitempty"`
	Color          string                `json:"color,omitempty"`
	Description    string                `json:"description,omitempty"`
	Inputs         map[string]NodeInput  `json:"inputs"`
	Outputs        map[string]NodeOutput `json:"outputs,omitempty"`
	ControlOutputs []ControlOutput       `json:"controlOutputs,omitempty"`
	Editor         *NodeEditor           `json:"editor,omitempty"`
}

type NodeInput struct {
	Type     string      `json:"type"`
	Required bool        `json:"required"`
	Default  interface{} `json:"default,omitempty"`
}

type NodeEditor struct {
	Properties []EditorProperty `json:"properties"`
}

type EditorProperty struct {
	Key         string                 `json:"key"`
	Label       string                 `json:"label"`
	Control     EditorControl          `json:"control"`
	Resource    string                 `json:"resource,omitempty"`
	DependsOn   []string               `json:"dependsOn,omitempty"`
	Options     []SelectOption         `json:"options,omitempty"`
	Placeholder string                 `json:"placeholder,omitempty"`
	Description string                 `json:"description,omitempty"`
	Props       map[string]interface{} `json:"props,omitempty"`
}

type EditorControl string

const (
	ControlText           EditorControl = "text"
	ControlTextarea       EditorControl = "textarea"
	ControlNumber         EditorControl = "number"
	ControlBoolean        EditorControl = "boolean"
	ControlSelect         EditorControl = "select"
	ControlResourceSelect EditorControl = "resource-select"
	ControlElementPicker  EditorControl = "element-picker"
	ControlJson           EditorControl = "json"
)

type SelectOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type NodeOutput struct {
	Type string `json:"type"`
}
type ControlOutput struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Executor interface {
	Execute(context.Context, map[string]interface{}, EventEmitter) (map[string]interface{}, error)
}
type EventEmitter interface{ Emit(RunEvent) }

type RunEvent struct {
	Type           string                 `json:"type"`
	RunID          string                 `json:"runId"`
	StepID         string                 `json:"stepId,omitempty"`
	DurationMs     *int64                 `json:"durationMs,omitempty"`
	Error          *string                `json:"error,omitempty"`
	ErrorCode      string                 `json:"errorCode,omitempty"`
	Attempt        int                    `json:"attempt,omitempty"`
	Message        *string                `json:"message,omitempty"`
	Level          *string                `json:"level,omitempty"`
	State          map[string]interface{} `json:"state,omitempty"`
	Inputs         map[string]interface{} `json:"inputs,omitempty"`
	Outputs        map[string]interface{} `json:"outputs,omitempty"`
	ControlOutputs []string               `json:"controlOutputs,omitempty"`
	Timestamp      int64                  `json:"timestamp"`
	NodeID         *string                `json:"nodeId,omitempty"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
	Data           map[string]interface{} `json:"data,omitempty"`
	DatasetID      string                 `json:"datasetId,omitempty"`
}

func (m *Manifest) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("manifest ID is required")
	}
	if m.EditorSchemaVersion != 0 && m.EditorSchemaVersion != 1 {
		return fmt.Errorf("unsupported EditorSchemaVersion: %d", m.EditorSchemaVersion)
	}

	for _, node := range m.Nodes {
		if node.ID == "" {
			return fmt.Errorf("node ID is required")
		}
		if node.Editor != nil {
			for _, prop := range node.Editor.Properties {
				if _, ok := node.Inputs[prop.Key]; !ok {
					return fmt.Errorf("editor property key %q does not match any input in node %q", prop.Key, node.ID)
				}

				for _, dep := range prop.DependsOn {
					if _, ok := node.Inputs[dep]; !ok {
						return fmt.Errorf("dependsOn %q references unknown input in node %q", dep, node.ID)
					}
				}

				switch prop.Control {
				case ControlText, ControlTextarea, ControlNumber, ControlBoolean, ControlElementPicker:
					// OK
				case ControlJson:
					shape, ok := prop.Props["shape"].(string)
					if !ok || (shape != "object" && shape != "array" && shape != "any") {
						return fmt.Errorf("json control for %q requires shape object, array, or any", prop.Key)
					}
					// OK
				case ControlSelect:
					if len(prop.Options) == 0 {
						return fmt.Errorf("select control for %q requires options", prop.Key)
					}
				case ControlResourceSelect:
					if prop.Resource == "" {
						return fmt.Errorf("resource-select control for %q requires a resource identifier", prop.Key)
					}
				default:
					return fmt.Errorf("unknown control type %q for property %q", prop.Control, prop.Key)
				}
			}
		}
	}
	return nil
}
