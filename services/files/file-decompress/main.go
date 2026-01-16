package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type DecompressRequest struct {
	ArchivePath string `json:"archive_path"`
	OutputDir   string `json:"output_dir"`
}

type DecompressResponse struct {
	Success        bool     `json:"success"`
	ExtractedFiles []string `json:"extracted_files"`
	Error          string   `json:"error,omitempty"`
}

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/decompress", decompressHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("File decompress service starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func decompressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DecompressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.ArchivePath == "" {
		sendError(w, "archive_path is required", http.StatusBadRequest)
		return
	}
	if req.OutputDir == "" {
		sendError(w, "output_dir is required", http.StatusBadRequest)
		return
	}

	// Ensure output directory exists
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		sendError(w, "Failed to create output directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var extractedFiles []string
	var err error

	lowerPath := strings.ToLower(req.ArchivePath)
	if strings.HasSuffix(lowerPath, ".zip") {
		extractedFiles, err = extractZip(req.ArchivePath, req.OutputDir)
	} else if strings.HasSuffix(lowerPath, ".tar.gz") || strings.HasSuffix(lowerPath, ".tgz") {
		extractedFiles, err = extractTarGz(req.ArchivePath, req.OutputDir)
	} else {
		sendError(w, "Unsupported archive format. Supported: .zip, .tar.gz, .tgz", http.StatusBadRequest)
		return
	}

	if err != nil {
		sendError(w, "Failed to extract archive: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DecompressResponse{
		Success:        true,
		ExtractedFiles: extractedFiles,
	})
}

func extractZip(archivePath, outputDir string) ([]string, error) {
	var extractedFiles []string

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		destPath := filepath.Join(outputDir, file.Name)

		// Security check: prevent zip slip attack
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(outputDir)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, file.Mode()); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create parent directory: %w", err)
		}

		srcFile, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file in archive: %w", err)
		}

		destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			srcFile.Close()
			return nil, fmt.Errorf("failed to create destination file: %w", err)
		}

		_, err = io.Copy(destFile, srcFile)
		srcFile.Close()
		destFile.Close()

		if err != nil {
			return nil, fmt.Errorf("failed to copy file contents: %w", err)
		}

		extractedFiles = append(extractedFiles, destPath)
	}

	return extractedFiles, nil
}

func extractTarGz(archivePath, outputDir string) ([]string, error) {
	var extractedFiles []string

	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		destPath := filepath.Join(outputDir, header.Name)

		// Security check: prevent path traversal attack
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(outputDir)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return nil, fmt.Errorf("failed to create parent directory: %w", err)
			}

			destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return nil, fmt.Errorf("failed to create destination file: %w", err)
			}

			_, err = io.Copy(destFile, tarReader)
			destFile.Close()

			if err != nil {
				return nil, fmt.Errorf("failed to copy file contents: %w", err)
			}

			extractedFiles = append(extractedFiles, destPath)
		case tar.TypeSymlink:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return nil, fmt.Errorf("failed to create parent directory: %w", err)
			}
			if err := os.Symlink(header.Linkname, destPath); err != nil {
				return nil, fmt.Errorf("failed to create symlink: %w", err)
			}
			extractedFiles = append(extractedFiles, destPath)
		}
	}

	return extractedFiles, nil
}

func sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(DecompressResponse{
		Success: false,
		Error:   message,
	})
}
