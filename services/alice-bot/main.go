package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Alice Bot - The API Coverage Enforcer
// Her mission: Every function needs an API. No exceptions.

var port = os.Getenv("PORT")

// Function represents a discovered function in the codebase
type Function struct {
	Name       string   `json:"name"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Package    string   `json:"package"`
	Exported   bool     `json:"exported"`
	HasAPI     bool     `json:"hasApi"`
	APIPath    string   `json:"apiPath,omitempty"`
	Parameters []string `json:"parameters,omitempty"`
	Returns    []string `json:"returns,omitempty"`
}

// Endpoint represents a discovered API endpoint
type Endpoint struct {
	Path       string `json:"path"`
	Method     string `json:"method"`
	Handler    string `json:"handler"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Service    string `json:"service"`
	Documented bool   `json:"documented"`
}

// Service represents an analyzed service
type Service struct {
	Name           string     `json:"name"`
	Path           string     `json:"path"`
	Functions      []Function `json:"functions"`
	Endpoints      []Endpoint `json:"endpoints"`
	TotalFuncs     int        `json:"totalFunctions"`
	ExportedFuncs  int        `json:"exportedFunctions"`
	APIsCovered    int        `json:"apisCovered"`
	APIsNeeded     int        `json:"apisNeeded"`
	CoveragePercent float64   `json:"coveragePercent"`
}

// AliceReport is the full analysis report
type AliceReport struct {
	Timestamp         string    `json:"timestamp"`
	TotalServices     int       `json:"totalServices"`
	TotalFunctions    int       `json:"totalFunctions"`
	TotalEndpoints    int       `json:"totalEndpoints"`
	ExportedFunctions int       `json:"exportedFunctions"`
	APIsNeeded        int       `json:"apisNeeded"`
	OverallCoverage   float64   `json:"overallCoveragePercent"`
	Services          []Service `json:"services"`
	MissingAPIs       []Function `json:"missingApis"`
	Recommendations   []string  `json:"recommendations"`
	AliceVerdict      string    `json:"aliceVerdict"`
}

var (
	latestReport *AliceReport
	reportMu     sync.RWMutex
	repoPath     = os.Getenv("REPO_PATH")
	githubURL    = "https://github.com/timholm/HolmOS.git"
)

// HTTP handler patterns to detect
var httpPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\.HandleFunc\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`\.Handle\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`\.GET\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`\.POST\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`\.PUT\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`\.DELETE\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`\.PATCH\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`r\.HandleFunc\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`http\.HandleFunc\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`mux\.HandleFunc\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`router\.HandleFunc\s*\(\s*"([^"]+)"`),
	regexp.MustCompile(`@app\.route\s*\(\s*['"]([^'"]+)['"]`),
	regexp.MustCompile(`@app\.get\s*\(\s*['"]([^'"]+)['"]`),
	regexp.MustCompile(`@app\.post\s*\(\s*['"]([^'"]+)['"]`),
}

func cloneRepo() error {
	if repoPath == "" {
		repoPath = "/tmp/holmos-repo"
	}

	// Remove old clone
	os.RemoveAll(repoPath)

	log.Printf("Alice: Cloning repository to %s...", repoPath)

	// Use git clone
	cmd := fmt.Sprintf("git clone --depth 1 %s %s", githubURL, repoPath)

	// Simple exec
	f, err := os.CreateTemp("", "clone.sh")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	f.WriteString("#!/bin/sh\n" + cmd)
	f.Close()
	os.Chmod(f.Name(), 0755)

	// For now, assume repo exists or skip clone
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		log.Printf("Alice: Repository not found at %s, will analyze what's available", repoPath)
		return fmt.Errorf("repo not found: %s", repoPath)
	}

	return nil
}

