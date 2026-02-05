# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A centralized client-server display system for Raspberry Pi kiosks and Tizen TVs. The server broadcasts synchronized timer updates and HTML content to multiple clients via WebSocket. Used for sporting events, auctions, or any multi-screen synchronized content delivery.

**Tech Stack:** Go (server + client), JavaScript (Tizen client), WebSocket, mDNS/Bonjour

## Building & Running

### Build Commands

```bash
# Server
make server              # Linux server binary → bin/server
make windows-server      # Windows server binary → bin/server.exe

# Client (Go-based)
make client              # Linux client → bin/client
make linux-arm-client    # Raspberry Pi 32-bit → bin/client-arm
make linux-arm64-client  # Raspberry Pi 64-bit → bin/client-arm64

# Client (Tizen TV)
make tizen-client        # Unsigned package → bin/client-tizen.wgt
./scripts/build_tizen_signed.sh  # Signed package via Docker → bin/client-tizen-signed.wgt

# Quick run (after building)
make run-server
make run-client

# Clean binaries
make clean
```

### Release Process

GitHub Actions automatically builds and releases on git tags:
```bash
git tag v1.0.0
git push origin v1.0.0
```
Produces: `server-windows-amd64.zip`, `client-linux-arm.tar.gz`, `client-linux-arm64.tar.gz`

### Custom Raspberry Pi Image

The `scripts/build_custom_image.sh` script creates a pre-configured Raspberry Pi OS image:
- Requires: sudo, `image/2025-12-04-raspios-trixie-arm64.img.xz`, `bin/client-arm64`
- Prompts for: WiFi SSID/password/country, Pi user password
- Outputs: `image/display-client-custom.img` (ready to flash)
- Configures: WiFi, auto-login, kiosk mode on boot

## Architecture

### Hub Pattern (Server Core)

**File:** `server/hub.go`

The Hub is the central goroutine-safe message router:

```go
type Hub struct {
    Clients    map[*Client]bool  // Connected clients
    Broadcast  chan []byte       // Broadcast to all
    Register   chan *Client      // New connections
    Unregister chan *Client      // Disconnections
    Handshake  chan *Client      // Client identification
    SendTo     chan              // Targeted messages
    State struct {
        ActiveResult string       // Current result file
    }
}
```

**Hub.Run()** event loop processes:
- `Register` - Adds clients, broadcasts updated client list
- `Unregister` - Removes clients, cleans up
- `Handshake` - Updates client metadata, re-broadcasts client list
- `Broadcast` - Sends to all clients
- `SendTo` - Sends to specific client

### WebSocket Message Flow

**File:** `server/client_conn.go`

Each client has two goroutines:

1. **ReadPump** - Receives JSON messages from client:
   - `timer_control` - Start/Pause/Reset timer
   - `handshake` - Client identification (name, ID, IP)
   - `set_result` - Broadcast result file change
   - `client_command` - Targeted commands (rename, display mode)

2. **WritePump** - Sends messages to client:
   - Ping/pong keep-alive every 54s (60s timeout)
   - 10-second write timeout per message
   - Max message size: 512 bytes

**Initial handshake sequence:**
```
Client connects → Server sends:
  1. Current timer state
  2. Active result file
  3. Display mode (defaults to "show_result")
```

### Timer Synchronization

**File:** `server/timer.go`

```go
type TimerState struct {
    Running   bool
    TimeLeft  int  // Seconds remaining
    TotalTime int  // Original duration
}
```

When running, broadcasts `timer_update` every 1 second to all clients for synchronized countdown.

### Client Discovery (mDNS)

**Files:** `server/discovery.go`, `client/discovery.go`

Server registers as: `DisplayServer._display._tcp.local.`

Go client browses for `_display._tcp` services with 5-second timeout, retries every 2 seconds until found.

**Tizen client:** Manual IP entry (no mDNS support).

### Client Architecture (Go)

**File:** `client/main.go`

Dual-process model:
1. **Discovery goroutine** - Finds server via mDNS, updates shared state
2. **Local HTTP server** (port 8081) - Serves static HTML/JS client UI
   - `/config` endpoint returns server connection details (polled by browser)
   - `/config/update` endpoint handles client name updates

