package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "login":
		handleLogin()
	case "init":
		fmt.Println("Scaffolding new K11 Addon project...")
		// TODO: Implement init logic
	case "build":
		handleBuild()
	case "publish":
		handlePublish()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`k11-addon: CLI tool for K11 Addon Developers

Usage:
  k11-addon <command> [arguments]

Commands:
  login      Authenticate with GitHub using Device Flow
  init       Initialize a new K11 addon project
  build      Compile the addon and generate distribution.json
  publish    Publish the addon using marketplace metadata (e.g. category)`)
}
