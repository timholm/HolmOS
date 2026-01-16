package main

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/image/draw"
)

var (
	requestCount uint64
	startTime    = time.Now()
)

const dataPath = "/data"

func resizeImage(src image.Image, size int) image.Image {
	srcBounds := src.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	if size == 0 {
		size = 200
	}

	// Maintain aspect ratio - fit within size x size bounds
	var newWidth, newHeight int
	if srcWidth > srcHeight {
		newWidth = size
		newHeight = int(float64(size) * float64(srcHeight) / float64(srcWidth))
	} else {
		newHeight = size
		newWidth = int(float64(size) * float64(srcWidth) / float64(srcHeight))
	}

	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, draw.Over, nil)

	return dst
}

func thumbnailHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// GET /thumbnail?path=/some/image.jpg&size=200
	path := r.URL.Query().Get("path")
	sizeStr := r.URL.Query().Get("size")

	if path == "" {
		http.Error(w, "path parameter is required", http.StatusBadRequest)
		return
	}

	size := 200
	if sizeStr != "" {
		var err error
		size, err = strconv.Atoi(sizeStr)
		if err != nil || size < 1 || size > 2000 {
			http.Error(w, "invalid size parameter (must be 1-2000)", http.StatusBadRequest)
			return
		}
	}

	// Sanitize path
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(dataPath, cleanPath)

	// Check file extension
	ext := strings.ToLower(filepath.Ext(fullPath))
	supportedFormats := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	}
	if !supportedFormats[ext] {
		http.Error(w, "unsupported image format", http.StatusBadRequest)
		return
	}

	file, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Decode image
	var img image.Image
	switch ext {
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(file)
	case ".png":
		img, err = png.Decode(file)
	case ".gif":
		img, err = gif.Decode(file)
	}

	if err != nil {
		http.Error(w, "failed to decode image", http.StatusInternalServerError)
		return
	}

	// Resize
	thumbnail := resizeImage(img, size)

	// Encode and write response
	var buf bytes.Buffer
	var contentType string

	switch ext {
	case ".png":
		err = png.Encode(&buf, thumbnail)
		contentType = "image/png"
	case ".gif":
		err = gif.Encode(&buf, thumbnail, nil)
		contentType = "image/gif"
	default:
		err = jpeg.Encode(&buf, thumbnail, &jpeg.Options{Quality: 85})
		contentType = "image/jpeg"
	}

	if err != nil {
		http.Error(w, "failed to encode thumbnail", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(buf.Bytes())
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy","service":"file-thumbnail"}`))
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uptime := time.Since(startTime).String()
	requests := atomic.LoadUint64(&requestCount)
	w.Write([]byte(`{"uptime":"` + uptime + `","requests":` + strconv.FormatUint(requests, 10) + `,"service":"file-thumbnail"}`))
}

func main() {
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Printf("Warning: Could not create data directory: %v", err)
	}

	http.HandleFunc("/thumbnail", thumbnailHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)

	log.Println("file-thumbnail service starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
