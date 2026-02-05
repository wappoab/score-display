# Tizen Client Implementation Plan

## Overview
Port the functionality of the Raspberry Pi Display Client to a **Samsung Tizen TV Web Application**. This removes the need for an external Raspberry Pi by running the client logic directly on the TV's browser engine.

## Architecture
*   **Platform:** Tizen Web Application (HTML/CSS/JS).
*   **Compatibility:** Samsung Smart TVs (Tizen 4.0+ recommended).
*   **Core Logic:** pure JavaScript implementation replacing the Go "Backend-for-Frontend".

## Key Differences from Raspberry Pi Client

| Feature | Raspberry Pi (Go + Browser) | Tizen (Pure Web) |
| :--- | :--- | :--- |
| **Runtime** | Go Binary + Chromium | Tizen Web View (Webkit based) |
| **Discovery** | Go `zeroconf` (mDNS) | Tizen Web Device API / Manual IP |
| **Config** | `client.json` file | `localStorage` |
| **Browser Control** | Go manages process | App *is* the browser |
| **Startup** | Systemd Service | Tizen App Lifecycle |

## Feature Implementation

### 1. Project Structure
Standard Tizen Web App layout:
```text
client-tizen/
├── config.xml          # Tizen Manifest (Privileges, Version)
├── icon.png            # App Icon
├── index.html          # Main Entry Point (The Display)
└── js/
    ├── main.js         # Application Logic
    └── discovery.js    # Server Discovery (mDNS/IP)
```

### 2. Server Discovery
The Go client used UDP-based mDNS. In a browser environment (even Tizen), raw UDP access is restricted.
**Strategies:**
1.  **Manual Configuration (MVP):** On first run, show a settings screen to enter Server IP.
2.  **Subnet Scan:** Try connecting to `http://192.168.1.X:8080/api/info` on the local subnet.
3.  **Tizen API:** Investigate `webapis.network` for discovery features.

*Decision:* We will implement a **Settings Screen** that appears if no server is configured/found. This allows manual entry using the TV remote.

### 3. Connection & Display
*   Reuse the existing WebSocket logic from the Client.
*   Reuse the `timerOverlay` and `iframe` switching logic.
*   **Modifications:**
    *   Remove dependency on `/config` endpoint (local Go server).
    *   Store `clientName` and `serverUrl` in `localStorage`.

### 4. TV Remote Support
*   Need to handle remote control keys (Return/Back, Enter, Numbers) for the initial setup screen.
*   Once connected, the remote is mostly unused (passive display), but "Return" should probably exit the app or open settings.

## Development Steps
1.  **Scaffold:** Create `config.xml` and folder structure.
2.  **UI Port:** Adapt `client/static/index.html` to be standalone.
    *   Create a "Setup" overlay for entering IP.
3.  **Logic:** Implement `main.js` to handle WebSocket connection and State management.
4.  **Packaging:** Create a script to zip files into `.wgt` (Widget) format for installation.

## Future / Advanced
*   **SSSP (Samsung Smart Signage Platform):** If this is a commercial display, we might have access to more APIs. Assuming Consumer TV for now.
