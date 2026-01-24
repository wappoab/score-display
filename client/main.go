package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

var (
	serverIP   string
	serverPort int
	clientName string
)

type LocalConfig struct {
	ClientName string `json:"clientName"`
}

type ConfigResponse struct {
	WsUrl         string `json:"wsUrl"`
	ServerBaseUrl string `json:"serverBaseUrl"`
	ClientName    string `json:"clientName"`
}

func loadOrInitConfig() {
	configPath := "config.json"
	data, err := os.ReadFile(configPath)
	if err == nil {
		var cfg LocalConfig
		if json.Unmarshal(data, &cfg) == nil && cfg.ClientName != "" {
			clientName = cfg.ClientName
			fmt.Printf("Loaded existing client name: %s\n", clientName)
			return
		}
	}

	// Generate new name
	hostname, _ := os.Hostname()
	clientName = "Client-" + hostname
	cfg := LocalConfig{ClientName: clientName}
	data, _ = json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath, data, 0644)
	fmt.Printf("Generated and saved new client name: %s\n", clientName)
}

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
	fmt.Println("Starting Display Client...")
	loadOrInitConfig()

	// 1. Discovery Loop
	for {
		entry, err := findServer()
		if err == nil {
			serverIP = entry.IP
			serverPort = entry.Port
			fmt.Printf("Connected to Server at %s:%d\n", serverIP, serverPort)
			break
		}
		fmt.Printf("Discovery failed: %v. Retrying in 2s...\n", err)
		time.Sleep(2 * time.Second)
	}

	// 2. Start Local Client Server
	port := 8081
	url := fmt.Sprintf("http://localhost:%d", port)
	
go func() {
		// Give the server a moment to bind
		time.Sleep(500 * time.Millisecond)
		fmt.Printf("Launching browser at %s...\n", url)
		openBrowser(url)
	}()

	fmt.Printf("Starting Local Client Server on port %d...\n", port)
	
	// Serve static files
	fs := http.FileServer(http.Dir("client/static"))
	http.Handle("/", fs)

	// Serve Config
	http.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		config := ConfigResponse{
			WsUrl:         fmt.Sprintf("ws://%s:%d/ws", serverIP, serverPort),
			ServerBaseUrl: fmt.Sprintf("http://%s:%d", serverIP, serverPort),
			ClientName:    clientName,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	})

	// Update Config
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
			// Save to disk
			cfg := LocalConfig{ClientName: clientName}
			data, _ := json.MarshalIndent(cfg, "", "  ")
			os.WriteFile("config.json", data, 0644)
			fmt.Printf("Updated client name to: %s\n", clientName)
		}
		w.WriteHeader(http.StatusOK)
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}