# Implementation Plan

## Overview
This project consists of a Client-Server architecture designed to display content on Raspberry Pi devices, controlled by a central Admin Dashboard.

*   **Server:** Central control plane, static file host, and Admin UI.
*   **Client:** Smart Go application on Raspberry Pi, controlling a browser to display results and synchronized timers.

## Architecture

### 1. Server Application
*   **Language:** Go (Golang).
*   **OS Support:** Cross-platform (Linux, Windows, macOS).
*   **Components:**
    *   **Admin Dashboard (Web UI):**
        *   **File Selector:** Lists files in the configured `results_folder`. Allows selecting one to be the "active" result.
        *   **Timer Controls:** Input for seconds, [Play], [Pause], [Reset] buttons.
        *   **Client Grid:**
            *   Displays connected clients as "cards".
            *   Shows Client Name (editable/assignable).
            *   **Per-Client Controls:** Toggle between "Show Timer" mode and "Show Result" mode.
    *   **Static File Host:** Serves the selected result files.
    *   **Service Discovery:** Broadcasts presence using mDNS (Bonjour).
    *   **WebSocket Hub:**
        *   Streams Timer data to clients.
        *   Sends "State Updates" (Switch to URL X, Switch to Timer, etc.) to clients.
        *   Receives Heartbeats/Handshakes from clients.

### 2. Client Application
*   **Language:** Go (Golang).
*   **Hardware:** Raspberry Pi.
*   **Architecture Pattern:** **Local Container (Reverse Proxy/Wrapper)**.
*   **Key Features:**
    *   **Auto-Discovery:** Finds Server via mDNS.
    *   **Identity:** Remembers its assigned name.
    *   **View Modes:**
        *   **Result Mode:** Shows an iframe loaded with the URL provided by the Server.
        *   **Timer Mode:** Shows a high-visibility countdown timer (synchronized via WebSocket).
    *   **Hybrid/Overlay:** Ideally, the timer can overlay the result, or they can be mutually exclusive views, controlled by the Server.

## Data Flow

1.  **Admin Action: Select Result File**
    *   Admin clicks "match_1_final.html" in Server UI.
    *   Server stores this as the `ActiveResultURL`.
    *   Server broadcasts update to Clients in "Result Mode": "Reload iframe with `match_1_final.html`".

2.  **Admin Action: Start Timer**
    *   Admin enters "300" (5 mins) and clicks [Play].
    *   Server starts internal ticker.
    *   Server broadcasts "Timer: 300, State: Running" to all Clients.
    *   Clients in "Timer Mode" display the countdown.

3.  **Admin Action: Manage Client**
    *   Client "Pi-1" connects. Shows up in Admin Grid.
    *   Admin renames "Pi-1" to "Red Corner Monitor".
    *   Admin clicks "Show Timer" on "Red Corner Monitor".
    *   "Red Corner Monitor" switches to Timer View.

## Development Phases

1.  **Project Init**: Server/Client directories, Go mods, Makefile.
2.  **Server - Core**: HTTP Server, Static Host (Results), Admin UI Host.
3.  **Discovery (mDNS)**: Implement Server broadcast and Client listener.
4.  **Communication**: WebSocket Hub (Server) and Client connection logic.
5.  **Server - Admin UI**: Build the HTML/JS Dashboard for the operator.
6.  **Client - Display**: Implement the "Container Page" with iframe/timer toggling.
7.  **Timer Logic**: Synchronized timer implementation.

## Next Steps
*   Create `server` and `client` directories.
*   Initialize Go modules.
*   Create Makefile.