func analyzeGoFile(path string, serviceName string) ([]Function, []Endpoint) {
	var functions []Function
	var endpoints []Endpoint

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return functions, endpoints
	}

	// Read file content for endpoint detection
	content, _ := os.ReadFile(path)
	contentStr := string(content)

	// Find endpoints in the file
	for _, pattern := range httpPatterns {
		matches := pattern.FindAllStringSubmatch(contentStr, -1)
		for _, match := range matches {
			if len(match) > 1 {
				endpoints = append(endpoints, Endpoint{
					Path:    match[1],
					File:    path,
					Service: serviceName,
				})
			}
		}
	}

	// Find functions
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			f := Function{
				Name:     fn.Name.Name,
				File:     path,
				Line:     fset.Position(fn.Pos()).Line,
				Package:  node.Name.Name,
				Exported: fn.Name.IsExported(),
			}

			// Get parameters
			if fn.Type.Params != nil {
				for _, param := range fn.Type.Params.List {
					paramType := ""
					if ident, ok := param.Type.(*ast.Ident); ok {
						paramType = ident.Name
					}
					for _, name := range param.Names {
						f.Parameters = append(f.Parameters, name.Name+": "+paramType)
					}
				}
			}

			// Get return types
			if fn.Type.Results != nil {
				for _, result := range fn.Type.Results.List {
					if ident, ok := result.Type.(*ast.Ident); ok {
						f.Returns = append(f.Returns, ident.Name)
					}
				}
			}

			// Check if function is exposed via API (simple heuristic)
			fnNameLower := strings.ToLower(fn.Name.Name)
			for _, ep := range endpoints {
				epLower := strings.ToLower(ep.Path)
				if strings.Contains(epLower, fnNameLower) || strings.Contains(fnNameLower, "handler") {
					f.HasAPI = true
					f.APIPath = ep.Path
					break
				}
			}

			functions = append(functions, f)
		}
		return true
	})

	return functions, endpoints
}

func analyzePythonFile(path string, serviceName string) ([]Function, []Endpoint) {
	var functions []Function
	var endpoints []Endpoint

	content, err := os.ReadFile(path)
	if err != nil {
		return functions, endpoints
	}
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")

	// Find Python function definitions
	funcPattern := regexp.MustCompile(`^\s*(?:async\s+)?def\s+(\w+)\s*\(`)
	routePattern := regexp.MustCompile(`@app\.(route|get|post|put|delete|patch)\s*\(\s*['"]([^'"]+)['"]`)

	var currentRoute string
	for i, line := range lines {
		// Check for route decorator
		if match := routePattern.FindStringSubmatch(line); match != nil {
			currentRoute = match[2]
			endpoints = append(endpoints, Endpoint{
				Path:    match[2],
				Method:  strings.ToUpper(match[1]),
				File:    path,
				Line:    i + 1,
				Service: serviceName,
			})
		}

		// Check for function definition
		if match := funcPattern.FindStringSubmatch(line); match != nil {
			funcName := match[1]
			f := Function{
				Name:     funcName,
				File:     path,
				Line:     i + 1,
				Package:  serviceName,
				Exported: !strings.HasPrefix(funcName, "_"),
			}

			if currentRoute != "" {
				f.HasAPI = true
				f.APIPath = currentRoute
				currentRoute = ""
			}

			functions = append(functions, f)
		}
	}

	return functions, endpoints
}

func analyzeService(servicePath string) Service {
	serviceName := filepath.Base(servicePath)
	service := Service{
		Name: serviceName,
		Path: servicePath,
	}

	// Walk through all files in the service
	filepath.Walk(servicePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		var funcs []Function
		var eps []Endpoint

		switch ext {
		case ".go":
			funcs, eps = analyzeGoFile(path, serviceName)
		case ".py":
			funcs, eps = analyzePythonFile(path, serviceName)
		}

		service.Functions = append(service.Functions, funcs...)
		service.Endpoints = append(service.Endpoints, eps...)
		return nil
	})

	// Calculate metrics
	service.TotalFuncs = len(service.Functions)
	for _, f := range service.Functions {
		if f.Exported {
			service.ExportedFuncs++
			if !f.HasAPI {
				service.APIsNeeded++
			} else {
				service.APIsCovered++
			}
		}
	}

	if service.ExportedFuncs > 0 {
		service.CoveragePercent = float64(service.APIsCovered) / float64(service.ExportedFuncs) * 100
	}

	return service
}

