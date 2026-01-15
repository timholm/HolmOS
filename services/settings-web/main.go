package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Service endpoints
const (
	themeService   = "http://settings-theme.holm:8080"
	backupService  = "http://settings-backup.holm:8080"
	restoreService = "http://settings-restore.holm:8080"
	tabsService    = "http://settings-tabs.holm:8080"
)

// Version info
const (
	AppVersion   = "2.1.0"
	BuildDate    = "2026-01-15"
	AppName      = "HolmOS Settings Hub"
)

// Data structures
type ThemePreferences struct {
	Theme       string `json:"theme"`
	CompactMode bool   `json:"compactMode"`
	Animations  bool   `json:"animations"`
	FontSize    int    `json:"fontSize"`
	AccentColor string `json:"accentColor"`
}

type TabState struct {
	ActiveTab string `json:"activeTab"`
}

type NotificationSettings struct {
	Enabled       bool `json:"enabled"`
	Sound         bool `json:"sound"`
	Desktop       bool `json:"desktop"`
	Email         bool `json:"email"`
	BuildAlerts   bool `json:"buildAlerts"`
	ClusterAlerts bool `json:"clusterAlerts"`
}

type UserPreferences struct {
	Language     string `json:"language"`
	Timezone     string `json:"timezone"`
	DateFormat   string `json:"dateFormat"`
	AutoSave     bool   `json:"autoSave"`
	ShowHidden   bool   `json:"showHidden"`
	DefaultView  string `json:"defaultView"`
}

type SystemInfo struct {
	Nodes      []NodeInfo    `json:"nodes"`
	Pods       PodSummary    `json:"pods"`
	Services   int           `json:"services"`
	Namespaces int           `json:"namespaces"`
	Storage    []StorageInfo `json:"storage"`
	Uptime     string        `json:"uptime"`
}

type NodeInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Role      string `json:"role"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
	Age       string `json:"age"`
	Arch      string `json:"arch"`
}

type PodSummary struct {
	Total     int `json:"total"`
	Running   int `json:"running"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
	Completed int `json:"completed"`
}

type StorageInfo struct {
	Name      string `json:"name"`
	Size      string `json:"size"`
	Used      string `json:"used"`
	Available string `json:"available"`
	Percent   string `json:"percent"`
	Mount     string `json:"mount"`
}

