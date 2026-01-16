package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

type EncryptRequest struct {
	Input  string `json:"input"`
	Output string `json:"output"`
	Key    string `json:"key"`
}

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type KeyResponse struct {
	Key string `json:"key"`
}

func main() {
	http.HandleFunc("/encrypt", encryptHandler)
	http.HandleFunc("/decrypt", decryptHandler)
	http.HandleFunc("/generate-key", generateKeyHandler)
	http.HandleFunc("/health", healthHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("file-encrypt service starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Success: true, Message: "healthy"})
}

func generateKeyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "method not allowed"})
		return
	}

	key := make([]byte, 32) // 256 bits for AES-256
	if _, err := rand.Read(key); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to generate key"})
		return
	}

	json.NewEncoder(w).Encode(KeyResponse{Key: base64.StdEncoding.EncodeToString(key)})
}

func encryptHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "method not allowed"})
		return
	}

	var req EncryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "invalid JSON"})
		return
	}

	key, err := base64.StdEncoding.DecodeString(req.Key)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "invalid base64 key"})
		return
	}

	if len(key) != 32 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "key must be 32 bytes (256 bits)"})
		return
	}

	plaintext, err := os.ReadFile(req.Input)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to read input file: " + err.Error()})
		return
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to create cipher"})
		return
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to create GCM"})
		return
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to generate nonce"})
		return
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	if err := os.WriteFile(req.Output, ciphertext, 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to write output file: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(Response{Success: true, Message: "file encrypted successfully"})
}

func decryptHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "method not allowed"})
		return
	}

	var req EncryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "invalid JSON"})
		return
	}

	key, err := base64.StdEncoding.DecodeString(req.Key)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "invalid base64 key"})
		return
	}

	if len(key) != 32 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "key must be 32 bytes (256 bits)"})
		return
	}

	ciphertext, err := os.ReadFile(req.Input)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to read input file: " + err.Error()})
		return
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to create cipher"})
		return
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to create GCM"})
		return
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "ciphertext too short"})
		return
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "decryption failed: " + err.Error()})
		return
	}

	if err := os.WriteFile(req.Output, plaintext, 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(Response{Success: false, Error: "failed to write output file: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(Response{Success: true, Message: "file decrypted successfully"})
}
