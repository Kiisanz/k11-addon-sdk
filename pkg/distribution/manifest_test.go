package distribution_test

import (
	"encoding/json"
	"testing"

	"github.com/Kiisanz/k11-addon-sdk/pkg/distribution"
)

var goldenJSON = []byte(`{
  "schemaVersion": 1,
  "id": "k11.android",
  "version": "1.1.0",
  "api": {
    "addonApi": "v1",
    "editorSchemaVersion": 1,
    "minAgentVersion": "0.1.0"
  },
  "assets": [
    {
      "os": "linux",
      "arch": "amd64",
      "url": "https://github.com/org/k11-addon-android/releases/download/v1.1.0/k11-addon-android-linux-amd64.tar.gz",
      "sha256": "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
      "size": 8421920,
      "format": "tar.gz",
      "executable": "k11-addon-android"
    },
    {
      "os": "windows",
      "arch": "amd64",
      "url": "https://github.com/org/k11-addon-android/releases/download/v1.1.0/k11-addon-android-windows-amd64.zip",
      "sha256": "1234567890123456789012345678901234567890123456789012345678901234",
      "size": 9011200,
      "format": "zip",
      "executable": "k11-addon-android.exe"
    }
  ]
}`)

func TestDistributionManifest_GoldenParse(t *testing.T) {
	var m distribution.DistributionManifest
	if err := json.Unmarshal(goldenJSON, &m); err != nil {
		t.Fatalf("Failed to parse golden JSON: %v", err)
	}

	if err := m.Validate(); err != nil {
		t.Errorf("Expected golden JSON to be valid, got: %v", err)
	}

	asset, err := m.SelectAsset("linux", "amd64")
	if err != nil {
		t.Fatalf("SelectAsset failed: %v", err)
	}
	if asset.Format != "tar.gz" || asset.Executable != "k11-addon-android" {
		t.Errorf("Unexpected asset selected: %+v", asset)
	}
}

func TestDistributionManifest_Validate(t *testing.T) {
	validAsset := distribution.AddonAsset{
		OS:           "darwin",
		Architecture: "arm64",
		URL:          "https://example.com/asset.zip",
		SHA256:       "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Size:         1000,
		Format:       "zip",
		Executable:   "bin/addon",
	}

	baseManifest := distribution.DistributionManifest{
		SchemaVersion: 1,
		ID:            "test.addon",
		Version:       "1.0.0",
		API: distribution.Compatibility{
			AddonAPI:            "v1",
			EditorSchemaVersion: 1,
		},
		Assets: []distribution.AddonAsset{validAsset},
	}

	tests := []struct {
		name    string
		mutate  func(*distribution.DistributionManifest)
		wantErr bool
	}{
		{
			name:    "valid",
			mutate:  func(m *distribution.DistributionManifest) {},
			wantErr: false,
		},
		{
			name:    "invalid schemaVersion",
			mutate:  func(m *distribution.DistributionManifest) { m.SchemaVersion = 2 },
			wantErr: true,
		},
		{
			name:    "missing id",
			mutate:  func(m *distribution.DistributionManifest) { m.ID = "" },
			wantErr: true,
		},
		{
			name:    "invalid semver (v prefix)",
			mutate:  func(m *distribution.DistributionManifest) { m.Version = "v1.0.0" },
			wantErr: true,
		},
		{
			name:    "invalid semver (bad format)",
			mutate:  func(m *distribution.DistributionManifest) { m.Version = "1.0" },
			wantErr: true,
		},
		{
			name:    "missing addonApi",
			mutate:  func(m *distribution.DistributionManifest) { m.API.AddonAPI = "" },
			wantErr: true,
		},
		{
			name:    "zero editorSchemaVersion",
			mutate:  func(m *distribution.DistributionManifest) { m.API.EditorSchemaVersion = 0 },
			wantErr: true,
		},
		{
			name:    "no assets",
			mutate:  func(m *distribution.DistributionManifest) { m.Assets = nil },
			wantErr: true,
		},
		{
			name: "duplicate os/arch",
			mutate: func(m *distribution.DistributionManifest) {
				m.Assets = append(m.Assets, validAsset)
			},
			wantErr: true,
		},
		{
			name: "http url not allowed",
			mutate: func(m *distribution.DistributionManifest) {
				m.Assets[0].URL = "http://example.com/asset.zip"
			},
			wantErr: true,
		},
		{
			name: "invalid sha256 length",
			mutate: func(m *distribution.DistributionManifest) {
				m.Assets[0].SHA256 = "abcdef"
			},
			wantErr: true,
		},
		{
			name: "invalid sha256 uppercase",
			mutate: func(m *distribution.DistributionManifest) {
				m.Assets[0].SHA256 = "ABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCDEFABCD"
			},
			wantErr: true,
		},
		{
			name: "zero size",
			mutate: func(m *distribution.DistributionManifest) {
				m.Assets[0].Size = 0
			},
			wantErr: true,
		},
		{
			name: "unsupported format",
			mutate: func(m *distribution.DistributionManifest) {
				m.Assets[0].Format = "rar"
			},
			wantErr: true,
		},
		{
			name: "absolute executable path",
			mutate: func(m *distribution.DistributionManifest) {
				m.Assets[0].Executable = "/bin/addon"
			},
			wantErr: true,
		},
		{
			name: "path traversal in executable",
			mutate: func(m *distribution.DistributionManifest) {
				m.Assets[0].Executable = "../bin/addon"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// deep copy
			m := baseManifest
			m.Assets = make([]distribution.AddonAsset, len(baseManifest.Assets))
			copy(m.Assets, baseManifest.Assets)

			tt.mutate(&m)
			err := m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDistributionManifest_SelectAsset(t *testing.T) {
	m := distribution.DistributionManifest{
		Assets: []distribution.AddonAsset{
			{OS: "linux", Architecture: "amd64"},
			{OS: "windows", Architecture: "amd64"},
		},
	}

	_, err := m.SelectAsset("linux", "amd64")
	if err != nil {
		t.Errorf("Expected to find linux/amd64 asset")
	}

	_, err = m.SelectAsset("darwin", "arm64")
	if err == nil {
		t.Errorf("Expected error for missing darwin/arm64 asset")
	}
}
