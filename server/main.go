package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Printf("Could not open browser automatically: %v", err)
		log.Printf("Please open %s in your browser.", url)
	}
}

func main() {
	// Parse flags
	resultsDirFlag := flag.String("results", "", "Path to the folder containing result files (overrides config)")
	portFlag := flag.Int("port", 0, "Port to run the server on (overrides config)")
	flag.Parse()

	// Load Config
	finalResultsDir := "./results" // Default
	finalLanguage := "en"          // Default
	finalPort := 8080              // Default
	
	cfg, err := loadConfig("server.json")
	if err == nil {
		if cfg.ResultsDir != "" {
			finalResultsDir = cfg.ResultsDir
		}
		if cfg.Language != "" {
			finalLanguage = cfg.Language
		}
		if cfg.Port != 0 {
			finalPort = cfg.Port
		}
	}

	// Flag overrides config
	if *resultsDirFlag != "" {
		finalResultsDir = *resultsDirFlag
	}
	if *portFlag != 0 {
		finalPort = *portFlag
	}

	// Validate results directory
	if _, err := os.Stat(finalResultsDir); os.IsNotExist(err) {
		log.Printf("Results directory '%s' does not exist. Creating it...", finalResultsDir)
		if err := os.MkdirAll(finalResultsDir, 0755); err != nil {
			log.Fatalf("Failed to create results directory: %v", err)
		}
	}

	fmt.Printf("Starting Display Server on port %d...\n", finalPort)
	fmt.Printf("Serving results from: %s\n", finalResultsDir)
	fmt.Printf("Admin UI Language: %s\n", finalLanguage)

	// Start mDNS discovery
	startDiscovery(finalPort)
	defer stopDiscovery()

	// Start WebSocket Hub
	hub := NewHub()
	go hub.Run()

	// Initialize Timer Manager
	timerMgr := NewTimerManager(hub)

	// 1. WebSocket Endpoint
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, timerMgr, w, r)
	})

	// 2. Admin UI
	// Serve static files from 'server/static' mapped to /admin/
	fs := http.FileServer(http.Dir("server/static"))
	http.Handle("/admin/", http.StripPrefix("/admin/", fs))
	
	// Redirect root to admin for convenience
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/admin/admin.html", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	// 3. Results File Server
	// Maps /results/filename.html -> finalResultsDir/filename.html
	resultsFs := http.FileServer(http.Dir(finalResultsDir))
	http.Handle("/results/", http.StripPrefix("/results/", resultsFs))

	// 4. API: List Files
	http.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		files, err := ioutil.ReadDir(finalResultsDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var fileNames []string
		for _, f := range files {
			if !f.IsDir() {
				fileNames = append(fileNames, f.Name())
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fileNames)
	})

	// 5. API: Server Info
	http.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			ResultsDir string `json:"resultsDir"`
			Language   string `json:"language"`
		}{
			ResultsDir: finalResultsDir,
			Language:   finalLanguage,
		})
	})

	// Open Browser
	go func() {
		// Give the server a moment to bind
		time.Sleep(500 * time.Millisecond)
		url := fmt.Sprintf("http://localhost:%d/admin/admin.html", finalPort)
		fmt.Printf("Launching browser at %s...\n", url)
		openBrowser(url)
	}()

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", finalPort), nil))
}
