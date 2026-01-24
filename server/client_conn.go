package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
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
						msgData, _ := json.Marshal(struct {
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
						c.Hub.SendTo <- struct {
							Client *Client
							Msg    []byte
						}{Client: targetClient, Msg: msgData}

					} else {
						// Forward other commands as display_mode
						msgData, _ := json.Marshal(struct {
							Type    string `json:"type"`
							Payload string `json:"payload"`
						}{
							Type:    "display_mode",
							Payload: payload.Command,
						})

						// Brute-force reliability: Send 3 times
						go func(c *Client, msg []byte) {
							for i := 0; i < 3; i++ {
								c.Hub.SendTo <- struct {
									Client *Client
									Msg    []byte
								}{Client: targetClient, Msg: msg}
								time.Sleep(100 * time.Millisecond)
							}
						}(c, msgData)
						
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
	client.Hub.Register <- client

	// Send current timer state immediately upon connection
	conn.WriteJSON(struct {
		Type    string     `json:"type"`
		Payload TimerState `json:"payload"`
	}{
		Type:    "timer_update",
		Payload: timerMgr.State,
	})

	// Send current active result
	hub.mu.Lock()
	if hub.State.ActiveResult != "" {
		conn.WriteJSON(struct {
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
	}
	
	// Send initial display mode (defaults to "show_result" if empty)
	initMode := client.DisplayMode
	if initMode == "" {
		initMode = "show_result"
	}
	conn.WriteJSON(struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}{
		Type:    "display_mode",
		Payload: initMode,
	})

	hub.mu.Unlock()

	go client.writePump()
	go client.readPump()
}