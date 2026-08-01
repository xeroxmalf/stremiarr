package main

import (
	"log"
	"os"
	"os/exec"
	"sync"
)

var (
	hwAccelOnce sync.Once
	hwAccelArgs []string
)

// LoadHWAccelProfiles returns the best available FFmpeg hardware acceleration arguments.
// Result is cached after first call.
func LoadHWAccelProfiles() []string {
	hwAccelOnce.Do(func() {
		hwAccelArgs = detectHWAccel()
	})
	return hwAccelArgs
}

func detectHWAccel() []string {
	// NVIDIA: check via nvidia-smi or env
	if os.Getenv("NVIDIA_VISIBLE_DEVICES") != "" || checkCmd("nvidia-smi") {
		log.Printf("⚡ NVIDIA GPU detected! Using NVENC/CUVID hardware acceleration.")
		return []string{"-hwaccel", "cuda", "-c:v", "h264_cuvid"}
	}

	// Intel QuickSync: check /dev/dri
	if _, err := os.Stat("/dev/dri/renderD128"); err == nil {
		log.Printf("⚡ Intel GPU detected! Using QuickSync hardware acceleration.")
		return []string{"-hwaccel", "qsv", "-c:v", "h264_qsv"}
	}

	// AMD AMF: check /dev/dri
	if _, err := os.Stat("/dev/dri/renderD128"); err == nil && os.Getenv("AMDGPU_VISIBLE_DEVICES") != "" {
		log.Printf("⚡ AMD GPU detected! Using AMF hardware acceleration.")
		return []string{"-hwaccel", "amf", "-c:v", "h264_amf"}
	}

	log.Printf("⚡ No specific hardware acceleration detected, falling back to software.")
	return []string{}
}

func checkCmd(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
