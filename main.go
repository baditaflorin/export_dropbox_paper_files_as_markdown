package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
)

var debugMode bool

// exportFile calls the /files/export endpoint to export a file (e.g. a Dropbox Paper doc)
// in the desired format (here, markdown).
func exportFile(fileID, token string) (string, error) {
	// Dropbox API endpoint for export.
	url := "https://content.dropboxapi.com/2/files/export"
	client := &http.Client{}

	// Prepare the Dropbox-API-Arg header as a JSON string.
	// Use fileID (which is in the format "id:...") and specify export_format.
	arg := map[string]string{
		"path":          fileID,
		"export_format": "markdown",
	}
	argJSON, err := json.Marshal(arg)
	if err != nil {
		return "", err
	}

	// Create a new POST request.
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Dropbox-API-Arg", string(argJSON))
	// Note: The body is empty; content is returned as the response body.

	if debugMode {
		log.Printf("Sending export request to %s with Dropbox-API-Arg: %s", url, string(argJSON))
	}

	// Execute the request.
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if debugMode {
		log.Printf("Received response with status: %s", resp.Status)
	}

	// Read the response body.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error exporting file: %s", data)
	}
	if debugMode {
		log.Printf("Exported file content length: %d", len(data))
	}
	return string(data), nil
}

func main() {
	// Parse debug flag.
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()
	debugMode = *debug

	// Get Dropbox access token from the environment.
	accessToken := os.Getenv("DROPBOX_ACCESS_TOKEN")
	if accessToken == "" {
		log.Fatal("DROPBOX_ACCESS_TOKEN is not set")
	}

	// Set up Dropbox client configuration.
	config := dropbox.Config{
		Token: accessToken,
	}
	if debugMode {
		config.LogLevel = dropbox.LogInfo
	} else {
		config.LogLevel = dropbox.LogOff
	}
	dbx := files.New(config)

	// Define the base folder in Dropbox where the Paper docs reside.
	baseFolder := "/Migrated Paper Docs"

	// List all files within the base folder, recursively.
	arg := files.NewListFolderArg(baseFolder)
	arg.Recursive = true

	if debugMode {
		log.Printf("Listing files in Dropbox folder: %s", baseFolder)
	}
	res, err := dbx.ListFolder(arg)
	if err != nil {
		log.Fatalf("Failed to list folder: %v", err)
	}

	// Directory to store exported Paper docs as Markdown.
	outputDir := "output_paper_markdown"
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// PAGINATION FIX: Start a loop to handle multiple pages of results.
	for {
		// Iterate over the file entries for the current page.
		for _, entry := range res.Entries {
			// We only care about files.
			fileMeta, ok := entry.(*files.FileMetadata)
			if !ok {
				continue
			}

			// Only process files ending in ".paper"
			if !strings.HasSuffix(fileMeta.Name, ".paper") {
				if debugMode {
					log.Printf("Skipping non-Paper file: %s", fileMeta.PathDisplay)
				}
				continue
			}

			fmt.Printf("Exporting Dropbox Paper doc: %s\n", fileMeta.PathDisplay)

			// Use the file ID (which is in "id:..." format) for the export call.
			exportedContent, err := exportFile(fileMeta.Id, accessToken)
			if err != nil {
				log.Printf("Failed to export file %s: %v", fileMeta.PathDisplay, err)
				continue
			}

			// Remove the base folder prefix so that the local output preserves the relative path.
			relativePath := strings.TrimPrefix(fileMeta.PathDisplay, baseFolder)
			relativePath = strings.TrimPrefix(relativePath, "/") // Remove leading slash if any

			// Construct the output file path: replace ".paper" extension with ".md".
			outputPath := filepath.Join(outputDir, relativePath)
			outputPath = strings.TrimSuffix(outputPath, ".paper") + ".md"

			if debugMode {
				log.Printf("Writing exported content to %s", outputPath)
			}

			// Ensure the output directory exists.
			if err := os.MkdirAll(filepath.Dir(outputPath), os.ModePerm); err != nil {
				log.Printf("Failed to create directory for %s: %v", outputPath, err)
				continue
			}

			// Write the exported Markdown content to the output file.
			if err := os.WriteFile(outputPath, []byte(exportedContent), 0644); err != nil {
				log.Printf("Failed to write file %s: %v", outputPath, err)
				continue
			}

			fmt.Printf("Exported and saved Paper doc as: %s\n", outputPath)
		}

		// PAGINATION FIX: Check if there are more files to fetch. If not, break the loop.
		if !res.HasMore {
			break
		}

		// PAGINATION FIX: If there are more files, call ListFolderContinue to get the next page.
		if debugMode {
			log.Println("Fetching next page of files...")
		}
		continueArg := files.NewListFolderContinueArg(res.Cursor)
		res, err = dbx.ListFolderContinue(continueArg)
		if err != nil {
			log.Fatalf("Failed to get next page of files: %v", err)
		}
	}
}
