package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// --- ALIAS MAPPINGS ---
var (
	aliasMappings sync.Map
)

func getMappingsPath() string {
	return DataDir + "/mappings.json"
}

// --------------------------------

func loadMappings() {
	if _, err := os.Stat(getMappingsPath()); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(getMappingsPath())
	if err != nil {
		log.Printf("⚠️ Failed to read mappings.json: %v", err)
		return
	}

	tempMappings := make(map[string]Config)
	if err := json.Unmarshal(data, &tempMappings); err != nil {
		log.Printf("⚠️ Failed to parse mappings.json: %v", err)
	} else {
		count := 0
		for k, v := range tempMappings {
			aliasMappings.Store(k, v)
			count++
		}
		log.Printf("📁 Loaded %d alias mappings from disk", count)
	}
}

func saveMappings() {
	tempMappings := make(map[string]Config)
	aliasMappings.Range(func(key, value interface{}) bool {
		tempMappings[key.(string)] = value.(Config)
		return true
	})

	data, err := json.MarshalIndent(tempMappings, "", "  ")
	if err != nil {
		log.Printf("❌ Failed to serialize mappings: %v", err)
		return
	}

	if err := os.WriteFile(getMappingsPath(), data, 0644); err != nil {
		log.Printf("❌ Failed to write mappings.json: %v", err)
	}
}

func generateID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
