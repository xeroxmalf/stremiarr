package main

import (
	"log"
	"net/http"
	"os/exec"
)

// TranscodeAudio streams video via FFmpeg to transcode unsupported audio codecs to AAC.
func TranscodeAudio(w http.ResponseWriter, r *http.Request, streamURL string) {
	log.Printf("🎵 On-the-fly Audio Transcoding triggered for %s", streamURL)

	hwArgs := LoadHWAccelProfiles()

	args := append(hwArgs,
		"-i", streamURL,
		"-c:v", "copy", // Copy video stream directly
		"-c:a", "aac", // Transcode audio to highly-compatible AAC
		"-b:a", "192k",
		"-f", "matroska", // Stream out as MKV
		"pipe:1",
	)

	cmd := exec.Command("ffmpeg", args...)

	// Stream ffmpeg stdout directly to the HTTP response writer
	cmd.Stdout = w
	cmd.Stderr = nil // ignore stderr to prevent console spam

	w.Header().Set("Content-Type", "video/x-matroska")
	w.Header().Set("Transfer-Encoding", "chunked")

	err := cmd.Run()
	if err != nil {
		log.Printf("⚠️ Transcoding stopped or failed: %v", err)
	}
}
