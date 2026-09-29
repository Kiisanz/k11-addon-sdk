package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const registryRepo = "Kiisanz/k11-addon-registry"

func submitToRegistry(token string, manifest DistributionManifest) {
	fmt.Printf("[INFO] Submitting %s to central registry (%s)...\n", manifest.ID, registryRepo)
	client := &http.Client{}

	// 1. Get default branch SHA
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/git/refs/heads/main", registryRepo), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		fmt.Printf("[WARN] Registry repo not found or accessible. Skipping registry submission.\n")
		return
	}
	defer resp.Body.Close()
	
	var refData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&refData)
	obj := refData["object"].(map[string]interface{})
	mainSha := obj["sha"].(string)

	// 2. Create new branch
	branchName := fmt.Sprintf("update-%s-v%s", manifest.ID, manifest.Version)
	branchReqBody := map[string]string{
		"ref": "refs/heads/" + branchName,
		"sha": mainSha,
	}
	bBytes, _ := json.Marshal(branchReqBody)
	req2, _ := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/repos/%s/git/refs", registryRepo), bytes.NewBuffer(bBytes))
	req2.Header = req.Header
	
	resp2, _ := client.Do(req2)
	if resp2.StatusCode == 422 {
		fmt.Printf("[INFO] Branch %s already exists. Continuing...\n", branchName)
	} else if resp2.StatusCode != 201 {
		fmt.Printf("[WARN] Failed to create branch: HTTP %d\n", resp2.StatusCode)
		return
	}
	if resp2 != nil { resp2.Body.Close() }

	// 3. Get existing file SHA (if updating)
	filePath := fmt.Sprintf("addons/%s.json", manifest.ID)
	req3, _ := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", registryRepo, filePath), nil)
	req3.Header = req.Header
	resp3, _ := client.Do(req3)
	fileSha := ""
	if resp3.StatusCode == 200 {
		var fileData map[string]interface{}
		json.NewDecoder(resp3.Body).Decode(&fileData)
		fileSha = fileData["sha"].(string)
	}
	if resp3 != nil { resp3.Body.Close() }

	// 4. Create/Update file
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	encodedContent := base64.StdEncoding.EncodeToString(manifestBytes)

	putReqBody := map[string]string{
		"message": fmt.Sprintf("Update addon %s to version %s", manifest.ID, manifest.Version),
		"content": encodedContent,
		"branch":  branchName,
	}
	if fileSha != "" {
		putReqBody["sha"] = fileSha
	}
	pBytes, _ := json.Marshal(putReqBody)

	req4, _ := http.NewRequest("PUT", fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", registryRepo, filePath), bytes.NewBuffer(pBytes))
	req4.Header = req.Header
	resp4, _ := client.Do(req4)
	if resp4.StatusCode >= 300 {
		b, _ := io.ReadAll(resp4.Body)
		fmt.Printf("[WARN] Failed to upload to registry: %s\n", string(b))
		return
	}
	if resp4 != nil { resp4.Body.Close() }

	// 5. Create Pull Request
	prReqBody := map[string]string{
		"title": fmt.Sprintf("Publish %s v%s", manifest.ID, manifest.Version),
		"head":  branchName,
		"base":  "main",
		"body":  fmt.Sprintf("Automated PR to publish `%s` version `%s`.", manifest.ID, manifest.Version),
	}
	prBytes, _ := json.Marshal(prReqBody)
	req5, _ := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/repos/%s/pulls", registryRepo), bytes.NewBuffer(prBytes))
	req5.Header = req.Header
	resp5, _ := client.Do(req5)
	
	if resp5.StatusCode == 201 {
		var prResp map[string]interface{}
		json.NewDecoder(resp5.Body).Decode(&prResp)
		fmt.Printf("[SUCCESS] Pull Request created in registry: %s\n", prResp["html_url"])
		
		pullNumber := int(prResp["number"].(float64))
		attemptAutoMerge(token, registryRepo, pullNumber)
	} else if resp5.StatusCode == 422 {
		fmt.Printf("[INFO] Pull Request already exists for this branch.\n")
	} else {
		b, _ := io.ReadAll(resp5.Body)
		fmt.Printf("[WARN] Failed to create PR: HTTP %d - %s\n", resp5.StatusCode, string(b))
	}
	if resp5 != nil { resp5.Body.Close() }
}

func attemptAutoMerge(token, repo string, pullNumber int) {
	fmt.Printf("[INFO] Attempting auto-merge for PR #%d...\n", pullNumber)
	
	reqBody := map[string]string{
		"commit_title":   fmt.Sprintf("Auto-merge PR #%d", pullNumber),
		"commit_message": "Automated merge by K11 CLI",
		"merge_method":   "squash",
	}
	bBytes, _ := json.Marshal(reqBody)
	
	req, _ := http.NewRequest("PUT", fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d/merge", repo, pullNumber), bytes.NewBuffer(bBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[WARN] Auto-merge failed (network error): %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("[SUCCESS] Pull Request auto-merged successfully!")
	} else if resp.StatusCode == 403 || resp.StatusCode == 404 {
		fmt.Println("[INFO] Auto-merge skipped: You do not have write access to the registry. The PR is pending review.")
	} else {
		b, _ := io.ReadAll(resp.Body)
		fmt.Printf("[WARN] Auto-merge failed (HTTP %d): %s\n", resp.StatusCode, string(b))
	}
}
