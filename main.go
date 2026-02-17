package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
)

var version = "dev"

func main() {
	outputDir := flag.String("o", ".", "output directory")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: gdoc2md [flags] <command|url>\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  configure    Set up Google OAuth2 credentials\n\n")
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  url          Google Docs URL to export\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("gdoc2md %s\n", version)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	switch args[0] {
	case "configure":
		if err := runConfigure(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		docID, err := extractDocID(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		client, err := GetAuthenticatedClient(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := ExportDoc(ctx, client, docID, *outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func runConfigure() error {
	var clientID, clientSecret string

	fmt.Print("Enter your Google OAuth2 Client ID: ")
	if _, err := fmt.Scanln(&clientID); err != nil {
		return fmt.Errorf("failed to read client ID: %w", err)
	}

	fmt.Print("Enter your OAuth2 Client Secret: ")
	if _, err := fmt.Scanln(&clientSecret); err != nil {
		return fmt.Errorf("failed to read client secret: %w", err)
	}

	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)

	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("client ID and secret cannot be empty")
	}

	if err := SaveAppConfig(&AppConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}); err != nil {
		return err
	}

	dir, _ := configDirPath()
	fmt.Printf("Credentials saved to %s/config.json\n", dir)
	return nil
}

// extractDocID parses a Google Docs URL and returns the document ID.
// Supports formats:
//   - https://docs.google.com/document/d/DOC_ID/edit
//   - https://docs.google.com/document/d/DOC_ID
//   - DOC_ID (plain ID)
func extractDocID(input string) (string, error) {
	input = strings.TrimSpace(input)

	// If it doesn't look like a URL, treat as a raw document ID.
	if !strings.Contains(input, "/") {
		return input, nil
	}

	u, err := url.Parse(input)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Expected path: /document/d/DOC_ID/...
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if part == "d" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}

	return "", fmt.Errorf("could not extract document ID from URL: %s", input)
}