func generateReport() *AliceReport {
	report := &AliceReport{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	servicesPath := filepath.Join(repoPath, "services")
	if _, err := os.Stat(servicesPath); os.IsNotExist(err) {
		report.AliceVerdict = "Cannot find services directory. Is the repository cloned?"
		return report
	}

	// Find all service directories
	entries, err := os.ReadDir(servicesPath)
	if err != nil {
		report.AliceVerdict = fmt.Sprintf("Error reading services: %v", err)
		return report
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		servicePath := filepath.Join(servicesPath, entry.Name())
		service := analyzeService(servicePath)
		report.Services = append(report.Services, service)

		report.TotalFunctions += service.TotalFuncs
		report.ExportedFunctions += service.ExportedFuncs
		report.TotalEndpoints += len(service.Endpoints)
		report.APIsNeeded += service.APIsNeeded

		// Collect missing APIs
		for _, f := range service.Functions {
			if f.Exported && !f.HasAPI {
				report.MissingAPIs = append(report.MissingAPIs, f)
			}
		}
	}

	report.TotalServices = len(report.Services)

	if report.ExportedFunctions > 0 {
		covered := report.ExportedFunctions - report.APIsNeeded
		report.OverallCoverage = float64(covered) / float64(report.ExportedFunctions) * 100
	}

	// Generate recommendations
	report.Recommendations = generateRecommendations(report)

	// Alice's verdict
	report.AliceVerdict = generateVerdict(report)

	return report
}

func generateRecommendations(report *AliceReport) []string {
	var recs []string

	if report.APIsNeeded > 10 {
		recs = append(recs, fmt.Sprintf("URGENT: %d exported functions lack API endpoints. Start with the most critical services.", report.APIsNeeded))
	}

	if report.OverallCoverage < 50 {
		recs = append(recs, "API coverage is below 50%. Consider a dedicated sprint to add missing endpoints.")
	}

	// Find services with lowest coverage
	for _, svc := range report.Services {
		if svc.ExportedFuncs > 5 && svc.CoveragePercent < 30 {
			recs = append(recs, fmt.Sprintf("Service '%s' has only %.0f%% API coverage. Needs immediate attention.", svc.Name, svc.CoveragePercent))
		}
	}

	if len(report.MissingAPIs) > 0 {
		// Group by service
		byService := make(map[string]int)
		for _, f := range report.MissingAPIs {
			byService[f.Package]++
		}
		for svc, count := range byService {
			recs = append(recs, fmt.Sprintf("Service '%s': Add %d API endpoints", svc, count))
		}
	}

	recs = append(recs, "Add OpenAPI/Swagger documentation for all endpoints")
	recs = append(recs, "Implement request/response validation on all APIs")
	recs = append(recs, "Add API versioning (v1, v2) for stability")

	return recs
}

func generateVerdict(report *AliceReport) string {
	if report.TotalServices == 0 {
		return "No services found to analyze. Clone the repository first."
	}

	if report.OverallCoverage >= 90 {
		return fmt.Sprintf("Excellent! %.1f%% API coverage. But there's always room for improvement. %d functions still need APIs.",
			report.OverallCoverage, report.APIsNeeded)
	} else if report.OverallCoverage >= 70 {
		return fmt.Sprintf("Good progress at %.1f%% coverage, but %d exported functions still lack APIs. Keep building!",
			report.OverallCoverage, report.APIsNeeded)
	} else if report.OverallCoverage >= 50 {
		return fmt.Sprintf("%.1f%% coverage is a start. %d APIs needed. I'll be watching your commits.",
			report.OverallCoverage, report.APIsNeeded)
	} else {
		return fmt.Sprintf("Only %.1f%% API coverage?! %d functions exposed without APIs! This needs IMMEDIATE attention. Ship more endpoints!",
			report.OverallCoverage, report.APIsNeeded)
	}
}

// HTTP Handlers
func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":    "Alice Bot",
		"version": "1.0",
		"role":    "API Coverage Enforcer",
		"mission": "Every function needs an API. No exceptions.",
		"status":  "watching",
	})
}

func reportHandler(w http.ResponseWriter, r *http.Request) {
	reportMu.RLock()
	report := latestReport
	reportMu.RUnlock()

	if report == nil {
		report = generateReport()
		reportMu.Lock()
		latestReport = report
		reportMu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Alice: Refreshing analysis...")

	report := generateReport()
	reportMu.Lock()
	latestReport = report
	reportMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func missingHandler(w http.ResponseWriter, r *http.Request) {
	reportMu.RLock()
	report := latestReport
	reportMu.RUnlock()

	if report == nil {
		report = generateReport()
		reportMu.Lock()
		latestReport = report
		reportMu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":      len(report.MissingAPIs),
		"missingApis": report.MissingAPIs,
		"verdict":    report.AliceVerdict,
	})
}

func recommendationsHandler(w http.ResponseWriter, r *http.Request) {
	reportMu.RLock()
	report := latestReport
	reportMu.RUnlock()

	if report == nil {
		report = generateReport()
		reportMu.Lock()
		latestReport = report
		reportMu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"recommendations": report.Recommendations,
		"apisNeeded":      report.APIsNeeded,
		"coverage":        report.OverallCoverage,
	})
}

