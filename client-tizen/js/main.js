// Global State
let ws = null;
let config = {
    serverIp: "",
    serverPort: "8080",
    clientName: "Client-Tizen-" + Math.floor(Math.random() * 1000)
};
let isSettingsOpen = false;
let retryTimeout = null;

// Tizen Key Codes
const KEYS = {
    RETURN: 10009,
    ENTER: 13,
    MENU: 10133 // Tools key often used as menu
};

// --- Initialization ---
window.onload = function() {
    loadConfig();
    
    // Register Keys
    if (window.tizen) {
        try {
            tizen.tvinputdevice.registerKey('Return');
            tizen.tvinputdevice.registerKey('Menu');
        } catch (e) {
            console.error("Key registration failed", e);
        }
    }

    // Input Event Listener (Remote Control)
    document.body.addEventListener('keydown', handleKeyDown);

    // Initial Logic
    if (!config.serverIp) {
        openSettings("Please configure Server IP");
    } else {
        connect();
    }
};

// --- Configuration ---
function loadConfig() {
    const savedIp = localStorage.getItem('serverIp');
    const savedPort = localStorage.getItem('serverPort');
    const savedName = localStorage.getItem('clientName');

    if (savedIp) config.serverIp = savedIp;
    if (savedPort) config.serverPort = savedPort;
    if (savedName) config.clientName = savedName;

    // Pre-fill inputs
    document.getElementById('serverIp').value = config.serverIp;
    document.getElementById('serverPort').value = config.serverPort;
    document.getElementById('clientName').value = config.clientName;
}

function saveSettings() {
    const ip = document.getElementById('serverIp').value.trim();
    const port = document.getElementById('serverPort').value.trim();
    const name = document.getElementById('clientName').value.trim();

    if (!ip) {
        alert("Server IP is required");
        return;
    }

    config.serverIp = ip;
    config.serverPort = port || "8080";
    config.clientName = name || ("Client-Tizen-" + Math.floor(Math.random() * 1000));

    localStorage.setItem('serverIp', config.serverIp);
    localStorage.setItem('serverPort', config.serverPort);
    localStorage.setItem('clientName', config.clientName);

    closeSettings();
    connect(); // Reconnect with new settings
}

// --- UI Control ---
function openSettings(msg) {
    isSettingsOpen = true;
    const overlay = document.getElementById('settingsOverlay');
    overlay.classList.add('visible');
    
    // Focus the first input
    document.getElementById('serverIp').focus();
    
    if (msg) console.log("Settings opened:", msg);
}

function closeSettings() {
    if (!config.serverIp) return; // Force open if no config
    isSettingsOpen = false;
    document.getElementById('settingsOverlay').classList.remove('visible');
    document.body.focus(); // Blur inputs
}

function updateStatus(text, color) {
    const el = document.getElementById('statusIndicator');
    el.innerText = text;
    el.style.color = color;
}

// --- WebSocket Logic ---
function connect() {
    if (ws) {
        ws.close();
        ws = null;
    }
    if (retryTimeout) clearTimeout(retryTimeout);

    const wsUrl = `ws://${config.serverIp}:${config.serverPort}/ws`;
    updateStatus("Connecting to " + wsUrl + "...", "orange");
    console.log("Connecting to", wsUrl);

    try {
        ws = new WebSocket(wsUrl);

        ws.onopen = function() {
            console.log("WS Connected");
            updateStatus("Connected: " + config.clientName, "lime");
            
            // Handshake
            ws.send(JSON.stringify({ 
                type: "handshake", 
                payload: { name: config.clientName, id: config.clientName } 
            }));
            
            // Set title
            document.title = config.clientName;
            
            // Hide status after a while
            setTimeout(() => {
                if (ws && ws.readyState === WebSocket.OPEN) {
                    document.getElementById('statusIndicator').style.display = 'none';
                }
            }, 5000);
        };

        ws.onmessage = function(event) {
            const msg = JSON.parse(event.data);
            handleMessage(msg);
        };

        ws.onclose = function() {
            console.log("WS Closed");
            document.getElementById('statusIndicator').style.display = 'block';
            updateStatus("Disconnected. Retrying...", "red");
            retryTimeout = setTimeout(connect, 3000);
        };

        ws.onerror = function(e) {
            console.error("WS Error", e);
            ws.close();
        };

    } catch (e) {
        console.error("Connection failed", e);
        updateStatus("Error: " + e.message, "red");
        retryTimeout = setTimeout(connect, 3000);
    }
}

function handleMessage(msg) {
    const overlay = document.getElementById('timerOverlay');
    const iframe = document.getElementById('resultFrame');
    
    if (msg.type === "timer_update") {
        const state = msg.payload;
        const m = Math.floor(state.timeLeft / 60).toString().padStart(2, '0');
        const s = (state.timeLeft % 60).toString().padStart(2, '0');
        overlay.innerText = `${m}:${s}`;
    } else if (msg.type === "display_mode") {
        if (msg.payload === "show_timer") {
            overlay.classList.add("active");
            iframe.style.visibility = 'hidden';
            iframe.style.opacity = '0';
        } else {
            overlay.classList.remove("active");
            iframe.style.visibility = 'visible';
            iframe.style.opacity = '1';
        }
    } else if (msg.type === "set_result") {
        // Construct URL
        const url = `http://${config.serverIp}:${config.serverPort}/results/${msg.payload.file}`;
        if (iframe.src !== url) {
            iframe.src = url;
        }
    } else if (msg.type === "update_config") {
        // Handle Rename from Server
        if (msg.payload.key === "ClientName") {
            config.clientName = msg.payload.value;
            localStorage.setItem('clientName', config.clientName);
            document.title = config.clientName;
            
            // Re-handshake
            ws.send(JSON.stringify({ 
                type: "handshake", 
                payload: { name: config.clientName, id: config.clientName } 
            }));
            
            // Show status briefly
            const s = document.getElementById('statusIndicator');
            s.style.display = 'block';
            s.innerText = "Renamed to: " + config.clientName;
            setTimeout(() => s.style.display = 'none', 3000);
        }
    }
}

// --- Input Handling ---
function handleKeyDown(e) {
    console.log("Key:", e.keyCode);

    if (e.keyCode === KEYS.RETURN) {
        if (isSettingsOpen) {
            // If in settings, Return closes it (if valid config exists)
            closeSettings();
        } else {
            // If normal mode, Return opens settings
            openSettings();
        }
    }
    
    if (e.keyCode === KEYS.MENU) {
        if (!isSettingsOpen) openSettings();
    }
}
