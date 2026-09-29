package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
)

type BuildConfig struct {
	Entrypoint string `json:"entrypoint,omitempty"`
}

type AddonAsset struct {
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Format     string `json:"format,omitempty"`
	Executable string `json:"executable,omitempty"`
}

type DistributionManifest struct {
	SchemaVersion int                    `json:"schemaVersion"`
	ID            string                 `json:"id"`
	Version       string                 `json:"version"`
	Category      string                 `json:"category,omitempty"`
	Repository    string                 `json:"repository,omitempty"`
	BuildConfig   *BuildConfig           `json:"buildConfig,omitempty"`
	API           map[string]interface{} `json:"api"`
	Assets        []AddonAsset           `json:"assets"`
}

func handleBuild() {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)

	s.Suffix = " Reading distribution.json..."
	s.Start()
	configData, err := os.ReadFile("distribution.json")
	if err != nil {
		s.Stop()
		fmt.Println("[ERROR] distribution.json not found. Run this in your addon root directory.")
		os.Exit(1)
	}

	var manifest DistributionManifest
	if err := json.Unmarshal(configData, &manifest); err != nil {
		s.Stop()
		fmt.Printf("[ERROR] Error parsing distribution.json: %v\n", err)
		os.Exit(1)
	}

	entrypoint := "."
	if manifest.BuildConfig != nil && manifest.BuildConfig.Entrypoint != "" {
		entrypoint = manifest.BuildConfig.Entrypoint
	}

	os.MkdirAll("dist", 0755)

	targetOS := "linux"
	targetArch := "amd64"
	binName := fmt.Sprintf("%s-%s-%s", manifest.ID, targetOS, targetArch)
	binPath := filepath.Join("dist", binName)

	s.Suffix = fmt.Sprintf(" Compiling %s for %s/%s...", entrypoint, targetOS, targetArch)
	cmd := exec.Command("go", "build", "-o", binPath, entrypoint)
	cmd.Env = append(os.Environ(), "GOOS="+targetOS, "GOARCH="+targetArch, "CGO_ENABLED=0")
	if err := cmd.Run(); err != nil {
		s.Stop()
		fmt.Printf("[ERROR] Build failed: %v\n", err)
		os.Exit(1)
	}

	tarName := binName + ".tar.gz"
	tarPath := filepath.Join("dist", tarName)
	s.Suffix = fmt.Sprintf(" Packaging %s...", tarName)
	
	if err := createTarGz(tarPath, binPath, binName); err != nil {
		s.Stop()
		fmt.Printf("[ERROR] Packaging failed: %v\n", err)
		os.Exit(1)
	}

	s.Suffix = " Calculating SHA-256..."
	hash, size := getFileHashAndSize(tarPath)

	s.Suffix = " Updating distribution.json with new asset metadata..."
	
	assetFound := false
	for i, a := range manifest.Assets {
		if a.OS == targetOS && a.Arch == targetArch {
			manifest.Assets[i].SHA256 = hash
			manifest.Assets[i].Size = size
			manifest.Assets[i].URL = fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", manifest.Repository, manifest.Version, tarName)
			manifest.Assets[i].Format = "tar.gz"
			manifest.Assets[i].Executable = binName
			assetFound = true
			break
		}
	}

	if !assetFound {
		manifest.Assets = append(manifest.Assets, AddonAsset{
			OS:         targetOS,
			Arch:       targetArch,
			URL:        fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", manifest.Repository, manifest.Version, tarName),
			SHA256:     hash,
			Size:       size,
			Format:     "tar.gz",
			Executable: binName,
		})
	}

	distBytes, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile("distribution.json", distBytes, 0644)
	
	s.Stop()
	fmt.Println("[SUCCESS] Build complete. 'distribution.json' has been updated automatically.")
}

func createTarGz(tarPath, srcFile, internalName string) error {
	out, err := os.Create(tarPath)
	if err != nil { return err }
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	file, err := os.Open(srcFile)
	if err != nil { return err }
	defer file.Close()

	stat, _ := file.Stat()
	hdr := &tar.Header{
		Name: internalName,
		Mode: 0755,
		Size: stat.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil { return err }
	if _, err := io.Copy(tw, file); err != nil { return err }

	return nil
}

func getFileHashAndSize(path string) (string, int64) {
	f, _ := os.Open(path)
	defer f.Close()
	stat, _ := f.Stat()
	
	h := sha256.New()
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), stat.Size()
}