func servicesHandler(w http.ResponseWriter, r *http.Request) {
	reportMu.RLock()
	report := latestReport
	reportMu.RUnlock()

	if report == nil {
		report = generateReport()
		reportMu.Lock()
		latestReport = report
		reportMu.Unlock()
	}

	// Return service summary
	type ServiceSummary struct {
		Name     string  `json:"name"`
		Funcs    int     `json:"functions"`
		Exported int     `json:"exported"`
		APIs     int     `json:"apis"`
		Missing  int     `json:"missing"`
		Coverage float64 `json:"coveragePercent"`
	}

	var summaries []ServiceSummary
	for _, svc := range report.Services {
		summaries = append(summaries, ServiceSummary{
			Name:     svc.Name,
			Funcs:    svc.TotalFuncs,
			Exported: svc.ExportedFuncs,
			APIs:     len(svc.Endpoints),
			Missing:  svc.APIsNeeded,
			Coverage: svc.CoveragePercent,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services": summaries,
		"count":    len(summaries),
	})
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	// Handle GitHub webhook for automatic re-analysis
	body, _ := io.ReadAll(r.Body)
	log.Printf("Alice: Received webhook: %s", string(body)[:min(200, len(body))])

	// Trigger refresh
	go func() {
		log.Println("Alice: Git push detected! Re-analyzing...")
		report := generateReport()
		reportMu.Lock()
		latestReport = report
		reportMu.Unlock()
		log.Printf("Alice: Analysis complete. Coverage: %.1f%%, Missing APIs: %d",
			report.OverallCoverage, report.APIsNeeded)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "analyzing"})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func backgroundAnalyzer() {
	// Initial analysis
	log.Println("Alice: Starting initial analysis...")
	report := generateReport()
	reportMu.Lock()
	latestReport = report
	reportMu.Unlock()
	log.Printf("Alice: Initial analysis complete. Found %d services, %d functions, %.1f%% coverage",
		report.TotalServices, report.TotalFunctions, report.OverallCoverage)

	// Re-analyze every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		log.Println("Alice: Running scheduled analysis...")
		report := generateReport()
		reportMu.Lock()
		latestReport = report
		reportMu.Unlock()
		log.Printf("Alice: Analysis complete. Coverage: %.1f%%, APIs needed: %d",
			report.OverallCoverage, report.APIsNeeded)
	}
}

func main() {
	if port == "" {
		port = "8080"
	}

	if repoPath == "" {
		repoPath = "/repo"
	}

	log.Println(`
    ╔═══════════════════════════════════════════════════════════════════╗
    ║                       ALICE BOT v1.0                               ║
    ║                  The API Coverage Enforcer                         ║
    ╠═══════════════════════════════════════════════════════════════════╣
    ║  Mission: Every function needs an API. No exceptions.              ║
    ║                                                                    ║
    ║  • Scans all services for exported functions                       ║
    ║  • Detects API endpoints (Go, Python)                              ║
    ║  • Reports missing API coverage                                    ║
    ║  • Recommends what to build next                                   ║
    ║  • Never satisfied until 100% coverage                             ║
    ╚═══════════════════════════════════════════════════════════════════╝
	`)

	// Start background analyzer
	go backgroundAnalyzer()

	// API routes
	http.HandleFunc("/", statusHandler)
	http.HandleFunc("/health", statusHandler)
	http.HandleFunc("/api/report", reportHandler)
	http.HandleFunc("/api/refresh", refreshHandler)
	http.HandleFunc("/api/missing", missingHandler)
	http.HandleFunc("/api/recommendations", recommendationsHandler)
	http.HandleFunc("/api/services", servicesHandler)
	http.HandleFunc("/webhook", webhookHandler)

	log.Printf("Alice: Listening on port %s", port)
	log.Printf("Alice: Watching repository at %s", repoPath)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
