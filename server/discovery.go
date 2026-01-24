package main

import (
	"log"
	"os"

	"github.com/grandcat/zeroconf"
)

var server *zeroconf.Server

func startDiscovery(port int) {
	hostname, _ := os.Hostname()
	// Service Name: DisplayServer
	// Service Type: _display._tcp
	// Domain: local.
	var err error
	server, err = zeroconf.Register("DisplayServer", "_display._tcp", "local.", port, []string{"txtv=0", "version=1.0"}, nil)
	if err != nil {
		log.Fatalf("Failed to register mDNS service: %v", err)
	}

	log.Printf("mDNS Service registered: %s._display._tcp.local. on port %d", hostname, port)
}

func stopDiscovery() {
	if server != nil {
		server.Shutdown()
	}
}
