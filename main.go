package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func extractImages(htmlContent string) ([]string, error) {
	var allImageURLs []string

	// Extract background-image URLs from <style> tags
	styleRegex := regexp.MustCompile(`(?s)<style.*?>.*?</style>`)
	styleMatches := styleRegex.FindAllString(htmlContent, -1)
	for _, styleMatch := range styleMatches {
		bgImageRegex := regexp.MustCompile(`background-image\s*:\s*([^;]+)`)
		bgImageMatches := bgImageRegex.FindAllStringSubmatch(styleMatch, -1)

		urlRegex := regexp.MustCompile(`url\(['"]?([^'")]+)['"]?\)`)
		for _, bgImageMatch := range bgImageMatches {
			if len(bgImageMatch) < 2 {
				continue
			}
			urlMatches := urlRegex.FindAllStringSubmatch(bgImageMatch[1], -1)
			for _, match := range urlMatches {
				if len(match) > 1 {
					url := strings.TrimSpace(match[1])
					if url != "" {
						allImageURLs = append(allImageURLs, url)
					}
				}
			}
		}
	}

	// Extract <img> src attributes
	imgRegex := regexp.MustCompile(`<img[^>]+src=['"]?([^'"\s>]+)['"]?`)
	imgMatches := imgRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, match := range imgMatches {
		if len(match) > 1 {
			url := strings.TrimSpace(match[1])
			if url != "" {
				allImageURLs = append(allImageURLs, url)
			}
		}
	}

	if len(allImageURLs) == 0 {
		return nil, nil
	}
	return allImageURLs, nil
}

func extractStylesheets(htmlContent string) ([]string, error) {
	// More flexible regex for <link> tags
	linkRegex := regexp.MustCompile(`<link[^>]+href=['"]?([^'"\s>]+)['"]?[^>]*rel=['"]?stylesheet['"]?`)
	linkMatches := linkRegex.FindAllStringSubmatch(htmlContent, -1)
	var stylesheets []string
	for _, match := range linkMatches {
		if len(match) > 1 {
			url := strings.TrimSpace(match[1])
			if url != "" {
				stylesheets = append(stylesheets, url)
			}
		}
	}

	// Debug: Print if no stylesheets found
	if len(stylesheets) == 0 {
		fmt.Println("No stylesheets found in HTML")
	}

	return stylesheets, nil
}

