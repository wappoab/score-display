package main

import (
	"encoding/json"
	"os"
)

type ServerConfig struct {
	ResultsDir string `json:"resultsDir"`
	Language   string `json:"language"`
	Port       int    `json:"port"`
}

func loadConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
