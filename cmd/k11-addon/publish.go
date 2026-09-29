package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
)

func handlePublish() {
	token := getToken()
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	
	s.Suffix = " Reading distribution.json..."
	s.Start()
	configData, err := os.ReadFile("distribution.json")
	if err != nil {
		s.Stop()
		fmt.Println("[ERROR] distribution.json not found.")
		os.Exit(1)
	}

	var manifest DistributionManifest
	json.Unmarshal(configData, &manifest)

	if manifest.Repository == "" {
		s.Stop()
		fmt.Println("[ERROR] 'repository' field is missing in distribution.json.")
		os.Exit(1)
	}

	client := &http.Client{}

	s.Suffix = fmt.Sprintf(" Verifying repository %s on GitHub...", manifest.Repository)
	repoReq, _ := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s", manifest.Repository), nil)
	repoReq.Header.Set("Authorization", "Bearer "+token)
	repoReq.Header.Set("Accept", "application/vnd.github+json")
	repoReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	repoResp, err := client.Do(repoReq)
	if err != nil {
		s.Stop()
		fmt.Printf("[ERROR] Network error while verifying repository: %v\n", err)
		os.Exit(1)
	}
	defer repoResp.Body.Close()

	if repoResp.StatusCode == 404 {
		s.Stop()
		fmt.Printf("[ERROR] Repository '%s' not found on GitHub.\n", manifest.Repository)
		fmt.Println("[INFO] Action Required: Please create the repository on GitHub and push your initial commit before publishing.")
		os.Exit(1)
	} else if repoResp.StatusCode == 401 || repoResp.StatusCode == 403 {
		s.Stop()
		fmt.Printf("[ERROR] Unauthorized access to repository '%s'. Please verify your permissions or login again.\n", manifest.Repository)
		os.Exit(1)
	} else if repoResp.StatusCode != 200 {
		s.Stop()
		fmt.Printf("[ERROR] Unexpected response from GitHub API (HTTP %d).\n", repoResp.StatusCode)
		os.Exit(1)
	}

	tagName := "v" + manifest.Version
	s.Suffix = fmt.Sprintf(" Creating GitHub Release %s for %s...", tagName, manifest.Repository)

	releaseReqBody := map[string]interface{}{
		"tag_name": tagName,
		"name": "Release " + tagName,
		"body": fmt.Sprintf("K11 Addon Release for %s\nCategory: %s", manifest.ID, manifest.Category),
		"draft": false,
		"prerelease": false,
	}
	bodyBytes, _ := json.Marshal(releaseReqBody)

	req, _ := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/repos/%s/releases", manifest.Repository), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		s.Stop()
		fmt.Printf("[ERROR] Failed to create release: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 422 {
		s.Stop()
		fmt.Println("[ERROR] Release or tag already exists. Please update the version in distribution.json or delete the existing tag on GitHub.")
		os.Exit(1)
	} else if resp.StatusCode != 201 {
		s.Stop()
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("[ERROR] Error creating release (HTTP %d): %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	var releaseResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&releaseResp)
	uploadUrlRaw := releaseResp["upload_url"].(string)
	uploadUrlBase := strings.Split(uploadUrlRaw, "{")[0]

	assetsToUpload := []string{"distribution.json"}
	for _, a := range manifest.Assets {
		filename := filepath.Base(a.URL)
		assetsToUpload = append(assetsToUpload, filepath.Join("dist", filename))
	}

	for _, assetPath := range assetsToUpload {
		assetName := filepath.Base(assetPath)
		s.Suffix = fmt.Sprintf(" Uploading %s...", assetName)
		
		fileData, err := os.ReadFile(assetPath)
		if err != nil {
			s.Stop()
			fmt.Printf("[WARN] Asset %s not found. Did you run 'k11-addon build' first?\n", assetPath)
			s.Start()
			continue
		}

		uploadUrl := fmt.Sprintf("%s?name=%s", uploadUrlBase, assetName)
		upReq, _ := http.NewRequest("POST", uploadUrl, bytes.NewBuffer(fileData))
		upReq.Header.Set("Authorization", "Bearer "+token)
		upReq.Header.Set("Content-Type", "application/octet-stream")
		upReq.Header.Set("Accept", "application/vnd.github+json")
		upReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		upResp, err := client.Do(upReq)
		if err != nil || upResp.StatusCode >= 300 {
			s.Stop()
			b, _ := io.ReadAll(upResp.Body)
			fmt.Printf("[ERROR] Failed to upload %s: %s\n", assetName, string(b))
			s.Start()
		}
		if upResp != nil { upResp.Body.Close() }
	}

	s.Stop()
	fmt.Printf("[SUCCESS] Published %s %s to GitHub repository %s.\n", manifest.ID, tagName, manifest.Repository)
	
	s.Suffix = " Submitting to central registry..."
	s.Start()
	submitToRegistry(token, manifest)
	s.Stop()
}
