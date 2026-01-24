package main

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Message defines the JSON structure for communication
type Message struct {
	Type    string          `json:"type"`              // e.g., "timer", "command", "handshake"
	Payload json.RawMessage `json:"payload,omitempty"` // Flexible payload
}

type Client struct {
	Hub         *Hub
	TimerMgr    *TimerManager
	Conn        *websocket.Conn
	Send        chan []byte
	ID          string
	Name        string
	DisplayMode string // "show_timer" or "show_result"
}

type Hub struct {
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	Handshake  chan *Client
	SendTo     chan struct {
		Client *Client
		Msg    []byte
	}
	State struct {
		ActiveResult string
	}
	mu sync.Mutex // Protects Clients map and State
}

func NewHub() *Hub {
	h := &Hub{
		Broadcast:  make(chan []byte),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Handshake:  make(chan *Client),
		SendTo: make(chan struct {
			Client *Client
			Msg    []byte
		}),
		Clients: make(map[*Client]bool),
	}
	return h
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client] = true
			h.mu.Unlock()
			log.Printf("Client connected: %s", client.Conn.RemoteAddr())
			h.broadcastClientList()

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				log.Printf("Client disconnected: %s", client.Conn.RemoteAddr())
			}
			h.mu.Unlock()
			h.broadcastClientList()

		case client := <-h.Handshake:
			log.Printf("Client handshake: %s (%s)", client.Name, client.Conn.RemoteAddr())
			h.broadcastClientList()

		case job := <-h.SendTo:
			h.mu.Lock()
			if _, ok := h.Clients[job.Client]; ok {
				select {
				case job.Client.Send <- job.Msg:
				default:
					close(job.Client.Send)
					delete(h.Clients, job.Client)
				}
			}
			h.mu.Unlock()

		case message := <-h.Broadcast:
			h.broadcastData(message)
		}
	}
}

func (h *Hub) broadcastData(message []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.Clients {
		select {
		case client.Send <- message:
		default:
			close(client.Send)
			delete(h.Clients, client)
		}
	}
}

func (h *Hub) broadcastClientList() {
	h.mu.Lock()
	type ClientInfo struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Addr        string `json:"addr"`
		DisplayMode string `json:"display_mode"`
	}
	var list []ClientInfo
	for client := range h.Clients {
		name := client.Name
		if name == "" {
			name = "Unknown"
		}
		mode := client.DisplayMode
		if mode == "" {
			mode = "show_result" // Default
		}
		list = append(list, ClientInfo{
			ID:          client.ID,
			Name:        name,
			Addr:        client.Conn.RemoteAddr().String(),
			DisplayMode: mode,
		})
	}
	h.mu.Unlock() // Unlock before marshaling/broadcasting to avoid holding lock too long (though broadcastData re-locks)

	msg := struct {
		Type    string       `json:"type"`
		Payload []ClientInfo `json:"payload"`
	}{
		Type:    "client_list",
		Payload: list,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling client list: %v", err)
		return
	}
	
	// Directly call broadcastData instead of sending to channel, 
	// because we are already in the Run loop (or called from it) 
	// and sending to channel would deadlock if channel is unbuffered and we are the reader.
	h.broadcastData(data)
}

// Helper to broadcast JSON messages
func (h *Hub) BroadcastJSON(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}
	h.Broadcast <- data
}
