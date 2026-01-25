# Display System

A robust Client-Server display system designed for Raspberry Pi kiosks. It features a centralized Admin UI to control content (HTML results) and a synchronized match timer across multiple screens.

## Architecture

*   **Server:**
    *   Central control hub (Go).
    *   Hosts the Admin Dashboard.
    *   Serves result files (HTML).
    *   Broadcasts timer synchronization and display commands via WebSocket.
    *   Supports mDNS (Bonjour) for automatic discovery.
*   **Client:**
    *   Raspberry Pi application (Go).
    *   Runs in Kiosk mode (fullscreen Chromium).
    *   Automatically finds the server on the local network.
    *   Displays either the content (Results) or a high-visibility Timer overlay.
    *   Auto-recovers from crashes and connection loss.

## Prerequisites

*   **Go** (Golang 1.16+)
*   **Make**
*   **Raspberry Pi Image:** Raspberry Pi OS with Desktop (64-bit recommended).

## Building

Use the included `Makefile` to build for different platforms.

```bash
# Build Server and Client for your current OS (Linux/Mac)
make server client

# Build Server for Windows
make windows-server

# Build Client for Raspberry Pi (Linux ARM64)
make linux-arm64-client
```

Binaries are output to the `bin/` directory.

## Deployment

### 1. Server Setup

1.  Copy `bin/server` (or `server.exe`) to your server machine.
2.  Copy the `server/static` folder to the same directory (required for Admin UI).
3.  (Optional) Create a `server.json` to configure settings:
    ```json
    {
      "resultsDir": "./results",
      "language": "en",
      "port": 8080
    }
    ```
4.  Run the server:
    ```bash
    ./server
    ```
    It will automatically open the Admin UI in your browser.

### 2. Client Setup (Raspberry Pi)

#### Option A: Automatic Image Creation (Recommended)
This script creates a custom Raspberry Pi OS image that is pre-configured with WiFi, User, and the Client Application set to auto-start in Kiosk mode.

1.  Download the **Raspberry Pi OS with Desktop (64-bit)** image (`.img.xz`).
2.  Ensure you have built the client: `make linux-arm64-client`.
3.  Run the builder script:
    ```bash
    sudo ./scripts/build_custom_image.sh
    ```
    *   Follow the prompts to enter WiFi credentials and Pi password.
4.  Write the resulting `image/display-client-custom.img` to your SD card using Raspberry Pi Imager or `dd`.

#### Option B: Manual Installation
1.  Flash standard Raspberry Pi OS Desktop.
2.  Boot and configure WiFi/User.
3.  Copy `bin/client-arm64` to `/home/pi/display-client/client`.
4.  Copy `client/static` folder to `/home/pi/display-client/static`.
5.  Make executable: `chmod +x client`.
6.  Run: `./client -kiosk`.

## Usage

### Admin Dashboard
*   **Timer Control:** Start, Pause, Resume, and Reset the match timer.
*   **Results:** Select an HTML file from the `resultsDir` to display on all clients.
*   **Connected Clients:**
    *   See list of active screens.
    *   **Rename:** Click the pencil icon to give a screen a friendly name (e.g., "Lobby").
    *   **Toggle View:** Switch individual screens between "Show Timer" and "Show Result".

### Client
*   **Status Indicator:** Bottom-right corner shows connection status (Green = Connected, Red = Connecting) and current mode.
*   **Persistence:** The client saves its name to `client.json`. If you rename it in the Admin UI, it remembers the new name after reboot.

## Troubleshooting

*   **Client not finding Server:** Ensure both are on the same subnet. Check Firewall on Server (allow port 8080/UDP 5353).
*   **Browser not starting:** Ensure you are using the Desktop version of Raspberry Pi OS (not Lite).
*   **Logs:**
    *   Server logs are printed to stdout.
    *   Admin UI has a "System Logs" section (append `?debug=true` to URL to see it).
