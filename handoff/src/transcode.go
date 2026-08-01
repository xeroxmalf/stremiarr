package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strings"
)

// TranscodeAudio streams video via FFmpeg to transcode unsupported audio codecs to AAC.
func TranscodeAudio(w http.ResponseWriter, r *http.Request, streamURL string) {
	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Printf("⚠️ ffmpeg not found, falling back to direct proxy")
		proxyURL, err := url.Parse(streamURL)
		if err != nil {
			http.Error(w, "Invalid stream URL", http.StatusBadRequest)
			return
		}
		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL = proxyURL
				req.Host = proxyURL.Host
			},
			Transport: customTransport,
		}
		proxy.ServeHTTP(w, r)
		return
	}

	log.Printf("🎵 On-the-fly Audio Transcoding triggered for %s", streamURL)

	// Note: Byte-range seeking doesn't work with transcoded streams.
	// We stream from the start and let the client buffer.
	hwArgs := LoadHWAccelProfiles()

	// hwArgs (e.g., -hwaccel cuda -c:v h264_cuvid) must come before -i
	args := append([]string(nil), hwArgs...)
	args = append(args,
		"-i", streamURL,
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-f", "matroska",
		"pipe:1",
	)

	cmd := exec.Command("ffmpeg", args...)

	var stderrBuf bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &stderrBuf

	w.Header().Set("Content-Type", "video/x-matroska")

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderrBuf.String())
		if !strings.Contains(errMsg, "Operation not permitted") && !strings.Contains(errMsg, "Broken pipe") {
			log.Printf("⚠️ Transcoding failed: %v | ffmpeg: %s", err, errMsg)
		}
	}
}