**Browser supervisor:**
- Launches Chromium in kiosk mode (Linux only): `--kiosk --no-first-run --disable-infobars`
- Monitors process, auto-restarts on crash (2s delay)

**Frontend:** `client/static/index.html`
- Polls `/config` until server found
- Connects WebSocket when available
- Handles: timer updates, display mode toggle, result iframe updates

### Client Architecture (Tizen)

**Files:** `client-tizen/index.html`, `client-tizen/js/main.js`

Pure JavaScript client for Samsung Tizen TVs:
- **Configuration:** localStorage (`serverIp`, `serverPort`, `clientName`)
- **Remote control:** Return key (10009) toggles settings, Menu key (10133) opens settings
- **Settings overlay:** Remote-navigable form for server connection
- **Package format:** Zipped .wgt with `config.xml` manifest

**Key differences from Go client:**
- No mDNS (manual IP)
- No local server (runs directly in Tizen browser)
- Remote control navigation

## Configuration Files

### server.json (optional)
```json
{
  "resultsDir": "./results",  // Path to HTML result files
  "language": "sv",           // Admin UI language (en/sv)
  "port": 8080                // Server port
}
```
Override with flags: `--results`, `--port`

### client.json (auto-generated)
```json
{
  "clientName": "Vardagsrummet"  // Persistent display name
}
```
Created on first run with hostname fallback.

## HTTP Endpoints

### Server

**WebSocket:**
- `POST /ws` - Main WebSocket connection

**Admin UI:**
- `GET /admin/admin.html` - Admin dashboard
- `GET /admin/locales/{lang}.json` - Translations

**Results:**
- `GET /results/{filename}` - Serves HTML result files

**APIs:**
- `GET /api/files` - Lists available result files
- `GET /api/info` - Returns `{resultsDir, language}`

### Client (Go)

- `GET /` - Static client UI
- `GET /config` - Returns `{wsUrl, serverBaseUrl, clientName, connected}`
- `POST /config/update` - Updates client name

## Key Data Flows

### Timer Synchronization
```
Admin → WebSocket → Server → Timer.Reset(seconds)
  → Hub broadcasts timer_update every 1s
  → All clients display synchronized countdown
```

### Result Display
```
Admin selects result → Server sets Hub.State.ActiveResult
  → Hub broadcasts set_result message
  → Clients update iframe src to /results/{file}
```

### Display Mode Toggle
```
Admin clicks "Show Timer" on client X
  → Server sends display_mode message to client X (3x retry, 100ms delay)
  → Client toggles between timer overlay and result iframe
  → Server broadcasts updated client list
```

### Client Rename
```
Admin renames client → Server sends update_config message
  → Go client: POST /config/update → updates client.json
  → Tizen client: Updates localStorage
  → Client re-handshakes with new name
```

## Adding New Features

### New Message Type

1. Define handler in `server/client_conn.go` → `readPump()` switch statement
2. Add broadcast/send logic in Hub if needed
3. Implement client-side handler in `client/static/index.html` or `client-tizen/js/main.js`

### New Admin UI Feature

Edit `server/static/admin.html`:
- Add UI elements
- Send WebSocket message with new type
- Update localization files: `server/static/locales/{en,sv}.json`

### New Timer Behavior

Modify `server/timer.go`:
- `Start()` - Resume/run logic
- `Pause()` - Freeze logic
- `Reset(seconds)` - Initialization logic

## Testing Locally

1. Start server: `./bin/server` (opens admin UI at http://localhost:8080/admin/admin.html)
2. Start client: `./bin/client` (auto-discovers server, launches browser at http://localhost:8081)
3. Use admin UI to control timer and results
4. Test with multiple clients on different terminals

## Important Implementation Notes

- **WebSocket reliability:** Display mode commands sent 3x with 100ms delay for guaranteed delivery
- **Connection keep-alive:** Ping every 54s, pong deadline 60s
- **Client list sorting:** Alphabetical by name for consistent admin UI display
- **Thread safety:** All Hub state mutations use mutex locks
- **Auto-recovery:** Client browser auto-restarts on crash (kiosk mode only)
- **Status indicator:** Shows connection state in bottom-right corner, auto-hides after 5s when connected
