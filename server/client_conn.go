package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

// isPrivateIP checks if an IP address is in a private range
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	// Check private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return false
}

func splitHostPortSafe(s string) string {
	host, _, err := net.SplitHostPort(s)
	if err == nil {
		return host
	}
	return s
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// No origin header - allow (some clients don't send it)
			return true
		}

		u, err := url.Parse(origin)
		if err != nil {
			log.Printf("Rejected WebSocket connection with invalid origin %q: %v", origin, err)
			return false
		}
		originHost := u.Hostname()
		if originHost == "" {
			log.Printf("Rejected WebSocket connection with empty origin host: %s", origin)
			return false
		}
		requestHost := splitHostPortSafe(r.Host)

		// Always allow same-host origin (covers LAN hostnames / .local names).
		if strings.EqualFold(originHost, requestHost) {
			return true
		}

		// Allow localhost
		if originHost == "localhost" || originHost == "127.0.0.1" || originHost == "::1" {
			return true
		}

		// Check if it's a private IP
		ip := net.ParseIP(originHost)
		if ip != nil && isPrivateIP(ip) {
			return true
		}

		// Reject all other origins
		log.Printf("Rejected WebSocket connection from origin: %s", origin)
		return false
	},
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Client) readPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		
		// Handle incoming messages
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Invalid JSON: %v", err)
			continue
		}

		switch msg.Type {
		case "timer_control":
			var payload struct {
				Action  string `json:"action"`
				Seconds int    `json:"seconds"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				if payload.Action == "start" {
					c.TimerMgr.Start()
				} else if payload.Action == "pause" {
					c.TimerMgr.Pause()
				} else if payload.Action == "reset" {
					c.TimerMgr.Reset(payload.Seconds)
				}
			}
		case "handshake":
			var payload struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				c.Name = payload.Name
				c.ID = payload.ID
				c.Hub.Handshake <- c
			}
		case "set_result":
			var payload struct {
				File string `json:"file"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				c.Hub.mu.Lock()
				c.Hub.State.ActiveResult = payload.File
				c.Hub.mu.Unlock()
				c.Hub.BroadcastJSON(msg)
			}
		case "client_command":
			var payload struct {
				Target  string `json:"target"`
				Command string `json:"command"`
				Value   string `json:"value"` // Generic value field
			}
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				var targetClient *Client

				c.Hub.mu.Lock()
				for target := range c.Hub.Clients {
					if target.Conn.RemoteAddr().String() == payload.Target {
						targetClient = target
						if payload.Command != "rename" {
							target.DisplayMode = payload.Command // Update state immediately under lock
						}
						break
					}
				}
				c.Hub.mu.Unlock()

				if targetClient != nil {
					if payload.Command == "rename" {
						// Send update_config to client
						msgData, err := json.Marshal(struct {
							Type    string `json:"type"`
							Payload struct {
								Key   string `json:"key"`
								Value string `json:"value"`
							} `json:"payload"`
						}{
							Type: "update_config",
							Payload: struct {
								Key   string `json:"key"`
								Value string `json:"value"`
							}{Key: "ClientName", Value: payload.Value},
						})
						if err != nil {
							log.Printf("Error marshaling update_config message: %v", err)
						} else {
							c.Hub.SendTo <- struct {
								Client *Client
								Msg    []byte
							}{Client: targetClient, Msg: msgData}
						}

					} else {
						// Forward other commands as display_mode
						msgData, err := json.Marshal(struct {
							Type    string `json:"type"`
							Payload string `json:"payload"`
						}{
							Type:    "display_mode",
							Payload: payload.Command,
						})
						if err != nil {
							log.Printf("Error marshaling display_mode message: %v", err)
						} else {
							// Send once to the target client (channel is now buffered)
							c.Hub.SendTo <- struct {
								Client *Client
								Msg    []byte
							}{Client: targetClient, Msg: msgData}
						}

						// Broadcast updated list (DisplayMode changed)
						c.Hub.broadcastClientList()
					}
				}
			}
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveWs handles websocket requests from the peer.
func serveWs(hub *Hub, timerMgr *TimerManager, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{Hub: hub, TimerMgr: timerMgr, Conn: conn, Send: make(chan []byte, 256)}

	// Start writePump before sending messages so it can handle them
	go client.writePump()
	go client.readPump()

	client.Hub.Register <- client

	// Send current timer state immediately upon connection
	timerMgr.mu.Lock()
	timerStateMsg, err := json.Marshal(struct {
		Type    string     `json:"type"`
		Payload TimerState `json:"payload"`
	}{
		Type:    "timer_update",
		Payload: timerMgr.State,
	})
	timerMgr.mu.Unlock()
	if err != nil {
		log.Printf("Error marshaling timer state: %v", err)
	} else {
		client.Send <- timerStateMsg
	}

	// Send current active result
	hub.mu.Lock()
	if hub.State.ActiveResult != "" {
		resultMsg, err := json.Marshal(struct {
			Type    string `json:"type"`
			Payload struct {
				File string `json:"file"`
			} `json:"payload"`
		}{
			Type: "set_result",
			Payload: struct {
				File string `json:"file"`
			}{File: hub.State.ActiveResult},
		})
		hub.mu.Unlock()
		if err != nil {
			log.Printf("Error marshaling result message: %v", err)
		} else {
			client.Send <- resultMsg
		}
	} else {
		hub.mu.Unlock()
	}

	// Send initial display mode (defaults to "show_result" if empty)
	initMode := client.DisplayMode
	if initMode == "" {
		initMode = "show_result"
	}
	modeMsg, err := json.Marshal(struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}{
		Type:    "display_mode",
		Payload: initMode,
	})
	if err != nil {
		log.Printf("Error marshaling display mode message: %v", err)
	} else {
		client.Send <- modeMsg
	}
}
