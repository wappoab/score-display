package main

import (
	"encoding/json"
	"log"
	"sort"
	"strings"
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
	closeOnce   sync.Once
}

// closeClientSend safely closes the client's Send channel exactly once
func (c *Client) closeClientSend() {
	c.closeOnce.Do(func() {
		close(c.Send)
	})
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
	MaxClients int        // Maximum allowed clients (0 = unlimited)
	mu         sync.Mutex // Protects Clients map and State
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
		}, 256),
		Clients:    make(map[*Client]bool),
		MaxClients: 100, // Default connection limit
	}
	return h
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			// Check connection limit
			if h.MaxClients > 0 && len(h.Clients) >= h.MaxClients {
				h.mu.Unlock()
				log.Printf("Client rejected (limit reached): %s", client.Conn.RemoteAddr())
				// Send error message and close
				errorMsg, err := json.Marshal(struct {
					Type    string `json:"type"`
					Payload string `json:"payload"`
				}{
					Type:    "error",
					Payload: "Server connection limit reached",
				})
				if err != nil {
					log.Printf("Error marshaling rejection message: %v", err)
				} else {
					select {
					case client.Send <- errorMsg:
					default:
					}
				}
				client.closeClientSend()
				continue
			}
			h.Clients[client] = true
			h.mu.Unlock()
			log.Printf("Client connected: %s", client.Conn.RemoteAddr())
			h.broadcastClientList()

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				client.closeClientSend()
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
					delete(h.Clients, job.Client)
					h.mu.Unlock()
					job.Client.closeClientSend()
					continue
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
	// Collect clients to remove
	var toRemove []*Client
	for client := range h.Clients {
		select {
		case client.Send <- message:
		default:
			toRemove = append(toRemove, client)
		}
	}
	// Remove failed clients
	for _, client := range toRemove {
		delete(h.Clients, client)
	}
	h.mu.Unlock()

	// Close channels outside the lock
	for _, client := range toRemove {
		client.closeClientSend()
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
	h.mu.Unlock() // Unlock before expensive operations

	// Sort by Name, then Addr (done outside lock)
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		}
		return list[i].Addr < list[j].Addr
	})

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
