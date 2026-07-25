package main

import (
	"log"
	"os"
)

// LoadHWAccelProfiles returns the best available FFmpeg hardware acceleration arguments.
func LoadHWAccelProfiles() []string {
	log.Printf("⚡ Loading Hardware Acceleration Profiles...")

	// E.g. Check for NVIDIA NVENC
	if os.Getenv("NVIDIA_VISIBLE_DEVICES") != "" {
		log.Printf("⚡ NVIDIA GPU detected! Using NVENC/CUVID hardware acceleration.")
		return []string{"-hwaccel", "cuda", "-c:v", "h264_cuvid"}
	}

	// E.g. Check for Intel QuickSync
	if _, err := os.Stat("/dev/dri/renderD128"); err == nil {
		log.Printf("⚡ Intel GPU detected! Using QuickSync hardware acceleration.")
		return []string{"-hwaccel", "qsv", "-c:v", "h264_qsv"}
	}

	log.Printf("⚡ No specific hardware acceleration detected, falling back to software.")
	return []string{}
}
