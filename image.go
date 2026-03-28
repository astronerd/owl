package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BourgeoisBear/rasterm"
)

var imageCacheDir = filepath.Join(os.TempDir(), "feishu-tui-images")

func init() {
	os.MkdirAll(imageCacheDir, 0755)
}

// parseImageKey extracts image_key from message content.
// Handles both JSON {"image_key":"img_xxx"} and text "[Image: img_xxx]" formats.
func parseImageKey(content string) string {
	var m map[string]string
	if json.Unmarshal([]byte(content), &m) == nil {
		if k := m["image_key"]; k != "" {
			return k
		}
	}
	if strings.HasPrefix(content, "[Image: ") && strings.HasSuffix(content, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(content, "[Image: "), "]")
	}
	idx := strings.Index(content, "img_")
	if idx >= 0 {
		end := idx
		for end < len(content) && content[end] != ']' && content[end] != '"' && content[end] != ' ' && content[end] != '}' {
			end++
		}
		return content[idx:end]
	}
	return ""
}

// downloadImage downloads an image from Feishu API and caches it locally.
func downloadImage(messageID, imageKey string) (string, error) {
	outName := imageKey
	cachePath := filepath.Join(imageCacheDir, outName)
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	cmd := exec.Command("lark-cli", "im", "+messages-resources-download",
		"--message-id", messageID,
		"--file-key", imageKey,
		"--type", "image",
		"--output", outName,
		"--as", "user")
	cmd.Dir = imageCacheDir
	if err := cmd.Run(); err != nil {
		return "", err
	}

	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}
	return "", fmt.Errorf("download failed")
}

// renderImageKitty renders an image using Kitty graphics protocol.
// Returns the escape sequence string and the number of terminal rows it occupies.
func renderImageKitty(path string, maxCols, maxRows int) (string, int) {
	f, err := os.Open(path)
	if err != nil {
		return "[image: error]", 1
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "[image]", 1
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return "[image]", 1
	}

	// Calculate display dimensions preserving aspect ratio
	// Assume ~2:1 pixel aspect ratio for terminal cells (cells are taller than wide)
	dispCols := maxCols
	dispRows := maxRows
	aspectW := float64(w) / float64(dispCols)
	aspectH := float64(h) / float64(dispRows*2) // *2 because cells are ~2x tall
	if aspectW > aspectH {
		// Width-constrained
		dispRows = int(float64(h) / aspectW / 2)
	} else {
		// Height-constrained
		dispCols = int(float64(w) / aspectH)
	}
	if dispCols < 1 {
		dispCols = 1
	}
	if dispRows < 1 {
		dispRows = 1
	}

	opts := rasterm.KittyImgOpts{
		DstCols: uint32(dispCols),
		DstRows: uint32(dispRows),
	}

	var buf bytes.Buffer
	if err := rasterm.KittyWriteImage(&buf, img, opts); err != nil {
		return "[image: render error]", 1
	}

	// Wrap with cursor save/restore so the image doesn't break bubbletea's layout.
	// The image renders as an overlay; placeholder lines in the view leave visual space.
	kittyStr := "\x1b7" + buf.String() + "\x1b8"
	return kittyStr, dispRows
}
