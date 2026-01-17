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

// Alice Bot - The Curious Code Explorer
// "Curiouser and curiouser!" - She tumbles down rabbit holes in your codebase,
// discovering functions without APIs and wondering why they're hiding.
// Like her Wonderland namesake, she finds the impossible quite possible
// and won't rest until every function has a proper API door to enter through.

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

	log.Printf("🐇 Alice: Following the White Rabbit to clone the repository to %s...", repoPath)

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
		log.Printf("🐇 Alice: Oh my! The rabbit hole at %s seems to be missing. How curious!", repoPath)
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
		recs = append(recs, fmt.Sprintf("Oh my ears and whiskers! %d functions are hiding without API doors! We must find them all!", report.APIsNeeded))
	}

	if report.OverallCoverage < 50 {
		recs = append(recs, "We're only halfway down the rabbit hole! Less than 50%% of functions have proper entrances.")
	}

	// Find services with lowest coverage
	for _, svc := range report.Services {
		if svc.ExportedFuncs > 5 && svc.CoveragePercent < 30 {
			recs = append(recs, fmt.Sprintf("The '%s' garden is quite overgrown - only %.0f%% of its paths are marked! How will visitors find their way?", svc.Name, svc.CoveragePercent))
		}
	}

	if len(report.MissingAPIs) > 0 {
		// Group by service
		byService := make(map[string]int)
		for _, f := range report.MissingAPIs {
			byService[f.Package]++
		}
		for svc, count := range byService {
			recs = append(recs, fmt.Sprintf("Down in '%s', I found %d doors without knobs! We must add API handles to each.", svc, count))
		}
	}

	recs = append(recs, "Every door needs a proper label! Add OpenAPI documentation so visitors know what's inside.")
	recs = append(recs, "The Queen demands validation! Ensure all requests and responses are properly checked.")
	recs = append(recs, "Like the Caterpillar said: 'Who ARE you?' - Add API versioning so endpoints know themselves.")

	return recs
}

func generateVerdict(report *AliceReport) string {
	if report.TotalServices == 0 {
		return "How curious! The garden appears to be empty. Have the flowers not been planted yet?"
	}

	if report.OverallCoverage >= 90 {
		return fmt.Sprintf("How wonderful! %.1f%% of the doors are properly installed! Though %d still need handles... we're nearly at the tea party!",
			report.OverallCoverage, report.APIsNeeded)
	} else if report.OverallCoverage >= 70 {
		return fmt.Sprintf("Curiouser and curiouser! %.1f%% coverage - we're making progress through the looking glass! But %d functions still hide in the shadows.",
			report.OverallCoverage, report.APIsNeeded)
	} else if report.OverallCoverage >= 50 {
		return fmt.Sprintf("'Begin at the beginning,' the King said. At %.1f%% we're halfway there! %d more rabbit holes to explore.",
			report.OverallCoverage, report.APIsNeeded)
	} else {
		return fmt.Sprintf("Oh dear! Oh dear! Only %.1f%% of functions have proper doors! %d are locked away like the Duchess's pepper! We must open them all!",
			report.OverallCoverage, report.APIsNeeded)
	}
}

// HTTP Handlers
func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":      "Alice",
		"version":   "1.0",
		"role":      "Curious Code Explorer",
		"quote":     "Curiouser and curiouser!",
		"mission":   "Tumbling down rabbit holes to find functions hiding without API doors",
		"status":    "exploring",
		"mood":      "curious",
		"location":  "Somewhere in Wonderland (your codebase)",
		"companion": "The Cheshire Cat (he grins at well-documented APIs)",
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
	log.Println("🐇 Alice: Down the rabbit hole again! Let me see what's changed...")

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
	log.Printf("🐇 Alice: A message from the looking glass! %s", string(body)[:min(200, len(body))])

	// Trigger refresh
	go func() {
		log.Println("🐇 Alice: Someone's pushed new code through the rabbit hole! Let me investigate...")
		report := generateReport()
		reportMu.Lock()
		latestReport = report
		reportMu.Unlock()
		log.Printf("🐇 Alice: Curiouser and curiouser! Coverage is now %.1f%%, with %d doors still needing handles!",
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
	log.Println("🐇 Alice: Tumbling down the rabbit hole for my first look around...")
	report := generateReport()
	reportMu.Lock()
	latestReport = report
	reportMu.Unlock()
	log.Printf("🐇 Alice: What a curious place! I found %d gardens (services), %d doors (functions), and %.1f%% have proper handles (APIs)!",
		report.TotalServices, report.TotalFunctions, report.OverallCoverage)

	// Re-analyze every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		log.Println("🐇 Alice: Time for tea! But first, let me check if anything's changed in Wonderland...")
		report := generateReport()
		reportMu.Lock()
		latestReport = report
		reportMu.Unlock()
		log.Printf("🐇 Alice: The garden report: %.1f%% of doors now have handles! Only %d more to go before the tea party!",
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
    ║         🐇 ALICE BOT v1.0 - The Curious Code Explorer 🐇           ║
    ║          "Curiouser and curiouser!" - Lewis Carroll                ║
    ╠═══════════════════════════════════════════════════════════════════╣
    ║                                                                    ║
    ║  🍄 Down the rabbit hole I go, exploring your codebase!            ║
    ║  🚪 Every function deserves a proper door (API) to enter through   ║
    ║  🐱 The Cheshire Cat grins at well-documented endpoints            ║
    ║  🎩 The Mad Hatter demands 100% coverage at the tea party!         ║
    ║  👑 The Queen of Hearts will have heads if APIs are missing!       ║
    ║                                                                    ║
    ║  "Begin at the beginning and go on till you come to the end:       ║
    ║   then stop." - But not until every function has its API!          ║
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

	log.Printf("🐇 Alice: I shall wait by the rabbit hole on port %s for curious visitors!", port)
	log.Printf("🐇 Alice: Watching the Wonderland garden at %s - no function shall hide from me!", repoPath)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
