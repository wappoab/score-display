package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

//go:embed static
var staticFiles embed.FS

var (
	serverIP   string
	serverPort int
	clientName string
	baseDir    string
	serverFound bool
	mu          sync.Mutex
)

type LocalConfig struct {
	ClientName string `json:"clientName"`
}

type ConfigResponse struct {
	WsUrl         string `json:"wsUrl"`
	ServerBaseUrl string `json:"serverBaseUrl"`
	ClientName    string `json:"clientName"`
	Connected     bool   `json:"connected"`
}

func init() {
	ex, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	baseDir = filepath.Dir(ex)
}

func loadOrInitConfig() {
	configPath := filepath.Join(baseDir, "client.json")
	data, err := os.ReadFile(configPath)
	if err == nil {
		var cfg LocalConfig
		if json.Unmarshal(data, &cfg) == nil && cfg.ClientName != "" {
			clientName = cfg.ClientName
			fmt.Printf("Loaded existing client name: %s\n", clientName)
			return
		}
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Warning: Failed to get hostname: %v. Using 'unknown'", err)
		hostname = "unknown"
	}
	clientName = "Client-" + hostname
	cfg := LocalConfig{ClientName: clientName}
	data, err = json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Printf("Error: Failed to marshal config: %v", err)
		return
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		log.Printf("Error: Failed to write config file: %v", err)
		return
	}
	fmt.Printf("Generated and saved new client name: %s\n", clientName)
}

func launchBrowser(url string, kiosk bool) (*exec.Cmd, error) {
	if kiosk && runtime.GOOS == "linux" {
		browsers := []string{"chromium-browser", "chromium", "google-chrome"}
		var browserCmd string
		for _, b := range browsers {
			if _, err := exec.LookPath(b); err == nil {
				browserCmd = b
				break
			}
		}

		if browserCmd != "" {
			log.Printf("Launching Kiosk mode using %s...", browserCmd)
			args := []string{
				"--kiosk",
				"--no-first-run",
				"--no-errdialogs",
				"--disable-infobars",
				"--disable-restore-session-state",
				"--check-for-update-interval=31536000",
				"--start-maximized",
				"--enable-features=OverlayScrollbar",
				"--ozone-platform-hint=auto",
				"--password-store=basic",
				"--user-data-dir=" + os.TempDir() + "/display-client-chrome",
				url,
			}
			cmd := exec.Command(browserCmd, args...)
			err := cmd.Start()
			return cmd, err
		}
		log.Println("Chromium not found for Kiosk mode.")
	}

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
	return nil, err
}

func browserSupervisor(ctx context.Context, url string, kiosk bool) {
	var currentCmd *exec.Cmd
	for {
		select {
		case <-ctx.Done():
			log.Println("Supervisor: Shutdown requested")
			if currentCmd != nil && currentCmd.Process != nil {
				log.Println("Supervisor: Killing browser process...")
				if err := currentCmd.Process.Kill(); err != nil {
					log.Printf("Supervisor: Failed to kill process: %v", err)
				}
				// Wait with timeout to reap zombie
				done := make(chan error, 1)
				go func() {
					done <- currentCmd.Wait()
				}()
				select {
				case <-done:
					log.Println("Supervisor: Browser process cleaned up")
				case <-time.After(5 * time.Second):
					log.Println("Supervisor: Wait timeout, process may be zombie")
				}
			}
			return
		default:
		}

		log.Println("Supervisor: Starting browser...")
		cmd, err := launchBrowser(url, kiosk)
		if err != nil {
			log.Printf("Supervisor: Failed to start browser: %v. Retrying in 5s...", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		if cmd != nil {
			currentCmd = cmd
			log.Println("Supervisor: Browser running. Waiting for exit...")

			// Wait for process with context cancellation
			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			select {
			case <-ctx.Done():
				// Context cancelled, kill the process
				if cmd.Process != nil {
					log.Println("Supervisor: Killing browser due to shutdown...")
					cmd.Process.Kill()
					<-done // Wait for it to finish
				}
				return
			case err := <-done:
				log.Printf("Supervisor: Browser exited (%v). Restarting in 2s...", err)
			}
		} else {
			if !kiosk {
				log.Println("Supervisor: Browser launched in detached mode. Supervisor exiting.")
				return
			}
		}

		// Check context before sleeping
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func discoveryLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Discovery: Shutdown requested")
			return
		default:
		}

		entry, err := findServer()
		if err == nil {
			mu.Lock()
			serverIP = entry.IP
			serverPort = entry.Port
			serverFound = true
			mu.Unlock()
			fmt.Printf("Connected to Server at %s:%d\n", serverIP, serverPort)
			// Continue discovery to handle server IP changes
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
		} else {
			fmt.Printf("Discovery failed: %v. Retrying in 2s...\n", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func main() {
	kiosk := flag.Bool("kiosk", false, "Run in Kiosk mode (Linux/Raspberry Pi)")
	flag.Parse()

	fmt.Println("Starting Display Client...")
	fmt.Printf("Running from: %s\n", baseDir)
	loadOrInitConfig()

	// Setup context and signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 1. Start Server Discovery in Background
	go discoveryLoop(ctx)

	// 2. Start Local Client Server immediately
	port := 8081
	url := fmt.Sprintf("http://localhost:%d", port)

	go browserSupervisor(ctx, url, *kiosk)

	fmt.Printf("Starting Local Client Server on port %d...\n", port)

	// Try to use embedded static files first, fallback to filesystem for development
	var staticFS http.FileSystem
	staticDir := filepath.Join(baseDir, "static")
	if _, err := os.Stat(staticDir); err == nil {
		// Development mode: serve from filesystem
		fmt.Printf("Serving static files from filesystem: %s\n", staticDir)
		staticFS = http.Dir(staticDir)
	} else if _, err := os.Stat("client/static"); err == nil {
		// Development mode: serve from source directory
		fmt.Println("Serving static files from filesystem: client/static")
		staticFS = http.Dir("client/static")
	} else {
		// Production mode: use embedded files
		fmt.Println("Serving static files from embedded filesystem")
		embeddedFS, err := fs.Sub(staticFiles, "static")
		if err != nil {
			log.Fatalf("Failed to access embedded static files: %v", err)
		}
		staticFS = http.FS(embeddedFS)
	}

	http.Handle("/", http.FileServer(staticFS))

	http.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		config := ConfigResponse{
			WsUrl:         fmt.Sprintf("ws://%s:%d/ws", serverIP, serverPort),
			ServerBaseUrl: fmt.Sprintf("http://%s:%d", serverIP, serverPort),
			ClientName:    clientName,
			Connected:     serverFound,
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	})

	http.HandleFunc("/config/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var newCfg LocalConfig
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}

		if newCfg.ClientName != "" {
			mu.Lock()
			clientName = newCfg.ClientName
			mu.Unlock()
			configPath := filepath.Join(baseDir, "client.json")
			cfg := LocalConfig{ClientName: clientName}
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				log.Printf("Error: Failed to marshal config: %v", err)
				http.Error(w, "Failed to marshal config", http.StatusInternalServerError)
				return
			}
			if err := os.WriteFile(configPath, data, 0644); err != nil {
				log.Printf("Error: Failed to write config file: %v", err)
				http.Error(w, "Failed to write config file", http.StatusInternalServerError)
				return
			}
			fmt.Printf("Updated client name to: %s\n", clientName)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create HTTP server
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", port),
	}

	// Start server in goroutine
	go func() {
		log.Printf("Client server listening on port %d\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("\nShutdown signal received, gracefully shutting down...")

	// Cancel context to signal all goroutines
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Client stopped")
}