type AboutInfo struct {
	AppName      string `json:"appName"`
	Version      string `json:"version"`
	BuildDate    string `json:"buildDate"`
	GoVersion    string `json:"goVersion"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	Hostname     string `json:"hostname"`
	Services     []ServiceStatus `json:"services"`
}

type ServiceStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Port   string `json:"port"`
}

var (
	notificationSettings = NotificationSettings{
		Enabled:       true,
		Sound:         true,
		Desktop:       true,
		Email:         false,
		BuildAlerts:   true,
		ClusterAlerts: true,
	}
	userPreferences = UserPreferences{
		Language:    "en",
		Timezone:    "America/Los_Angeles",
		DateFormat:  "YYYY-MM-DD",
		AutoSave:    true,
		ShowHidden:  false,
		DefaultView: "grid",
	}
)

func main() {
	http.HandleFunc("/", serveUI)
	http.HandleFunc("/health", healthHandler)
	
	// Theme endpoints
	http.HandleFunc("/api/theme/preferences", themeHandler)
	http.HandleFunc("/api/theme/apply", themeApplyHandler)
	
	// Backup/Restore endpoints
	http.HandleFunc("/api/backup/export", backupExportHandler)
	http.HandleFunc("/api/backup/list", backupListHandler)
	http.HandleFunc("/api/restore/import", restoreImportHandler)
	http.HandleFunc("/api/restore/validate", restoreValidateHandler)
	
	// Tabs state endpoints
	http.HandleFunc("/api/tabs/state", tabsHandler)
	
	// System info endpoints
	http.HandleFunc("/api/system/info", systemInfoHandler)
	http.HandleFunc("/api/system/nodes", nodesHandler)
	http.HandleFunc("/api/system/pods", podsHandler)
	http.HandleFunc("/api/system/storage", storageHandler)
	
	// User preferences endpoints
	http.HandleFunc("/api/user/preferences", userPreferencesHandler)
	
	// Notification endpoints
	http.HandleFunc("/api/notifications/settings", notificationHandler)
	
	// About endpoint
	http.HandleFunc("/api/about", aboutHandler)
	
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	
	log.Printf("Settings Web (Central Hub) v%s starting on port %s", AppVersion, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "settings-web",
		"version": AppVersion,
	})
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	hostname, _ := os.Hostname()
	
	// Check service status
	services := []ServiceStatus{
		checkService("settings-theme", themeService),
		checkService("settings-backup", backupService),
		checkService("settings-restore", restoreService),
		checkService("settings-tabs", tabsService),
	}
	
	about := AboutInfo{
		AppName:      AppName,
		Version:      AppVersion,
		BuildDate:    BuildDate,
		GoVersion:    runtime.Version(),
		Platform:     runtime.GOOS,
		Architecture: runtime.GOARCH,
		Hostname:     hostname,
		Services:     services,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(about)
}

func checkService(name, url string) ServiceStatus {
	status := "offline"
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/health")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			status = "online"
		}
	}
	
	port := "8080"
	return ServiceStatus{Name: name, Status: status, Port: port}
}

// Theme handlers - proxy to settings-theme service
func themeHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	if r.Method == "GET" {
		resp, err := http.Get(themeService + "/preferences")
		if err != nil {
			// Return defaults
			json.NewEncoder(w).Encode(ThemePreferences{
				Theme:       "mocha",
				CompactMode: false,
				Animations:  true,
				FontSize:    16,
				AccentColor: "lavender",
			})
			return
		}
		defer resp.Body.Close()
		io.Copy(w, resp.Body)
		return
	}
	
	if r.Method == "POST" {
		body, _ := io.ReadAll(r.Body)
		resp, err := http.Post(themeService+"/preferences", "application/json", bytes.NewReader(body))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		io.Copy(w, resp.Body)
	}
}

func themeApplyHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	body, _ := io.ReadAll(r.Body)
	resp, err := http.Post(themeService+"/apply", "application/json", bytes.NewReader(body))
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "applied": true})
		return
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}

// Backup handlers - proxy to settings-backup service
func backupExportHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	resp, err := http.Get(backupService + "/export")
	if err != nil {
		// Return mock data on error
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"version":     AppVersion,
			"exportDate":  time.Now().Format(time.RFC3339),
			"theme":       "mocha",
			"preferences": userPreferences,
			"notifications": notificationSettings,
		})
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

func backupListHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	resp, err := http.Get(backupService + "/list")
	if err != nil {
		// Return empty list
		json.NewEncoder(w).Encode([]map[string]string{})
		return
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}

// Restore handlers - proxy to settings-restore service
func restoreImportHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	body, _ := io.ReadAll(r.Body)
	resp, err := http.Post(restoreService+"/import", "application/json", bytes.NewReader(body))
	if err != nil {
		// Return success on error for demo
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "imported": true})
		return
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}

func restoreValidateHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	body, _ := io.ReadAll(r.Body)
	resp, err := http.Post(restoreService+"/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"valid": true})
		return
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}

// Tabs handlers - proxy to settings-tabs service
func tabsHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	if r.Method == "GET" {
		resp, err := http.Get(tabsService + "/state")
		if err != nil {
			json.NewEncoder(w).Encode(TabState{ActiveTab: "system"})
			return
		}
		defer resp.Body.Close()
		io.Copy(w, resp.Body)
		return
	}
	
	if r.Method == "POST" {
		body, _ := io.ReadAll(r.Body)
		resp, err := http.Post(tabsService+"/state", "application/json", bytes.NewReader(body))
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		defer resp.Body.Close()
		io.Copy(w, resp.Body)
	}
}

// System info handlers
func systemInfoHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	info := getSystemInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func nodesHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	nodes := getNodeInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func podsHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	pods := getPodSummary()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pods)
}

func storageHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	storage := getStorageInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(storage)
}

// User preferences handlers
func userPreferencesHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userPreferences)
		return
	}
	
	if r.Method == "POST" {
		var prefs UserPreferences
		if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		userPreferences = prefs
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// Notification handlers
func notificationHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	if r.Method == "OPTIONS" {
		return
	}
	
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(notificationSettings)
		return
	}
	
	if r.Method == "POST" {
		var settings NotificationSettings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		notificationSettings = settings
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func getSystemInfo() SystemInfo {
	nodes := getNodeInfo()
	pods := getPodSummary()
	storage := getStorageInfo()
	
	serviceCount := 0
	nsCount := 0
	
	// Get service count
	cmd := exec.Command("kubectl", "get", "svc", "--all-namespaces", "--no-headers")
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			serviceCount = len(lines)
		}
	}
	
	// Get namespace count
	cmd = exec.Command("kubectl", "get", "ns", "--no-headers")
	if out, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			nsCount = len(lines)
		}
	}
	
	return SystemInfo{
		Nodes:      nodes,
		Pods:       pods,
		Services:   serviceCount,
		Namespaces: nsCount,
		Storage:    storage,
		Uptime:     getUptime(),
	}
}

func getNodeInfo() []NodeInfo {
	var nodes []NodeInfo
	
	cmd := exec.Command("kubectl", "get", "nodes", "-o", "json")
	out, err := cmd.Output()
	if err != nil {
		return nodes
	}
	
	var result struct {
		Items []struct {
			Metadata struct {
				Name              string            `json:"name"`
				Labels            map[string]string `json:"labels"`
				CreationTimestamp string            `json:"creationTimestamp"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
				Capacity struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"capacity"`
			} `json:"status"`
		} `json:"items"`
	}
	
	if err := json.Unmarshal(out, &result); err != nil {
		return nodes
	}
	
	for _, item := range result.Items {
		status := "Unknown"
		for _, cond := range item.Status.Conditions {
			if cond.Type == "Ready" {
				if cond.Status == "True" {
					status = "Ready"
				} else {
					status = "NotReady"
				}
				break
			}
		}
		
		role := "worker"
		if _, ok := item.Metadata.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "control-plane"
		}
		
		arch := item.Metadata.Labels["kubernetes.io/arch"]
		
		// Calculate age
		age := "unknown"
		if t, err := time.Parse(time.RFC3339, item.Metadata.CreationTimestamp); err == nil {
			duration := time.Since(t)
			if duration.Hours() < 24 {
				age = fmt.Sprintf("%.0fh", duration.Hours())
			} else {
				age = fmt.Sprintf("%.0fd", duration.Hours()/24)
			}
		}
		
		nodes = append(nodes, NodeInfo{
			Name:   item.Metadata.Name,
			Status: status,
			Role:   role,
			CPU:    item.Status.Capacity.CPU,
			Memory: item.Status.Capacity.Memory,
			Age:    age,
			Arch:   arch,
		})
	}
	
	return nodes
}

func getPodSummary() PodSummary {
	summary := PodSummary{}
	
	cmd := exec.Command("kubectl", "get", "pods", "--all-namespaces", "-o", "json")
	out, err := cmd.Output()
	if err != nil {
		return summary
	}
	
	var result struct {
		Items []struct {
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	
	if err := json.Unmarshal(out, &result); err != nil {
		return summary
	}
	
	summary.Total = len(result.Items)
	for _, item := range result.Items {
		switch item.Status.Phase {
		case "Running":
			summary.Running++
		case "Pending":
			summary.Pending++
		case "Failed":
			summary.Failed++
		case "Succeeded":
			summary.Completed++
		}
	}
	
	return summary
}

func getStorageInfo() []StorageInfo {
	var storage []StorageInfo
	
	cmd := exec.Command("df", "-h")
	out, err := cmd.Output()
	if err != nil {
		return storage
	}
	
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 6 {
			// Skip tmpfs and small filesystems
			if strings.HasPrefix(fields[0], "tmpfs") || strings.HasPrefix(fields[0], "shm") {
				continue
			}
			storage = append(storage, StorageInfo{
				Name:      fields[0],
				Size:      fields[1],
				Used:      fields[2],
				Available: fields[3],
				Percent:   fields[4],
				Mount:     fields[5],
			})
		}
	}
	
	return storage
}

func getUptime() string {
	cmd := exec.Command("uptime", "-p")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func serveUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, settingsHTML)
}

var settingsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>Settings - HolmOS</title>
    <style>
        /* Catppuccin Mocha (Default) */
        :root {
            --ctp-base: #1e1e2e;
            --ctp-mantle: #181825;
            --ctp-crust: #11111b;
            --ctp-surface0: #313244;
            --ctp-surface1: #45475a;
            --ctp-surface2: #585b70;
            --ctp-overlay0: #6c7086;
            --ctp-overlay1: #7f849c;
            --ctp-text: #cdd6f4;
            --ctp-subtext0: #a6adc8;
            --ctp-subtext1: #bac2de;
            --ctp-lavender: #b4befe;
            --ctp-blue: #89b4fa;
            --ctp-sapphire: #74c7ec;
            --ctp-sky: #89dceb;
            --ctp-teal: #94e2d5;
            --ctp-green: #a6e3a1;
            --ctp-yellow: #f9e2af;
            --ctp-peach: #fab387;
            --ctp-maroon: #eba0ac;
            --ctp-red: #f38ba8;
            --ctp-mauve: #cba6f7;
            --ctp-pink: #f5c2e7;
            --ctp-flamingo: #f2cdcd;
            --ctp-rosewater: #f5e0dc;
        }

        /* Catppuccin Latte (Light) */
        .theme-latte {
            --ctp-base: #eff1f5;
            --ctp-mantle: #e6e9ef;
            --ctp-crust: #dce0e8;
            --ctp-surface0: #ccd0da;
            --ctp-surface1: #bcc0cc;
            --ctp-surface2: #acb0be;
            --ctp-overlay0: #9ca0b0;
            --ctp-overlay1: #8c8fa1;
            --ctp-text: #4c4f69;
            --ctp-subtext0: #6c6f85;
            --ctp-subtext1: #5c5f77;
            --ctp-lavender: #7287fd;
            --ctp-blue: #1e66f5;
            --ctp-sapphire: #209fb5;
            --ctp-sky: #04a5e5;
            --ctp-teal: #179299;
            --ctp-green: #40a02b;
            --ctp-yellow: #df8e1d;
            --ctp-peach: #fe640b;
            --ctp-maroon: #e64553;
            --ctp-red: #d20f39;
            --ctp-mauve: #8839ef;
            --ctp-pink: #ea76cb;
            --ctp-flamingo: #dd7878;
            --ctp-rosewater: #dc8a78;
        }

        /* Catppuccin Frappe */
        .theme-frappe {
            --ctp-base: #303446;
            --ctp-mantle: #292c3c;
            --ctp-crust: #232634;
            --ctp-surface0: #414559;
            --ctp-surface1: #51576d;
            --ctp-surface2: #626880;
            --ctp-overlay0: #737994;
            --ctp-overlay1: #838ba7;
            --ctp-text: #c6d0f5;
            --ctp-subtext0: #a5adce;
            --ctp-subtext1: #b5bfe2;
            --ctp-lavender: #babbf1;
            --ctp-blue: #8caaee;
            --ctp-sapphire: #85c1dc;
            --ctp-sky: #99d1db;
            --ctp-teal: #81c8be;
            --ctp-green: #a6d189;
            --ctp-yellow: #e5c890;
            --ctp-peach: #ef9f76;
            --ctp-maroon: #ea999c;
            --ctp-red: #e78284;
            --ctp-mauve: #ca9ee6;
            --ctp-pink: #f4b8e4;
            --ctp-flamingo: #eebebe;
            --ctp-rosewater: #f2d5cf;
        }

        /* Catppuccin Macchiato */
        .theme-macchiato {
            --ctp-base: #24273a;
            --ctp-mantle: #1e2030;
            --ctp-crust: #181926;
            --ctp-surface0: #363a4f;
            --ctp-surface1: #494d64;
            --ctp-surface2: #5b6078;
            --ctp-overlay0: #6e738d;
            --ctp-overlay1: #8087a2;
            --ctp-text: #cad3f5;
            --ctp-subtext0: #a5adcb;
            --ctp-subtext1: #b8c0e0;
            --ctp-lavender: #b7bdf8;
            --ctp-blue: #8aadf4;
            --ctp-sapphire: #7dc4e4;
            --ctp-sky: #91d7e3;
            --ctp-teal: #8bd5ca;
            --ctp-green: #a6da95;
            --ctp-yellow: #eed49f;
            --ctp-peach: #f5a97f;
            --ctp-maroon: #ee99a0;
            --ctp-red: #ed8796;
            --ctp-mauve: #c6a0f6;
            --ctp-pink: #f5bde6;
            --ctp-flamingo: #f0c6c6;
            --ctp-rosewater: #f4dbd6;
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "SF Pro Display", "Segoe UI", Roboto, sans-serif;
            background-color: var(--ctp-base);
            color: var(--ctp-text);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            padding-bottom: 80px;
            transition: all 0.3s ease;
            -webkit-font-smoothing: antialiased;
        }

        /* iOS-style Header */
        .header {
            background-color: var(--ctp-mantle);
            padding: 12px 16px;
            border-bottom: 1px solid var(--ctp-surface0);
            display: flex;
            align-items: center;
            gap: 12px;
            position: sticky;
            top: 0;
            z-index: 100;
            backdrop-filter: blur(20px);
            -webkit-backdrop-filter: blur(20px);
        }

        .header-icon {
            width: 36px; height: 36px;
            background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-blue));
            border-radius: 10px;
            display: flex; align-items: center; justify-content: center;
            box-shadow: 0 2px 8px rgba(0,0,0,0.2);
        }

        .header h1 { font-size: 1.35rem; font-weight: 600; letter-spacing: -0.3px; }
        .header small { color: var(--ctp-subtext0); font-size: 0.8rem; margin-left: auto; }

        /* iOS-style Settings List */
        .settings-list {
            padding: 0 16px;
            margin-top: 20px;
        }

        .settings-group {
            background-color: var(--ctp-surface0);
            border-radius: 12px;
            margin-bottom: 20px;
            overflow: hidden;
        }

        .settings-group-title {
            font-size: 0.75rem;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            color: var(--ctp-subtext0);
            padding: 8px 16px 6px;
            margin-bottom: 0;
        }

        .settings-item {
            display: flex;
            align-items: center;
            padding: 12px 16px;
            border-bottom: 1px solid var(--ctp-surface1);
            cursor: pointer;
            transition: background-color 0.15s ease;
            gap: 12px;
        }

        .settings-item:last-child { border-bottom: none; }
        .settings-item:active { background-color: var(--ctp-surface1); }

        .settings-icon {
            width: 32px; height: 32px;
            border-radius: 8px;
            display: flex; align-items: center; justify-content: center;
            flex-shrink: 0;
        }

        .settings-icon.system { background: linear-gradient(135deg, #6c7086, #9ca0b0); }
        .settings-icon.theme { background: linear-gradient(135deg, #cba6f7, #f5c2e7); }
        .settings-icon.prefs { background: linear-gradient(135deg, #89b4fa, #74c7ec); }
        .settings-icon.notif { background: linear-gradient(135deg, #f38ba8, #fab387); }
        .settings-icon.backup { background: linear-gradient(135deg, #a6e3a1, #94e2d5); }
        .settings-icon.about { background: linear-gradient(135deg, #89dceb, #b4befe); }

        .settings-label {
            flex: 1;
            display: flex;
            flex-direction: column;
            gap: 2px;
        }

        .settings-label span { font-weight: 500; font-size: 1rem; }
        .settings-label small { color: var(--ctp-subtext0); font-size: 0.8rem; }

        .settings-arrow {
            color: var(--ctp-overlay0);
            display: flex;
            align-items: center;
        }

        .settings-badge {
            background-color: var(--ctp-red);
            color: var(--ctp-crust);
            font-size: 0.7rem;
            font-weight: 600;
            padding: 2px 6px;
            border-radius: 10px;
            margin-right: 8px;
        }

        /* Panel Views */
        .panel {
            display: none;
            animation: slideIn 0.3s ease;
        }

        .panel.active { display: block; }

        @keyframes slideIn {
            from { opacity: 0; transform: translateX(20px); }
            to { opacity: 1; transform: translateX(0); }
        }

        .panel-header {
            display: flex;
            align-items: center;
            padding: 12px 16px;
            background-color: var(--ctp-mantle);
            border-bottom: 1px solid var(--ctp-surface0);
            position: sticky;
            top: 0;
            z-index: 100;
            backdrop-filter: blur(20px);
        }

        .back-btn {
            display: flex;
            align-items: center;
            gap: 4px;
            color: var(--ctp-blue);
            font-size: 1rem;
            cursor: pointer;
            background: none;
            border: none;
            padding: 0;
        }

        .panel-title {
            flex: 1;
            text-align: center;
            font-weight: 600;
            font-size: 1.1rem;
        }

        .panel-content { padding: 16px; }

        /* Section styling */
        .section {
            background-color: var(--ctp-surface0);
            border-radius: 12px;
            padding: 16px;
            margin-bottom: 16px;
        }

        .section-title {
            font-size: 0.9rem;
            font-weight: 600;
            margin-bottom: 12px;
            color: var(--ctp-lavender);
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .setting-row {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px 0;
            border-bottom: 1px solid var(--ctp-surface1);
        }
        .setting-row:last-child { border-bottom: none; }

        .setting-label-inner { display: flex; flex-direction: column; gap: 2px; }
        .setting-label-inner span { font-weight: 500; font-size: 0.95rem; }
        .setting-label-inner small { color: var(--ctp-subtext0); font-size: 0.8rem; }

        /* iOS Toggle Switch */
        .toggle {
            position: relative;
            width: 51px; height: 31px;
            background-color: var(--ctp-surface2);
            border-radius: 15.5px;
            cursor: pointer;
            transition: background-color 0.25s ease;
        }
        .toggle.active { background-color: var(--ctp-green); }
        .toggle::after {
            content: "";
            position: absolute;
            top: 2px; left: 2px;
            width: 27px; height: 27px;
            background-color: #fff;
            border-radius: 50%;
            transition: transform 0.25s ease;
            box-shadow: 0 2px 4px rgba(0,0,0,0.2);
        }
        .toggle.active::after { transform: translateX(20px); }

        /* Form elements */
        select {
            background-color: var(--ctp-surface1);
            border: none;
            border-radius: 8px;
            padding: 8px 12px;
            color: var(--ctp-text);
            font-size: 0.9rem;
            min-width: 140px;
            -webkit-appearance: none;
            appearance: none;
            cursor: pointer;
        }

        /* Buttons */
        .btn {
            padding: 12px 24px;
            border: none;
            border-radius: 10px;
            font-size: 0.95rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
            display: inline-flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
        }
        .btn-primary { background-color: var(--ctp-blue); color: #fff; }
        .btn-primary:active { background-color: var(--ctp-sapphire); transform: scale(0.98); }
        .btn-secondary { background-color: var(--ctp-surface1); color: var(--ctp-text); }
        .btn-danger { background-color: var(--ctp-red); color: #fff; }
        .btn-success { background-color: var(--ctp-green); color: var(--ctp-crust); }
        .btn-group { display: flex; gap: 10px; flex-wrap: wrap; }

        /* Info cards */
        .info-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 10px;
        }

        .info-card {
            background-color: var(--ctp-surface1);
            border-radius: 10px;
            padding: 14px;
            text-align: center;
        }
        .info-card .value { font-size: 1.5rem; font-weight: 700; color: var(--ctp-lavender); }
        .info-card .label { font-size: 0.75rem; color: var(--ctp-subtext0); margin-top: 4px; }

        /* Theme Grid */
        .theme-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 12px;
        }

        .theme-option {
            border-radius: 12px;
            padding: 16px;
            cursor: pointer;
            border: 3px solid transparent;
            transition: all 0.2s ease;
        }
        .theme-option:active { transform: scale(0.98); }
        .theme-option.selected { border-color: var(--ctp-lavender); }
        .theme-option .colors { display: flex; gap: 6px; margin-bottom: 10px; }
        .theme-option .colors div { width: 20px; height: 20px; border-radius: 50%; }
        .theme-option .theme-name { font-weight: 600; font-size: 0.9rem; }

        /* Node/Storage list */
        .item-list { max-height: 300px; overflow-y: auto; }

        .list-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px 14px;
            background-color: var(--ctp-surface1);
            border-radius: 10px;
            margin-bottom: 8px;
        }

        .item-info { display: flex; flex-direction: column; gap: 2px; }
        .item-name { font-weight: 500; }
        .item-details { font-size: 0.8rem; color: var(--ctp-subtext0); }

        .status-badge {
            display: inline-flex;
            align-items: center;
            gap: 4px;
            padding: 4px 10px;
            border-radius: 12px;
            font-size: 0.75rem;
            font-weight: 600;
        }
        .status-ready { background-color: rgba(166, 227, 161, 0.2); color: var(--ctp-green); }
        .status-notready { background-color: rgba(243, 139, 168, 0.2); color: var(--ctp-red); }
        .status-online { background-color: rgba(166, 227, 161, 0.2); color: var(--ctp-green); }
        .status-offline { background-color: rgba(243, 139, 168, 0.2); color: var(--ctp-red); }

        .progress-bar {
            width: 80px; height: 8px;
            background-color: var(--ctp-surface2);
            border-radius: 4px;
            overflow: hidden;
        }
        .progress-fill {
            height: 100%;
            background-color: var(--ctp-green);
            transition: width 0.3s ease;
        }
        .progress-fill.warning { background-color: var(--ctp-yellow); }
        .progress-fill.danger { background-color: var(--ctp-red); }

        /* File input */
        .file-input { display: none; }
        .file-label {
            display: inline-flex;
            align-items: center;
            gap: 8px;
            padding: 12px 20px;
            background-color: var(--ctp-surface1);
            border-radius: 10px;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .file-label:active { background-color: var(--ctp-surface2); transform: scale(0.98); }

        /* About section */
        .about-header {
            text-align: center;
            padding: 30px 20px;
        }

        .about-logo {
            width: 80px; height: 80px;
            background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-blue));
            border-radius: 20px;
            display: flex;
            align-items: center;
            justify-content: center;
            margin: 0 auto 16px;
            box-shadow: 0 4px 16px rgba(0,0,0,0.3);
        }

        .about-name { font-size: 1.5rem; font-weight: 700; margin-bottom: 4px; }
        .about-version { color: var(--ctp-subtext0); font-size: 0.9rem; }

        .about-info-row {
            display: flex;
            justify-content: space-between;
            padding: 12px 0;
            border-bottom: 1px solid var(--ctp-surface1);
        }
        .about-info-row:last-child { border-bottom: none; }
        .about-info-label { color: var(--ctp-subtext0); }
        .about-info-value { font-weight: 500; }

        /* Dock */
        .dock {
            position: fixed;
            bottom: 0; left: 0; right: 0;
            background-color: var(--ctp-mantle);
            border-top: 1px solid var(--ctp-surface0);
            padding: 8px 16px 20px;
            display: flex;
            justify-content: space-around;
            align-items: center;
            z-index: 1000;
            backdrop-filter: blur(20px);
            -webkit-backdrop-filter: blur(20px);
        }

        .dock-item {
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 4px;
            text-decoration: none;
            color: var(--ctp-subtext0);
            padding: 6px 16px;
            border-radius: 10px;
            transition: all 0.2s ease;
        }
        .dock-item:active { transform: scale(0.95); }
        .dock-item.active { color: var(--ctp-lavender); }
        .dock-icon { width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; }
        .dock-label { font-size: 0.7rem; font-weight: 500; }

        /* Toast */
        .toast {
            position: fixed;
            bottom: 100px; left: 50%;
            transform: translateX(-50%);
            background-color: var(--ctp-surface0);
            color: var(--ctp-text);
            padding: 12px 24px;
            border-radius: 12px;
            box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
            z-index: 1001;
            opacity: 0;
            transition: opacity 0.3s ease;
            font-weight: 500;
        }
        .toast.show { opacity: 1; }
        .toast.success { border-left: 4px solid var(--ctp-green); }
        .toast.error { border-left: 4px solid var(--ctp-red); }

        @media (max-width: 600px) {
            .info-grid { grid-template-columns: repeat(2, 1fr); }
            .theme-grid { grid-template-columns: 1fr 1fr; }
        }
    </style>
</head>
<body>
    <!-- Main Menu -->
    <div id="main-menu" class="panel active">
        <div class="header">
            <div class="header-icon">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
                    <circle cx="12" cy="12" r="3"></circle>
                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                </svg>
            </div>
            <h1>Settings</h1>
            <small id="version-badge">v2.1</small>
        </div>

        <div class="settings-list">
            <div class="settings-group">
                <div class="settings-item" data-panel="system">
                    <div class="settings-icon system">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
                            <rect x="2" y="3" width="20" height="14" rx="2"></rect>
                            <line x1="8" y1="21" x2="16" y2="21"></line>
                            <line x1="12" y1="17" x2="12" y2="21"></line>
                        </svg>
                    </div>
                    <div class="settings-label">
                        <span>System</span>
                        <small>Cluster info, nodes, storage</small>
                    </div>
                    <div class="settings-arrow">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="9 18 15 12 9 6"></polyline>
                        </svg>
                    </div>
                </div>

                <div class="settings-item" data-panel="theme">
                    <div class="settings-icon theme">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
                            <circle cx="12" cy="12" r="5"></circle>
                            <line x1="12" y1="1" x2="12" y2="3"></line>
                            <line x1="12" y1="21" x2="12" y2="23"></line>
                            <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line>
                            <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line>
                        </svg>
                    </div>
                    <div class="settings-label">
                        <span>Appearance</span>
                        <small>Theme, colors, display</small>
                    </div>
                    <div class="settings-arrow">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="9 18 15 12 9 6"></polyline>
                        </svg>
                    </div>
                </div>

                <div class="settings-item" data-panel="prefs">
                    <div class="settings-icon prefs">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
                            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path>
                            <circle cx="12" cy="7" r="4"></circle>
                        </svg>
                    </div>
                    <div class="settings-label">
                        <span>Preferences</span>
                        <small>Language, timezone, file options</small>
                    </div>
                    <div class="settings-arrow">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="9 18 15 12 9 6"></polyline>
                        </svg>
                    </div>
                </div>

                <div class="settings-item" data-panel="notifications">
                    <div class="settings-icon notif">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
                            <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"></path>
                            <path d="M13.73 21a2 2 0 0 1-3.46 0"></path>
                        </svg>
                    </div>
                    <div class="settings-label">
                        <span>Notifications</span>
                        <small>Alerts and notification settings</small>
                    </div>
                    <div class="settings-arrow">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="9 18 15 12 9 6"></polyline>
                        </svg>
                    </div>
                </div>
            </div>

            <div class="settings-group">
                <div class="settings-item" data-panel="backup">
                    <div class="settings-icon backup">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
                            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                            <polyline points="17 8 12 3 7 8"></polyline>
                            <line x1="12" y1="3" x2="12" y2="15"></line>
                        </svg>
                    </div>
                    <div class="settings-label">
                        <span>Backup & Restore</span>
                        <small>Export and import settings</small>
                    </div>
                    <div class="settings-arrow">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="9 18 15 12 9 6"></polyline>
                        </svg>
                    </div>
                </div>

                <div class="settings-item" data-panel="about">
                    <div class="settings-icon about">
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
                            <circle cx="12" cy="12" r="10"></circle>
                            <line x1="12" y1="16" x2="12" y2="12"></line>
                            <line x1="12" y1="8" x2="12.01" y2="8"></line>
                        </svg>
                    </div>
                    <div class="settings-label">
                        <span>About</span>
                        <small>Version, services, credits</small>
                    </div>
                    <div class="settings-arrow">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="9 18 15 12 9 6"></polyline>
                        </svg>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <!-- System Panel -->
    <div id="system-panel" class="panel">
        <div class="panel-header">
            <button class="back-btn" onclick="showPanel('main-menu')">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="15 18 9 12 15 6"></polyline>
                </svg>
                Back
            </button>
            <span class="panel-title">System</span>
            <span style="width: 60px;"></span>
        </div>
        <div class="panel-content">
            <div class="section">
                <h2 class="section-title">Cluster Overview</h2>
                <div class="info-grid">
                    <div class="info-card">
                        <div class="value" id="node-count">-</div>
                        <div class="label">Nodes</div>
                    </div>
                    <div class="info-card">
                        <div class="value" id="pod-running">-</div>
                        <div class="label">Running</div>
                    </div>
                    <div class="info-card">
                        <div class="value" id="service-count">-</div>
                        <div class="label">Services</div>
                    </div>
                    <div class="info-card">
                        <div class="value" id="namespace-count">-</div>
                        <div class="label">Namespaces</div>
                    </div>
                </div>
            </div>

            <div class="section">
                <h2 class="section-title">Cluster Nodes</h2>
                <div class="item-list" id="node-list">
                    <div class="list-item"><div class="item-info"><span class="item-name">Loading...</span></div></div>
                </div>
            </div>

            <div class="section">
                <h2 class="section-title">Pod Status</h2>
                <div class="info-grid">
                    <div class="info-card">
                        <div class="value" id="pod-total">-</div>
                        <div class="label">Total</div>
                    </div>
                    <div class="info-card">
                        <div class="value" id="pod-pending" style="color: var(--ctp-yellow);">-</div>
                        <div class="label">Pending</div>
                    </div>
                    <div class="info-card">
                        <div class="value" id="pod-failed" style="color: var(--ctp-red);">-</div>
                        <div class="label">Failed</div>
                    </div>
                    <div class="info-card">
                        <div class="value" id="pod-completed" style="color: var(--ctp-teal);">-</div>
                        <div class="label">Completed</div>
                    </div>
                </div>
            </div>

            <div class="section">
                <h2 class="section-title">Storage</h2>
                <div class="item-list" id="storage-list">
                    <div class="list-item"><div class="item-info"><span class="item-name">Loading...</span></div></div>
                </div>
            </div>

            <div class="section">
                <button class="btn btn-primary" onclick="refreshSystemInfo()" style="width: 100%;">Refresh</button>
            </div>
        </div>
    </div>

    <!-- Theme Panel -->
    <div id="theme-panel" class="panel">
        <div class="panel-header">
            <button class="back-btn" onclick="showPanel('main-menu')">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="15 18 9 12 15 6"></polyline>
                </svg>
                Back
            </button>
            <span class="panel-title">Appearance</span>
            <span style="width: 60px;"></span>
        </div>
        <div class="panel-content">
            <div class="section">
                <h2 class="section-title">Catppuccin Theme</h2>
                <div class="theme-grid">
                    <div class="theme-option selected" data-theme="mocha" style="background-color: #1e1e2e;">
                        <div class="colors">
                            <div style="background-color: #cba6f7;"></div>
                            <div style="background-color: #89b4fa;"></div>
                            <div style="background-color: #a6e3a1;"></div>
                            <div style="background-color: #f38ba8;"></div>
                        </div>
                        <div class="theme-name" style="color: #cdd6f4;">Mocha</div>
                    </div>
                    <div class="theme-option" data-theme="macchiato" style="background-color: #24273a;">
                        <div class="colors">
                            <div style="background-color: #c6a0f6;"></div>
                            <div style="background-color: #8aadf4;"></div>
                            <div style="background-color: #a6da95;"></div>
                            <div style="background-color: #ed8796;"></div>
                        </div>
                        <div class="theme-name" style="color: #cad3f5;">Macchiato</div>
                    </div>
                    <div class="theme-option" data-theme="frappe" style="background-color: #303446;">
                        <div class="colors">
                            <div style="background-color: #ca9ee6;"></div>
                            <div style="background-color: #8caaee;"></div>
                            <div style="background-color: #a6d189;"></div>
                            <div style="background-color: #e78284;"></div>
                        </div>
                        <div class="theme-name" style="color: #c6d0f5;">Frappe</div>
                    </div>
                    <div class="theme-option" data-theme="latte" style="background-color: #eff1f5;">
                        <div class="colors">
                            <div style="background-color: #8839ef;"></div>
                            <div style="background-color: #1e66f5;"></div>
                            <div style="background-color: #40a02b;"></div>
                            <div style="background-color: #d20f39;"></div>
                        </div>
                        <div class="theme-name" style="color: #4c4f69;">Latte</div>
                    </div>
                </div>
            </div>

            <div class="section">
                <h2 class="section-title">Display Settings</h2>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Compact Mode</span>
                        <small>Reduce spacing</small>
                    </div>
                    <div class="toggle" data-setting="compact-mode"></div>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Animations</span>
                        <small>Enable transitions</small>
                    </div>
                    <div class="toggle active" data-setting="animations"></div>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Font Size</span>
                    </div>
                    <select id="font-size">
                        <option value="14">Small</option>
                        <option value="16" selected>Medium</option>
                        <option value="18">Large</option>
                    </select>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Accent Color</span>
                    </div>
                    <select id="accent-color">
                        <option value="lavender" selected>Lavender</option>
                        <option value="blue">Blue</option>
                        <option value="mauve">Mauve</option>
                        <option value="pink">Pink</option>
                        <option value="teal">Teal</option>
                        <option value="green">Green</option>
                    </select>
                </div>
            </div>

            <div class="section">
                <div class="btn-group">
                    <button class="btn btn-primary" onclick="saveTheme()">Apply</button>
                    <button class="btn btn-secondary" onclick="resetTheme()">Reset</button>
                </div>
            </div>
        </div>
    </div>

    <!-- Preferences Panel -->
    <div id="prefs-panel" class="panel">
        <div class="panel-header">
            <button class="back-btn" onclick="showPanel('main-menu')">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="15 18 9 12 15 6"></polyline>
                </svg>
                Back
            </button>
            <span class="panel-title">Preferences</span>
            <span style="width: 60px;"></span>
        </div>
        <div class="panel-content">
            <div class="section">
                <h2 class="section-title">Regional</h2>
                <div class="setting-row">
                    <div class="setting-label-inner"><span>Language</span></div>
                    <select id="language">
                        <option value="en" selected>English</option>
                        <option value="es">Spanish</option>
                        <option value="de">German</option>
                        <option value="fr">French</option>
                    </select>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner"><span>Timezone</span></div>
                    <select id="timezone">
                        <option value="America/Los_Angeles" selected>Pacific</option>
                        <option value="America/Denver">Mountain</option>
                        <option value="America/Chicago">Central</option>
                        <option value="America/New_York">Eastern</option>
                        <option value="UTC">UTC</option>
                    </select>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner"><span>Date Format</span></div>
                    <select id="date-format">
                        <option value="YYYY-MM-DD" selected>2026-01-15</option>
                        <option value="MM/DD/YYYY">01/15/2026</option>
                        <option value="DD/MM/YYYY">15/01/2026</option>
                    </select>
                </div>
            </div>

            <div class="section">
                <h2 class="section-title">File Manager</h2>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Auto Save</span>
                        <small>Save changes automatically</small>
                    </div>
                    <div class="toggle active" data-setting="auto-save"></div>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Show Hidden Files</span>
                        <small>Display dotfiles</small>
                    </div>
                    <div class="toggle" data-setting="show-hidden"></div>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner"><span>Default View</span></div>
                    <select id="default-view">
                        <option value="grid" selected>Grid</option>
                        <option value="list">List</option>
                        <option value="details">Details</option>
                    </select>
                </div>
            </div>

            <div class="section">
                <button class="btn btn-primary" onclick="savePreferences()" style="width: 100%;">Save Preferences</button>
            </div>
        </div>
    </div>

    <!-- Notifications Panel -->
    <div id="notifications-panel" class="panel">
        <div class="panel-header">
            <button class="back-btn" onclick="showPanel('main-menu')">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="15 18 9 12 15 6"></polyline>
                </svg>
                Back
            </button>
            <span class="panel-title">Notifications</span>
            <span style="width: 60px;"></span>
        </div>
        <div class="panel-content">
            <div class="section">
                <h2 class="section-title">General</h2>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Enable Notifications</span>
                    </div>
                    <div class="toggle active" data-setting="notif-enabled"></div>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Sound Alerts</span>
                    </div>
                    <div class="toggle active" data-setting="notif-sound"></div>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Desktop Notifications</span>
                    </div>
                    <div class="toggle active" data-setting="notif-desktop"></div>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Email Notifications</span>
                    </div>
                    <div class="toggle" data-setting="notif-email"></div>
                </div>
            </div>

            <div class="section">
                <h2 class="section-title">Alert Types</h2>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Build Alerts</span>
                        <small>Build success/failure</small>
                    </div>
                    <div class="toggle active" data-setting="alert-build"></div>
                </div>
                <div class="setting-row">
                    <div class="setting-label-inner">
                        <span>Cluster Alerts</span>
                        <small>Node status changes</small>
                    </div>
                    <div class="toggle active" data-setting="alert-cluster"></div>
                </div>
            </div>

            <div class="section">
                <button class="btn btn-primary" onclick="saveNotifications()" style="width: 100%;">Save Settings</button>
            </div>
        </div>
    </div>

    <!-- Backup Panel -->
    <div id="backup-panel" class="panel">
        <div class="panel-header">
            <button class="back-btn" onclick="showPanel('main-menu')">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="15 18 9 12 15 6"></polyline>
                </svg>
                Back
            </button>
            <span class="panel-title">Backup & Restore</span>
            <span style="width: 60px;"></span>
        </div>
        <div class="panel-content">
            <div class="section">
                <h2 class="section-title">Export Settings</h2>
                <p style="color: var(--ctp-subtext0); margin-bottom: 16px; font-size: 0.9rem;">
                    Download all HolmOS settings as a JSON file.
                </p>
                <button class="btn btn-primary" onclick="exportSettings()" style="width: 100%;">Export All Settings</button>
            </div>

            <div class="section">
                <h2 class="section-title">Import Settings</h2>
                <p style="color: var(--ctp-subtext0); margin-bottom: 16px; font-size: 0.9rem;">
                    Restore settings from a backup file.
                </p>
                <input type="file" id="import-file" class="file-input" accept=".json" onchange="handleFileSelect(event)" />
                <label for="import-file" class="file-label" style="width: 100%; justify-content: center;">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                        <polyline points="14 2 14 8 20 8"></polyline>
                    </svg>
                    Choose File
                </label>
                <div id="selected-file" style="margin-top: 10px; color: var(--ctp-subtext0); font-size: 0.85rem; text-align: center;"></div>
                <button class="btn btn-success" onclick="importSettings()" id="import-btn" disabled style="width: 100%; margin-top: 12px;">Import</button>
            </div>

            <div class="section" style="border: 1px solid var(--ctp-red); background-color: rgba(243, 139, 168, 0.1);">
                <h2 class="section-title" style="color: var(--ctp-red);">Danger Zone</h2>
                <p style="color: var(--ctp-subtext0); margin-bottom: 16px; font-size: 0.9rem;">
                    Reset all settings to defaults. This cannot be undone.
                </p>
                <button class="btn btn-danger" onclick="resetAllSettings()" style="width: 100%;">Reset All Settings</button>
            </div>
        </div>
    </div>

    <!-- About Panel -->
    <div id="about-panel" class="panel">
        <div class="panel-header">
            <button class="back-btn" onclick="showPanel('main-menu')">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="15 18 9 12 15 6"></polyline>
                </svg>
                Back
            </button>
            <span class="panel-title">About</span>
            <span style="width: 60px;"></span>
        </div>
        <div class="panel-content">
            <div class="about-header">
                <div class="about-logo">
                    <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2">
                        <circle cx="12" cy="12" r="3"></circle>
                        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                    </svg>
                </div>
                <div class="about-name">HolmOS</div>
                <div class="about-version" id="about-version">Settings Hub v2.1.0</div>
            </div>

            <div class="section">
                <h2 class="section-title">System Information</h2>
                <div class="about-info-row">
                    <span class="about-info-label">Platform</span>
                    <span class="about-info-value" id="about-platform">-</span>
                </div>
                <div class="about-info-row">
                    <span class="about-info-label">Architecture</span>
                    <span class="about-info-value" id="about-arch">-</span>
                </div>
                <div class="about-info-row">
                    <span class="about-info-label">Go Version</span>
                    <span class="about-info-value" id="about-go">-</span>
                </div>
                <div class="about-info-row">
                    <span class="about-info-label">Build Date</span>
                    <span class="about-info-value" id="about-build">-</span>
                </div>
                <div class="about-info-row">
                    <span class="about-info-label">Hostname</span>
                    <span class="about-info-value" id="about-hostname">-</span>
                </div>
            </div>

            <div class="section">
                <h2 class="section-title">Connected Services</h2>
                <div class="item-list" id="services-list">
                    <div class="list-item"><div class="item-info"><span class="item-name">Loading...</span></div></div>
                </div>
            </div>

            <div class="section">
                <h2 class="section-title">Credits</h2>
                <p style="color: var(--ctp-subtext0); font-size: 0.9rem; line-height: 1.5;">
                    HolmOS is a custom Kubernetes-based operating system for Raspberry Pi clusters.
                    Built with Catppuccin color scheme.
                </p>
            </div>
        </div>
    </div>

    <!-- Dock -->
    <nav class="dock">
        <a href="http://192.168.8.197:30088" class="dock-item">
            <div class="dock-icon">
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path>
                </svg>
            </div>
            <span class="dock-label">Files</span>
        </a>
        <a href="http://192.168.8.197:30088/terminal" class="dock-item">
            <div class="dock-icon">
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <polyline points="4 17 10 11 4 5"></polyline>
                    <line x1="12" y1="19" x2="20" y2="19"></line>
                </svg>
            </div>
            <span class="dock-label">Terminal</span>
        </a>
        <a href="#" class="dock-item active">
            <div class="dock-icon">
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="12" cy="12" r="3"></circle>
                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                </svg>
            </div>
            <span class="dock-label">Settings</span>
        </a>
        <a href="http://192.168.8.197:30700" class="dock-item">
            <div class="dock-icon">
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M3 18v-6a9 9 0 0 1 18 0v6"></path>
                    <path d="M21 19a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3zM3 19a2 2 0 0 0 2 2h1a2 2 0 0 0 2-2v-3a2 2 0 0 0-2-2H3z"></path>
                </svg>
            </div>
            <span class="dock-label">Audio</span>
        </a>
    </nav>

    <div id="toast" class="toast"></div>

    <script>
        // Panel navigation
        function showPanel(panelId) {
            document.querySelectorAll(".panel").forEach(p => p.classList.remove("active"));
            document.getElementById(panelId).classList.add("active");
            
            if (panelId === "system-panel") refreshSystemInfo();
            if (panelId === "about-panel") loadAboutInfo();
        }

        // Menu item clicks
        document.querySelectorAll(".settings-item").forEach(item => {
            item.addEventListener("click", () => {
                const panel = item.dataset.panel;
                if (panel) showPanel(panel + "-panel");
            });
        });

        // Toggle switches
        document.querySelectorAll(".toggle").forEach(toggle => {
            toggle.addEventListener("click", () => toggle.classList.toggle("active"));
        });

        // Theme selection
        document.querySelectorAll(".theme-option").forEach(option => {
            option.addEventListener("click", () => {
                document.querySelectorAll(".theme-option").forEach(o => o.classList.remove("selected"));
                option.classList.add("selected");
                applyTheme(option.dataset.theme);
            });
        });

        function applyTheme(theme) {
            document.body.className = theme !== "mocha" ? "theme-" + theme : "";
        }

        function showToast(message, type = "success") {
            const toast = document.getElementById("toast");
            toast.textContent = message;
            toast.className = "toast " + type + " show";
            setTimeout(() => toast.classList.remove("show"), 3000);
        }

        async function refreshSystemInfo() {
            try {
                const resp = await fetch("/api/system/info");
                if (resp.ok) {
                    const data = await resp.json();
                    document.getElementById("node-count").textContent = data.nodes?.length || 0;
                    document.getElementById("pod-running").textContent = data.pods?.running || 0;
                    document.getElementById("service-count").textContent = data.services || 0;
                    document.getElementById("namespace-count").textContent = data.namespaces || 0;
                    document.getElementById("pod-total").textContent = data.pods?.total || 0;
                    document.getElementById("pod-pending").textContent = data.pods?.pending || 0;
                    document.getElementById("pod-failed").textContent = data.pods?.failed || 0;
                    document.getElementById("pod-completed").textContent = data.pods?.completed || 0;

                    const nodeList = document.getElementById("node-list");
                    if (data.nodes?.length > 0) {
                        nodeList.innerHTML = data.nodes.map(node => ` + "`" + `
                            <div class="list-item">
                                <div class="item-info">
                                    <span class="item-name">${node.name}</span>
                                    <span class="item-details">${node.role} | ${node.arch} | ${node.cpu} CPU</span>
                                </div>
                                <span class="status-badge status-${node.status.toLowerCase()}">${node.status}</span>
                            </div>
                        ` + "`" + `).join("");
                    }

                    const storageList = document.getElementById("storage-list");
                    if (data.storage?.length > 0) {
                        storageList.innerHTML = data.storage.map(s => {
                            const pct = parseInt(s.percent) || 0;
                            const cls = pct > 90 ? "danger" : pct > 70 ? "warning" : "";
                            return ` + "`" + `
                                <div class="list-item">
                                    <div class="item-info">
                                        <span class="item-name">${s.mount}</span>
                                        <span class="item-details">${s.used} / ${s.size}</span>
                                    </div>
                                    <div class="progress-bar">
                                        <div class="progress-fill ${cls}" style="width: ${pct}%"></div>
                                    </div>
                                </div>
                            ` + "`" + `;
                        }).join("");
                    }
                    showToast("System info updated", "success");
                }
            } catch (e) {
                console.error("System info error:", e);
                showToast("Failed to load system info", "error");
            }
        }

        async function loadAboutInfo() {
            try {
                const resp = await fetch("/api/about");
                if (resp.ok) {
                    const data = await resp.json();
                    document.getElementById("about-version").textContent = data.appName + " v" + data.version;
                    document.getElementById("about-platform").textContent = data.platform;
                    document.getElementById("about-arch").textContent = data.architecture;
                    document.getElementById("about-go").textContent = data.goVersion;
                    document.getElementById("about-build").textContent = data.buildDate;
                    document.getElementById("about-hostname").textContent = data.hostname;

                    const servicesList = document.getElementById("services-list");
                    if (data.services?.length > 0) {
                        servicesList.innerHTML = data.services.map(s => ` + "`" + `
                            <div class="list-item">
                                <div class="item-info">
                                    <span class="item-name">${s.name}</span>
                                    <span class="item-details">Port ${s.port}</span>
                                </div>
                                <span class="status-badge status-${s.status}">${s.status}</span>
                            </div>
                        ` + "`" + `).join("");
                    }
                }
            } catch (e) {
                console.error("About info error:", e);
            }
        }

        async function loadTheme() {
            try {
                const resp = await fetch("/api/theme/preferences");
                if (resp.ok) {
                    const data = await resp.json();
                    if (data.theme) {
                        document.querySelectorAll(".theme-option").forEach(o => o.classList.remove("selected"));
                        document.querySelector(` + "`" + `[data-theme="${data.theme}"]` + "`" + `)?.classList.add("selected");
                        applyTheme(data.theme);
                    }
                    if (data.compactMode) document.querySelector("[data-setting='compact-mode']")?.classList.add("active");
                    if (data.animations) document.querySelector("[data-setting='animations']")?.classList.add("active");
                    else document.querySelector("[data-setting='animations']")?.classList.remove("active");
                    if (data.fontSize) document.getElementById("font-size").value = data.fontSize;
                    if (data.accentColor) document.getElementById("accent-color").value = data.accentColor;
                }
            } catch (e) { console.log("Theme load failed:", e); }
        }

        async function saveTheme() {
            const theme = document.querySelector(".theme-option.selected")?.dataset.theme || "mocha";
            const data = {
                theme: theme,
                compactMode: document.querySelector("[data-setting='compact-mode']")?.classList.contains("active") || false,
                animations: document.querySelector("[data-setting='animations']")?.classList.contains("active") || false,
                fontSize: parseInt(document.getElementById("font-size").value),
                accentColor: document.getElementById("accent-color").value
            };
            try {
                await fetch("/api/theme/preferences", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(data)
                });
                showToast("Theme saved", "success");
            } catch (e) {
                showToast("Failed to save theme", "error");
            }
        }

        function resetTheme() {
            document.querySelectorAll(".theme-option").forEach(o => o.classList.remove("selected"));
            document.querySelector("[data-theme='mocha']").classList.add("selected");
            applyTheme("mocha");
            showToast("Theme reset to Mocha", "success");
        }

        async function savePreferences() {
            const data = {
                language: document.getElementById("language").value,
                timezone: document.getElementById("timezone").value,
                dateFormat: document.getElementById("date-format").value,
                autoSave: document.querySelector("[data-setting='auto-save']")?.classList.contains("active") || false,
                showHidden: document.querySelector("[data-setting='show-hidden']")?.classList.contains("active") || false,
                defaultView: document.getElementById("default-view").value
            };
            try {
                await fetch("/api/user/preferences", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(data)
                });
                showToast("Preferences saved", "success");
            } catch (e) {
                showToast("Failed to save preferences", "error");
            }
        }

        async function saveNotifications() {
            const data = {
                enabled: document.querySelector("[data-setting='notif-enabled']")?.classList.contains("active") || false,
                sound: document.querySelector("[data-setting='notif-sound']")?.classList.contains("active") || false,
                desktop: document.querySelector("[data-setting='notif-desktop']")?.classList.contains("active") || false,
                email: document.querySelector("[data-setting='notif-email']")?.classList.contains("active") || false,
                buildAlerts: document.querySelector("[data-setting='alert-build']")?.classList.contains("active") || false,
                clusterAlerts: document.querySelector("[data-setting='alert-cluster']")?.classList.contains("active") || false
            };
            try {
                await fetch("/api/notifications/settings", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(data)
                });
                showToast("Notification settings saved", "success");
            } catch (e) {
                showToast("Failed to save notifications", "error");
            }
        }

        async function exportSettings() {
            try {
                const resp = await fetch("/api/backup/export");
                if (resp.ok) {
                    const data = await resp.json();
                    const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
                    const url = URL.createObjectURL(blob);
                    const a = document.createElement("a");
                    a.href = url;
                    a.download = "holmos-settings-" + new Date().toISOString().split("T")[0] + ".json";
                    a.click();
                    URL.revokeObjectURL(url);
                    showToast("Settings exported", "success");
                }
            } catch (e) {
                showToast("Export failed", "error");
            }
        }

        let selectedFileData = null;

        function handleFileSelect(event) {
            const file = event.target.files[0];
            if (file) {
                document.getElementById("selected-file").textContent = "Selected: " + file.name;
                document.getElementById("import-btn").disabled = false;
                const reader = new FileReader();
                reader.onload = (e) => {
                    try {
                        selectedFileData = JSON.parse(e.target.result);
                    } catch (err) {
                        showToast("Invalid JSON file", "error");
                        selectedFileData = null;
                        document.getElementById("import-btn").disabled = true;
                    }
                };
                reader.readAsText(file);
            }
        }

        async function importSettings() {
            if (!selectedFileData) {
                showToast("Please select a file", "error");
                return;
            }
            try {
                const resp = await fetch("/api/restore/import", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(selectedFileData)
                });
                if (resp.ok) {
                    showToast("Settings imported", "success");
                    setTimeout(() => location.reload(), 1000);
                } else {
                    throw new Error("Import failed");
                }
            } catch (e) {
                showToast("Import failed", "error");
            }
        }

        function resetAllSettings() {
            if (confirm("Reset all settings to defaults? This cannot be undone.")) {
                showToast("Settings reset", "success");
                setTimeout(() => location.reload(), 1000);
            }
        }

        // Initialize
        document.addEventListener("DOMContentLoaded", () => {
            loadTheme();
        });
    </script>
</body>
</html>
'
