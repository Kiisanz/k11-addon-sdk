package distribution

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// Strict SemVer without 'v' prefix
	semverRegex = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+)?$`)
	sha256Regex = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Compatibility struct {
	AddonAPI            string `json:"addonApi"`
	EditorSchemaVersion int    `json:"editorSchemaVersion"`
	MinAgentVersion     string `json:"minAgentVersion,omitempty"`
}

type AddonAsset struct {
	OS           string `json:"os"`
	Architecture string `json:"arch"`
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
	Format       string `json:"format"`     // raw, tar.gz, zip
	Executable   string `json:"executable"` // path setelah ekstraksi
}

type DistributionManifest struct {
	SchemaVersion int           `json:"schemaVersion"`
	ID            string        `json:"id"`
	Version       string        `json:"version"`
	API           Compatibility `json:"api"`
	Assets        []AddonAsset  `json:"assets"`
}

type CurrentState struct {
	Version     string    `json:"version"`
	ActivatedAt time.Time `json:"activatedAt"`
}

// Validate checks all constraints defined for the V1 Distribution Manifest.
func (m *DistributionManifest) Validate() error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schemaVersion: %d, expected 1", m.SchemaVersion)
	}
	if m.ID == "" {
		return fmt.Errorf("id is required")
	}
	if !semverRegex.MatchString(m.Version) {
		return fmt.Errorf("version must be valid strict SemVer without 'v' prefix, got %q", m.Version)
	}
	if m.API.AddonAPI == "" {
		return fmt.Errorf("api.addonApi is required")
	}
	if m.API.EditorSchemaVersion <= 0 {
		return fmt.Errorf("api.editorSchemaVersion is required and must be > 0")
	}
	if len(m.Assets) == 0 {
		return fmt.Errorf("at least one asset is required")
	}

	seenTargets := make(map[string]bool)
	for i, asset := range m.Assets {
		if asset.OS == "" || asset.Architecture == "" {
			return fmt.Errorf("asset[%d] missing os or arch", i)
		}
		target := asset.OS + "/" + asset.Architecture
		if seenTargets[target] {
			return fmt.Errorf("asset[%d] duplicate target os+arch: %s", i, target)
		}
		seenTargets[target] = true

		if !strings.HasPrefix(asset.URL, "https://") && !strings.HasPrefix(asset.URL, "file://") {
			return fmt.Errorf("asset[%d] url must use https:// or file://", i)
		}
		if !sha256Regex.MatchString(asset.SHA256) {
			return fmt.Errorf("asset[%d] sha256 must be exactly 64 lowercase hex characters", i)
		}
		if asset.Size <= 0 {
			return fmt.Errorf("asset[%d] size must be greater than 0", i)
		}
		if asset.Format != "raw" && asset.Format != "tar.gz" && asset.Format != "zip" {
			return fmt.Errorf("asset[%d] unsupported format: %q", i, asset.Format)
		}
		if asset.Executable == "" {
			return fmt.Errorf("asset[%d] executable path is required", i)
		}

		// Clean and validate executable path
		if filepath.IsAbs(asset.Executable) || path.IsAbs(asset.Executable) {
			return fmt.Errorf("asset[%d] executable must be a relative path", i)
		}
		cleaned := path.Clean(asset.Executable)
		if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
			return fmt.Errorf("asset[%d] executable path cannot traverse outside directory", i)
		}
	}

	return nil
}

// SelectAsset returns the appropriate AddonAsset for the given OS and Architecture.
func (m *DistributionManifest) SelectAsset(goos, goarch string) (*AddonAsset, error) {
	for _, asset := range m.Assets {
		if asset.OS == goos && asset.Architecture == goarch {
			return &asset, nil
		}
	}
	return nil, fmt.Errorf("no suitable asset found for %s/%s", goos, goarch)
}
