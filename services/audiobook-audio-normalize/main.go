package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

type NormalizeRequest struct {
	JobID  string `json:"job_id"`
	Input  string `json:"input"`
	Output string `json:"output"`
}

type NormalizeResponse struct {
	JobID  string `json:"job_id"`
	Output string `json:"output"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func main() {
	http.HandleFunc("/normalize", handleNormalize)
	http.HandleFunc("/health", handleHealth)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("audiobook-audio-normalize service starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleNormalize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req NormalizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, "", "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.JobID == "" || req.Input == "" || req.Output == "" {
		sendErrorResponse(w, req.JobID, "Missing required fields: job_id, input, output", http.StatusBadRequest)
		return
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(req.Output)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		sendErrorResponse(w, req.JobID, "Failed to create output directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if input file exists
	if _, err := os.Stat(req.Input); os.IsNotExist(err) {
		sendErrorResponse(w, req.JobID, "Input file not found: "+req.Input, http.StatusBadRequest)
		return
	}

	log.Printf("[%s] Normalizing audio: %s -> %s", req.JobID, req.Input, req.Output)

	// Use ffmpeg loudnorm filter for audio normalization (EBU R128)
	cmd := exec.Command("ffmpeg",
		"-i", req.Input,
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=11:print_format=summary",
		"-ar", "44100",
		"-y",
		req.Output,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[%s] ffmpeg error: %v, output: %s", req.JobID, err, string(output))
		sendErrorResponse(w, req.JobID, fmt.Sprintf("ffmpeg normalization failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[%s] Normalization complete: %s", req.JobID, req.Output)

	// Verify output file was created
	if _, err := os.Stat(req.Output); os.IsNotExist(err) {
		sendErrorResponse(w, req.JobID, "Output file was not created", http.StatusInternalServerError)
		return
	}

	resp := NormalizeResponse{
		JobID:  req.JobID,
		Output: req.Output,
		Status: "completed",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func sendErrorResponse(w http.ResponseWriter, jobID, errMsg string, statusCode int) {
	resp := NormalizeResponse{
		JobID:  jobID,
		Status: "failed",
		Error:  errMsg,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}
