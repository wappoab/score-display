package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

func main() {
	// Parse flags
	resultsDir := flag.String("results", "./results", "Path to the folder containing result files")
	port := flag.Int("port", 8080, "Port to run the server on")
	flag.Parse()

	// Validate results directory
	if _, err := os.Stat(*resultsDir); os.IsNotExist(err) {
		log.Printf("Results directory '%s' does not exist. Creating it...", *resultsDir)
		if err := os.MkdirAll(*resultsDir, 0755); err != nil {
			log.Fatalf("Failed to create results directory: %v", err)
		}
	}

	fmt.Printf("Starting Display Server on port %d...\n", *port)
	fmt.Printf("Serving results from: %s\n", *resultsDir)

	// Start mDNS discovery
	startDiscovery(*port)
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
	// In production, you might embed these or point to a specific folder.
	// For now, we assume CWD is the root of the repo (or we handle relative paths).
	// Let's assume we run from project root, so 'server/static' is correct.
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
	// Maps /results/filename.html -> resultsDir/filename.html
	resultsFs := http.FileServer(http.Dir(*resultsDir))
	http.Handle("/results/", http.StripPrefix("/results/", resultsFs))

	// 4. API: List Files
	http.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		files, err := ioutil.ReadDir(*resultsDir)
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

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}