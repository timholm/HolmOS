package main

import (
	"embed"
	"log"
	"net/http"
)

//go:embed index.html
var content embed.FS

func main() {
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/health", handleHealth)

	log.Println("Clock App listening on :30007")
	log.Fatal(http.ListenAndServe(":30007", nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, _ := content.ReadFile("index.html")
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("ok"))
}
