package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

var baseURL = "http://localhost:30088"

func main() {
	if url := os.Getenv("HOLM_URL"); url != "" {
		baseURL = url
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "ls", "list":
		err = cmdList(args)
	case "get", "download":
		err = cmdDownload(args)
	case "put", "upload":
		err = cmdUpload(args)
	case "rm", "delete":
		err = cmdDelete(args)
	case "mkdir":
		err = cmdMkdir(args)
	case "mv", "move":
		err = cmdMove(args)
	case "cp", "copy":
		err = cmdCopy(args)
	case "stat", "meta":
		err = cmdMeta(args)
	case "find", "search":
		err = cmdSearch(args)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Holm File Storage CLI

Usage: holm <command> [arguments]

Commands:
  ls [path]              List files in directory
  get <path> [local]     Download file to local path
  put <local> [path]     Upload local file to remote path
  rm <path>              Delete file or directory
  mkdir <path>           Create directory
  mv <src> <dst>         Move/rename file or directory
  cp <src> <dst>         Copy file
  stat <path>            Get file metadata
  find <query> [path]    Search for files

Environment:
  HOLM_URL               Base URL (default: http://localhost:30088)

Examples:
  holm ls
  holm ls documents
  holm put ./file.txt uploads/file.txt
  holm get uploads/file.txt ./downloaded.txt
  holm mkdir projects/new-project
  holm find "*.go" src`)
}

func cmdList(args []string) error {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}

	resp, err := http.Get(baseURL + "/api/v1/files/" + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Path  string `json:"path"`
		Files []struct {
			Name    string `json:"name"`
			Path    string `json:"path"`
			Size    int64  `json:"size"`
			IsDir   bool   `json:"is_dir"`
			ModTime string `json:"mod_time"`
		} `json:"files"`
		Count int `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "TYPE\tSIZE\tNAME\n")
	for _, f := range result.Files {
		ftype := "file"
		size := formatSize(f.Size)
		if f.IsDir {
			ftype = "dir"
			size = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", ftype, size, f.Name)
	}
	w.Flush()
	fmt.Printf("\nTotal: %d items\n", result.Count)
	return nil
}

func cmdDownload(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: holm get <remote-path> [local-path]")
	}

	remotePath := args[0]
	localPath := filepath.Base(remotePath)
	if len(args) > 1 {
		localPath = args[1]
	}

	resp, err := http.Get(baseURL + "/api/v1/download/" + remotePath)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed: %s", string(body))
	}

	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	fmt.Printf("Downloaded %s (%s)\n", localPath, formatSize(written))
	return nil
}

func cmdUpload(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: holm put <local-path> [remote-path]")
	}

	localPath := args[0]
	remotePath := ""
	if len(args) > 1 {
		remotePath = args[1]
	}

	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return err
	}
	io.Copy(part, file)

	if remotePath != "" {
		dir := filepath.Dir(remotePath)
		if dir != "." {
			writer.WriteField("path", dir)
		}
		writer.WriteField("filename", filepath.Base(remotePath))
	}

	writer.Close()

	resp, err := http.Post(baseURL+"/api/v1/upload", writer.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Path    string `json:"path"`
		Size    int64  `json:"size"`
		Error   string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.Success {
		return fmt.Errorf("upload failed: %s", result.Error)
	}

	fmt.Printf("Uploaded %s (%s)\n", result.Path, formatSize(result.Size))
	return nil
}

func cmdDelete(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: holm rm <path>")
	}

	path := args[0]
	recursive := len(args) > 1 && args[1] == "-r"

	url := baseURL + "/api/v1/delete/" + path
	if recursive {
		url += "?recursive=true"
	}

	req, _ := http.NewRequest("DELETE", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.Success {
		return fmt.Errorf("delete failed: %s", result.Error)
	}

	fmt.Printf("Deleted %s\n", path)
	return nil
}

func cmdMkdir(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: holm mkdir <path>")
	}

	resp, err := http.Post(baseURL+"/api/v1/mkdir/"+args[0], "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.Success {
		return fmt.Errorf("mkdir failed: %s", result.Error)
	}

	fmt.Printf("Created directory %s\n", args[0])
	return nil
}

func cmdMove(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: holm mv <source> <dest>")
	}

	body, _ := json.Marshal(map[string]string{
		"source": args[0],
		"dest":   args[1],
	})

	resp, err := http.Post(baseURL+"/api/v1/move", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.Success {
		return fmt.Errorf("move failed: %s", result.Error)
	}

	fmt.Printf("Moved %s -> %s\n", args[0], args[1])
	return nil
}

func cmdCopy(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: holm cp <source> <dest>")
	}

	body, _ := json.Marshal(map[string]string{
		"source": args[0],
		"dest":   args[1],
	})

	resp, err := http.Post(baseURL+"/api/v1/copy", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Size    int64  `json:"size"`
		Error   string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.Success {
		return fmt.Errorf("copy failed: %s", result.Error)
	}

	fmt.Printf("Copied %s -> %s (%s)\n", args[0], args[1], formatSize(result.Size))
	return nil
}

func cmdMeta(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: holm stat <path>")
	}

	resp, err := http.Get(baseURL + "/api/v1/meta/" + args[0] + "?checksum=true")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
		Meta    struct {
			Path     string `json:"path"`
			Name     string `json:"name"`
			Size     int64  `json:"size"`
			IsDir    bool   `json:"is_dir"`
			Mode     string `json:"mode"`
			ModTime  string `json:"mod_time"`
			Checksum string `json:"checksum"`
			Mime     string `json:"mime"`
		} `json:"meta"`
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.Success {
		return fmt.Errorf("stat failed: %s", result.Error)
	}

	m := result.Meta
	fmt.Printf("Path:     %s\n", m.Path)
	fmt.Printf("Name:     %s\n", m.Name)
	fmt.Printf("Size:     %s (%d bytes)\n", formatSize(m.Size), m.Size)
	fmt.Printf("Type:     %s\n", map[bool]string{true: "directory", false: "file"}[m.IsDir])
	fmt.Printf("Mode:     %s\n", m.Mode)
	fmt.Printf("Modified: %s\n", m.ModTime)
	if m.Mime != "" {
		fmt.Printf("MIME:     %s\n", m.Mime)
	}
	if m.Checksum != "" {
		fmt.Printf("SHA256:   %s\n", m.Checksum)
	}
	return nil
}

func cmdSearch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: holm find <query> [path]")
	}

	query := args[0]
	path := ""
	if len(args) > 1 {
		path = args[1]
	}

	url := baseURL + "/api/v1/search?q=" + query
	if path != "" {
		url += "&path=" + path
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
		Results []struct {
			Path  string `json:"path"`
			Name  string `json:"name"`
			Size  int64  `json:"size"`
			IsDir bool   `json:"is_dir"`
		} `json:"results"`
		Count int    `json:"count"`
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.Success {
		return fmt.Errorf("search failed: %s", result.Error)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "TYPE\tSIZE\tPATH\n")
	for _, f := range result.Results {
		ftype := "file"
		size := formatSize(f.Size)
		if f.IsDir {
			ftype = "dir"
			size = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", ftype, size, f.Path)
	}
	w.Flush()
	fmt.Printf("\nFound: %d results\n", result.Count)
	return nil
}

func formatSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	const k = 1024
	sizes := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	size := float64(bytes)
	for size >= k && i < len(sizes)-1 {
		size /= k
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", size), "0"), ".") + " " + sizes[i]
}
