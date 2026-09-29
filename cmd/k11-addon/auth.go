package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
)

const (
	clientID     = "Ov23liiVEhrc8Ak4Z1g2"
	clientSecret = "7211ab207098dc00c928014840f222a1261badd9"
)

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

func handleLogin() {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Initiating GitHub OAuth Device Flow..."
	s.Start()

	reqBody := []byte(fmt.Sprintf(`{"client_id":"%s","scope":"repo"}`, clientID))
	req, _ := http.NewRequest("POST", "https://github.com/login/device/code", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	s.Stop()
	if err != nil {
		fmt.Printf("[ERROR] Failed to request device code: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var deviceResp DeviceCodeResponse
	json.NewDecoder(resp.Body).Decode(&deviceResp)

	fmt.Printf("\n======================================================\n")
	fmt.Printf("1. Please open your browser to: %s\n", deviceResp.VerificationURI)
	fmt.Printf("2. Enter the following code:    %s\n", deviceResp.UserCode)
	fmt.Printf("======================================================\n\n")

	s.Suffix = " Waiting for authorization in browser..."
	s.Start()

	interval := time.Duration(deviceResp.Interval) * time.Second
	if interval == 0 {
		interval = 5 * time.Second
	}

	for {
		time.Sleep(interval)

		tokenReqBody := []byte(fmt.Sprintf(
			`{"client_id":"%s", "client_secret":"%s", "device_code":"%s", "grant_type":"urn:ietf:params:oauth:grant-type:device_code"}`,
			clientID, clientSecret, deviceResp.DeviceCode,
		))

		tokenReq, _ := http.NewRequest("POST", "https://github.com/login/oauth/access_token", bytes.NewBuffer(tokenReqBody))
		tokenReq.Header.Set("Content-Type", "application/json")
		tokenReq.Header.Set("Accept", "application/json")

		tokenResp, err := client.Do(tokenReq)
		if err != nil {
			continue
		}

		bodyBytes, _ := io.ReadAll(tokenResp.Body)
		tokenResp.Body.Close()

		var tr TokenResponse
		json.Unmarshal(bodyBytes, &tr)

		if tr.AccessToken != "" {
			s.Stop()
			saveToken(tr.AccessToken)
			fmt.Println("[SUCCESS] Successfully authenticated with GitHub.")
			return
		}

		if tr.Error != "authorization_pending" {
			s.Stop()
			fmt.Printf("\n[ERROR] Authentication failed or expired: %s\n", tr.Error)
			os.Exit(1)
		}
	}
}

func saveToken(token string) {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "k11")
	os.MkdirAll(configDir, 0755)

	authPath := filepath.Join(configDir, "auth.json")
	data := fmt.Sprintf(`{"github_token": "%s"}`, token)
	os.WriteFile(authPath, []byte(data), 0600)
}

func getToken() string {
	home, _ := os.UserHomeDir()
	authPath := filepath.Join(home, ".config", "k11", "auth.json")
	
	data, err := os.ReadFile(authPath)
	if err != nil {
		fmt.Println("[ERROR] You are not logged in. Please run 'k11-addon login' first.")
		os.Exit(1)
	}

	var auth struct {
		GitHubToken string `json:"github_token"`
	}
	json.Unmarshal(data, &auth)
	return auth.GitHubToken
}
