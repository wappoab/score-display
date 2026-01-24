package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/grandcat/zeroconf"
)

type ServiceEntry struct {
	Host string
	Port int
	IP   string
}

func findServer() (*ServiceEntry, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Browse for _display._tcp
	err = resolver.Browse(ctx, "_display._tcp", "local.", entries)
	if err != nil {
		return nil, fmt.Errorf("failed to browse: %w", err)
	}

	fmt.Println("Scanning for Display Server...")
	for entry := range entries {
		if len(entry.AddrIPv4) > 0 {
			ip := entry.AddrIPv4[0].String()
			log.Printf("Found Server: %s at %s:%d", entry.Instance, ip, entry.Port)
			return &ServiceEntry{
				Host: entry.HostName,
				Port: entry.Port,
				IP:   ip,
			}, nil
		}
	}

	return nil, fmt.Errorf("no server found within timeout")
}
