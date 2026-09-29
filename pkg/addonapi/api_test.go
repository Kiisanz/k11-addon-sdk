package addonapi_test

import (
	"encoding/json"
	"testing"

	"github.com/Kiisanz/k11-addon-sdk/pkg/addonapi"
)

func TestManifestEditorProperties(t *testing.T) {
	manifest := addonapi.Manifest{
		ApiVersion:          "v1",
		EditorSchemaVersion: 1,
		ID:                  "test.addon",
		Name:                "Test Addon",
		Version:             "1.0.0",
		Nodes: []addonapi.NodeDefinition{
			{
				ID:   "test.node",
				Name: "Test Node",
				Inputs: map[string]addonapi.NodeInput{
					"deviceId":    {Type: "string", Required: true},
					"packageName": {Type: "string", Required: true},
				},
				Editor: &addonapi.NodeEditor{
					Properties: []addonapi.EditorProperty{
						{
							Key:      "deviceId",
							Label:    "Device",
							Control:  "resource-select",
							Resource: "android.devices",
						},
						{
							Key:       "packageName",
							Label:     "Application",
							Control:   "resource-select",
							Resource:  "android.packages",
							DependsOn: []string{"deviceId"},
						},
					},
				},
			},
		},
	}

	b, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Failed to unmarshal manifest: %v", err)
	}

	if m["editorSchemaVersion"].(float64) != 1 {
		t.Errorf("Expected editorSchemaVersion 1, got %v", m["editorSchemaVersion"])
	}

	nodes := m["nodes"].([]interface{})
	node := nodes[0].(map[string]interface{})
	editor := node["editor"].(map[string]interface{})
	properties := editor["properties"].([]interface{})

	if len(properties) != 2 {
		t.Fatalf("Expected 2 properties, got %d", len(properties))
	}

	prop1 := properties[0].(map[string]interface{})
	if prop1["key"] != "deviceId" || prop1["control"] != "resource-select" || prop1["resource"] != "android.devices" {
		t.Errorf("Property 1 does not match expected values")
	}

	prop2 := properties[1].(map[string]interface{})
	dependsOn := prop2["dependsOn"].([]interface{})
	if dependsOn[0].(string) != "deviceId" {
		t.Errorf("Expected dependsOn to contain 'deviceId'")
	}
}

func TestManifestValidate_JsonShape(t *testing.T) {
	tests := []struct {
		name    string
		props   map[string]interface{}
		wantErr bool
	}{
		{"valid object", map[string]interface{}{"shape": "object"}, false},
		{"valid array", map[string]interface{}{"shape": "array"}, false},
		{"valid any", map[string]interface{}{"shape": "any"}, false},
		{"missing shape", nil, true},
		{"empty shape", map[string]interface{}{"shape": ""}, true},
		{"invalid string shape", map[string]interface{}{"shape": "invalid"}, true},
		{"non-string shape", map[string]interface{}{"shape": 123}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := addonapi.Manifest{
				ApiVersion:          "v1",
				EditorSchemaVersion: 1,
				ID:                  "test.addon",
				Name:                "Test Addon",
				Version:             "1.0.0",
				Nodes: []addonapi.NodeDefinition{
					{
						ID:   "test.node",
						Name: "Test Node",
						Inputs: map[string]addonapi.NodeInput{
							"data": {Type: "string", Required: true},
						},
						Editor: &addonapi.NodeEditor{
							Properties: []addonapi.EditorProperty{
								{
									Key:     "data",
									Label:   "Data",
									Control: addonapi.ControlJson,
									Props:   tt.props,
								},
							},
						},
					},
				},
			}
			err := m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
