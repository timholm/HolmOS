package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var (
	requestCount uint64
	startTime    = time.Now()
)

const dataPath = "/data"

type CompressRequest struct {
	Paths      []string `json:"paths"`       // List of files/folders to compress
	OutputPath string   `json:"output_path"` // Output zip file path
}

type CompressResponse struct {
	OutputPath   string   `json:"output_path,omitempty"`
	FilesAdded   int      `json:"files_added,omitempty"`
	TotalSize    int64    `json:"total_size,omitempty"`
	CompressedSize int64  `json:"compressed_size,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

type MetricsResponse struct {
	Uptime   string `json:"uptime"`
	Requests uint64 `json:"requests"`
	Service  string `json:"service"`
}

func addFileToZip(zipWriter *zip.Writer, filePath, basePath string) (int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return 0, err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return 0, err
	}

	relPath, err := filepath.Rel(basePath, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	header.Name = relPath
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return 0, err
	}

	_, err = io.Copy(writer, file)
	return info.Size(), err
}

func addDirToZip(zipWriter *zip.Writer, dirPath, basePath string) (int, int64, error) {
	var filesAdded int
	var totalSize int64

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		size, err := addFileToZip(zipWriter, path, basePath)
		if err != nil {
			return err
		}

		filesAdded++
		totalSize += size
		return nil
	})

	return filesAdded, totalSize, err
}

func compressHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(CompressResponse{Error: "Method not allowed"})
		return
	}

	var req CompressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CompressResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if len(req.Paths) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CompressResponse{Error: "Paths are required"})
		return
	}

	if req.OutputPath == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CompressResponse{Error: "Output path is required"})
		return
	}

	// Sanitize output path
	cleanOutput := filepath.Clean(req.OutputPath)
	if strings.Contains(cleanOutput, "..") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(CompressResponse{Error: "Invalid output path"})
		return
	}

	outputFullPath := filepath.Join(dataPath, cleanOutput)
	if !strings.HasSuffix(outputFullPath, ".zip") {
		outputFullPath += ".zip"
	}

	// Create output directory if needed
	if err := os.MkdirAll(filepath.Dir(outputFullPath), 0755); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(CompressResponse{Error: "Failed to create output directory: " + err.Error()})
		return
	}

	// Create zip file
	zipFile, err := os.Create(outputFullPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(CompressResponse{Error: "Failed to create zip file: " + err.Error()})
		return
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	var totalFiles int
	var totalSize int64

	for _, path := range req.Paths {
		cleanPath := filepath.Clean(path)
		if strings.Contains(cleanPath, "..") {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(CompressResponse{Error: "Invalid path: " + path})
			return
		}

		fullPath := filepath.Join(dataPath, cleanPath)
		info, err := os.Stat(fullPath)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(CompressResponse{Error: "Path not found: " + path})
			return
		}

		if info.IsDir() {
			files, size, err := addDirToZip(zipWriter, fullPath, filepath.Dir(fullPath))
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(CompressResponse{Error: "Failed to add directory: " + err.Error()})
				return
			}
			totalFiles += files
			totalSize += size
		} else {
			size, err := addFileToZip(zipWriter, fullPath, filepath.Dir(fullPath))
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(CompressResponse{Error: "Failed to add file: " + err.Error()})
				return
			}
			totalFiles++
			totalSize += size
		}
	}

	// Close zip writer to flush
	zipWriter.Close()

	// Get compressed size
	stat, _ := os.Stat(outputFullPath)
	compressedSize := stat.Size()

	json.NewEncoder(w).Encode(CompressResponse{
		OutputPath:     req.OutputPath,
		FilesAdded:     totalFiles,
		TotalSize:      totalSize,
		CompressedSize: compressedSize,
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status:    "healthy",
		Service:   "file-compress",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MetricsResponse{
		Uptime:   time.Since(startTime).String(),
		Requests: atomic.LoadUint64(&requestCount),
		Service:  "file-compress",
	})
}

func main() {
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Printf("Warning: Could not create data directory: %v", err)
	}

	http.HandleFunc("/compress", compressHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)

	fmt.Println("file-compress service starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