func downloadFile(urlStr, targetPath string, baseURL string) error {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("error parsing URL %s: %v", urlStr, err)
	}

	if !parsedURL.IsAbs() {
		base, err := url.Parse(baseURL)
		if err != nil {
			return fmt.Errorf("error parsing base URL %s: %v", baseURL, err)
		}
		parsedURL = base.ResolveReference(parsedURL)
	}

	err = os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return fmt.Errorf("error creating directory for %s: %v", targetPath, err)
	}

	resp, err := http.Get(parsedURL.String())
	if err != nil {
		return fmt.Errorf("error downloading %s: %v", parsedURL.String(), err)
	}
	defer resp.Body.Close()

	out, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("error creating file %s: %v", targetPath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func updateImageURLs(content string, oldURLs, newURLs []string) string {
	for i, oldURL := range oldURLs {
		if i < len(newURLs) {
			bgPattern := fmt.Sprintf(`url\(['"]?%s['"]?\)`, regexp.QuoteMeta(oldURL))
			newBgPattern := fmt.Sprintf(`url("%s")`, newURLs[i])
			content = regexp.MustCompile(bgPattern).ReplaceAllString(content, newBgPattern)

			imgPattern := fmt.Sprintf(`src=['"]?%s['"]?`, regexp.QuoteMeta(oldURL))
			newImgPattern := fmt.Sprintf(`src="%s"`, newURLs[i])
			content = regexp.MustCompile(imgPattern).ReplaceAllString(content, newImgPattern)
		}
	}
	return content
}

func updateStylesheetURLs(content string, oldURLs, newURLs []string) string {
	for i, oldURL := range oldURLs {
		if i < len(newURLs) {
			linkPattern := fmt.Sprintf(`href=['"]?%s['"]?`, regexp.QuoteMeta(oldURL))
			newLinkPattern := fmt.Sprintf(`href="%s"`, newURLs[i])
			content = regexp.MustCompile(linkPattern).ReplaceAllString(content, newLinkPattern)
		}
	}
	return content
}

func processHTMLContent(content string, filePath string, baseURL string, targetDir string) error {
	// Process images
	images, err := extractImages(content)
	if err != nil {
		return fmt.Errorf("error extracting images from %s: %v", filePath, err)
	}

	var newImagePaths []string
	if len(images) > 0 {
		fmt.Printf("\nFile: %s\n", filePath)
		fmt.Println("Found images:")
		for i, img := range images {
			fmt.Printf("%d: %s\n", i+1, img)
			
			targetPath := filepath.Join(targetDir, strings.TrimLeft(img, "/"))
			newImagePaths = append(newImagePaths, strings.TrimLeft(img, "/"))
			
			err := downloadFile(img, targetPath, baseURL)
			if err != nil {
				fmt.Printf("Failed to download %s: %v\n", img, err)
			} else {
				fmt.Printf("Downloaded %s to %s\n", img, targetPath)
			}
		}
	}

	// Process stylesheets
	stylesheets, err := extractStylesheets(content)
	if err != nil {
		return fmt.Errorf("error extracting stylesheets from %s: %v", filePath, err)
	}

	var newStylesheetPaths []string
	if len(stylesheets) > 0 {
		cssDir := filepath.Join(targetDir, "css")
		fmt.Println("Found stylesheets:")
		for i, css := range stylesheets {
			fmt.Printf("%d: %s\n", i+1, css)
			
			filename := filepath.Base(css)
			targetPath := filepath.Join(cssDir, filename)
			newStylesheetPaths = append(newStylesheetPaths, filepath.Join("css", filename))
			
			err := downloadFile(css, targetPath, baseURL)
			if err != nil {
				fmt.Printf("Failed to download %s: %v\n", css, err)
			} else {
				fmt.Printf("Downloaded %s to %s\n", css, targetPath)
			}
		}
	}

	// Update HTML content with new paths
	updatedContent := content
	if len(images) > 0 {
		updatedContent = updateImageURLs(updatedContent, images, newImagePaths)
	}
	if len(stylesheets) > 0 {
		updatedContent = updateStylesheetURLs(updatedContent, stylesheets, newStylesheetPaths)
	}

	// Write updated content back to file
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	if err != nil {
		return fmt.Errorf("error updating file %s: %v", filePath, err)
	}
	
	return nil
}

func downloadAndSave(urlStr, baseDir string, convertLinks bool) error {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL %s: %v", urlStr, err)
	}
	targetDir := filepath.Join(baseDir, parsedURL.Hostname())
	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating directory %s: %v", targetDir, err)
	}

	resp, err := http.Get(urlStr)
	if err != nil {
		return fmt.Errorf("error downloading %s: %v", urlStr, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	outputFile := filepath.Join(targetDir, "index.html")
	err = os.WriteFile(outputFile, body, 0644)
	if err != nil {
		return fmt.Errorf("error writing file %s: %v", outputFile, err)
	}

	if convertLinks {
		content := string(body)
		content = strings.ReplaceAll(content, urlStr, "/")
		err = os.WriteFile(outputFile, []byte(content), 0644)
		if err != nil {
			return fmt.Errorf("error writing converted file %s: %v", outputFile, err)
		}
	}

	return processHTMLContent(string(body), outputFile, urlStr, targetDir)
}

func main() {
	mirror := flag.Bool("mirror", false, "Mirror the website")
	convertLinks := flag.Bool("convert-links", false, "Convert absolute links to relative")
	dirPath := flag.String("dir", "", "Directory path containing HTML files")

	flag.Parse()

	args := flag.Args()
	if len(args) > 0 {
		parsedURL, err := url.Parse(args[0])
		if err == nil && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https") {
			if !*mirror {
				fmt.Println("Error: --mirror flag is required for URL downloads")
				fmt.Println("Usage: ./wget --mirror [--convert-links] <url>")
				os.Exit(1)
			}

			err := downloadAndSave(args[0], ".", *convertLinks)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if *dirPath == "" {
		fmt.Println("Usage:")
		fmt.Println("  For downloading: ./wget --mirror [--convert-links] <url>")
		fmt.Println("  For scanning: ./wget -dir <directory_path>")
		fmt.Println("Example: ./wget --mirror https://trypap.com/")
		fmt.Println("Example: ./wget -dir ./website")
		os.Exit(1)
	}

	if _, err := os.Stat(*dirPath); os.IsNotExist(err) {
		fmt.Printf("Error: Directory %s does not exist\n", *dirPath)
		os.Exit(1)
	}

	err := scanDirectory(*dirPath, "file://"+*dirPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func scanDirectory(dirPath string, baseURL string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".html") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading file %s: %v", path, err)
		}
		return processHTMLContent(string(content), path, baseURL, dirPath)
	})
}
