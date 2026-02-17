package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	docsv1 "google.golang.org/api/docs/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"
)

// tabResult holds the output of converting a single tab.
type tabResult struct {
	title    string
	filename string
	result   ConvertResult
}

// ExportDoc fetches a Google Doc and exports all tabs as markdown files.
func ExportDoc(ctx context.Context, client *http.Client, docID, outputDir string) error {
	srv, err := docsv1.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("failed to create Docs service: %w", err)
	}

	fmt.Printf("Fetching document %s...\n", docID)
	doc, err := srv.Documents.Get(docID).IncludeTabsContent(true).Do()
	if err != nil {
		return fmt.Errorf("failed to fetch document: %w", err)
	}

	// Flatten tab tree.
	tabs := flattenTabs(doc.Tabs)
	if len(tabs) == 0 {
		return fmt.Errorf("document has no tabs")
	}
	fmt.Printf("Found %d tab(s)\n", len(tabs))

	// Ensure output and images directories exist.
	imagesDir := filepath.Join(outputDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create images directory: %w", err)
	}

	// Process tabs in parallel.
	results := make([]tabResult, len(tabs))
	g, _ := errgroup.WithContext(ctx)
	for i, tab := range tabs {
		i, tab := i, tab
		g.Go(func() error {
			title := tabTitle(tab)
			filename := sanitizeFilename(title) + ".md"
			result := ConvertTab(tab, title, i)
			results[i] = tabResult{
				title:    title,
				filename: filename,
				result:   result,
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// Print conversion results (after parallel work, to avoid interleaved output).
	for _, r := range results {
		fmt.Printf("  Converted: %s\n", r.title)
	}

	// Collect all images from all tabs and download in parallel.
	var allImages []imageDownload
	for _, r := range results {
		for _, img := range r.result.Images {
			allImages = append(allImages, imageDownload{
				ref:       img,
				imagesDir: imagesDir,
			})
		}
	}

	if len(allImages) > 0 {
		fmt.Printf("Downloading %d image(s)...\n", len(allImages))
		if err := downloadImages(ctx, client, allImages); err != nil {
			return err
		}
	}

	// Write markdown files.
	for _, r := range results {
		outPath := filepath.Join(outputDir, r.filename)
		if err := os.WriteFile(outPath, []byte(r.result.Markdown), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", outPath, err)
		}
		fmt.Printf("  Wrote: %s\n", outPath)
	}

	// Write tabs.md index file.
	indexPath := filepath.Join(outputDir, "tabs.md")
	index := generateIndex(results)
	if err := os.WriteFile(indexPath, []byte(index), 0644); err != nil {
		return fmt.Errorf("failed to write tabs.md: %w", err)
	}
	fmt.Printf("  Wrote: %s\n", indexPath)

	fmt.Println("Done!")
	return nil
}

func flattenTabs(tabs []*docsv1.Tab) []*docsv1.Tab {
	var result []*docsv1.Tab
	for _, tab := range tabs {
		result = append(result, tab)
		result = append(result, flattenTabs(tab.ChildTabs)...)
	}
	return result
}

func tabTitle(tab *docsv1.Tab) string {
	if tab.TabProperties != nil && tab.TabProperties.Title != "" {
		return tab.TabProperties.Title
	}
	return "Untitled"
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
	)
	name = replacer.Replace(name)
	name = strings.TrimSpace(name)
	if name == "" {
		return "untitled"
	}
	return name
}

type imageDownload struct {
	ref       ImageRef
	imagesDir string
}

func downloadImages(ctx context.Context, client *http.Client, images []imageDownload) error {
	g, gctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, 10)
	var mu sync.Mutex
	var warnings []string

	for _, img := range images {
		img := img
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := downloadImage(gctx, client, img.ref.ContentURI, filepath.Join(img.imagesDir, img.ref.Filename)); err != nil {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("%s: %v", img.ref.Filename, err))
				mu.Unlock()
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warning: failed to download %d image(s):\n", len(warnings))
		for _, w := range warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
	return nil
}

func downloadImage(ctx context.Context, client *http.Client, uri, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	const maxImageSize = 50 << 20 // 50 MB
	_, err = io.Copy(f, io.LimitReader(resp.Body, maxImageSize))
	return err
}

func generateIndex(results []tabResult) string {
	var sb strings.Builder
	sb.WriteString("# Table of Contents\n\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("- [%s](%s)\n", r.title, r.filename))
	}
	sb.WriteString("\n")
	return sb.String()
}
