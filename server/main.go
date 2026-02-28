package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
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

func detectHTMLCharset(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "iso-8859-1"
	}
	if len(data) > 8192 {
		data = data[:8192]
	}
	lower := bytes.ToLower(data)
	switch {
	case bytes.Contains(lower, []byte("charset=utf-8")):
		return "utf-8"
	case bytes.Contains(lower, []byte("charset=windows-1252")):
		return "windows-1252"
	case bytes.Contains(lower, []byte("charset=iso-8859-1")):
		return "iso-8859-1"
	case bytes.Contains(lower, []byte("name=generator")) && bytes.Contains(lower, []byte("content=\"ruter\"")):
		// Legacy Ruter exports are typically Latin-1.
		return "iso-8859-1"
	default:
		return "iso-8859-1"
	}
}

func detectTextCharset(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "iso-8859-1"
	}
	if len(data) > 8192 {
		data = data[:8192]
	}
	if utf8.Valid(data) {
		return "utf-8"
	}
	// Legacy exports are often Latin-1.
	return "iso-8859-1"
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
	absResultsDir, err := filepath.Abs(finalResultsDir)
	if err != nil {
		log.Fatalf("Failed to resolve results directory path: %v", err)
	}
	http.HandleFunc("/results/", func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/results/")
		rel = strings.TrimPrefix(filepath.Clean("/"+rel), "/")
		if rel == "" || rel == "." {
			http.NotFound(w, r)
			return
		}

		fullPath := filepath.Join(absResultsDir, rel)
		absPath, err := filepath.Abs(fullPath)
		if err != nil {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		sep := string(os.PathSeparator)
		if absPath != absResultsDir && !strings.HasPrefix(absPath, absResultsDir+sep) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		switch ext := strings.ToLower(filepath.Ext(absPath)); ext {
		case ".htm", ".html":
			w.Header().Set("Content-Type", "text/html; charset="+detectHTMLCharset(absPath))
		case ".txt":
			w.Header().Set("Content-Type", "text/plain; charset="+detectTextCharset(absPath))
		}

		http.ServeFile(w, r, absPath)
	})

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

	// Create HTTP server
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", finalPort),
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on port %d\n", finalPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("\nShutdown signal received, gracefully shutting down...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
