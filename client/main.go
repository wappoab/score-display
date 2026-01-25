package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

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

	hostname, _ := os.Hostname()
	clientName = "Client-" + hostname
	cfg := LocalConfig{ClientName: clientName}
	data, _ = json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath, data, 0644)
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

func browserSupervisor(url string, kiosk bool) {
	for {
		log.Println("Supervisor: Starting browser...")
		cmd, err := launchBrowser(url, kiosk)
		if err != nil {
			log.Printf("Supervisor: Failed to start browser: %v. Retrying in 5s...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if cmd != nil {
			log.Println("Supervisor: Browser running. Waiting for exit...")
			err := cmd.Wait()
			log.Printf("Supervisor: Browser exited (%v). Restarting in 2s...", err)
		} else {
			if !kiosk {
				log.Println("Supervisor: Browser launched in detached mode. Supervisor exiting.")
				return
			}
		}
		
		time.Sleep(2 * time.Second)
	}
}

func main() {
	kiosk := flag.Bool("kiosk", false, "Run in Kiosk mode (Linux/Raspberry Pi)")
	flag.Parse()

	fmt.Println("Starting Display Client...")
	fmt.Printf("Running from: %s\n", baseDir)
	loadOrInitConfig()

	// 1. Start Server Discovery in Background
	go func() {
		for {
			entry, err := findServer()
			if err == nil {
				mu.Lock()
				serverIP = entry.IP
				serverPort = entry.Port
				serverFound = true
				mu.Unlock()
				fmt.Printf("Connected to Server at %s:%d\n", serverIP, serverPort)
				break // Found it, stop scanning (or keep scanning to reconnect? For now stop)
			}
			fmt.Printf("Discovery failed: %v. Retrying in 2s...\n", err)
			time.Sleep(2 * time.Second)
		}
	}()

	// 2. Start Local Client Server immediately
	port := 8081
	url := fmt.Sprintf("http://localhost:%d", port)
	
	go browserSupervisor(url, *kiosk)

	fmt.Printf("Starting Local Client Server on port %d...\n", port)
	
	staticDir := filepath.Join(baseDir, "static")
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		if _, err := os.Stat("client/static"); err == nil {
			staticDir = "client/static"
		}
	}
	
	fmt.Printf("Serving static files from: %s\n", staticDir)
	fs := http.FileServer(http.Dir(staticDir))
	http.Handle("/", fs)

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
			clientName = newCfg.ClientName
			configPath := filepath.Join(baseDir, "client.json")
			cfg := LocalConfig{ClientName: clientName}
			data, _ := json.MarshalIndent(cfg, "", "  ")
			os.WriteFile(configPath, data, 0644)
			fmt.Printf("Updated client name to: %s\n", clientName)
		}
		w.WriteHeader(http.StatusOK)
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
