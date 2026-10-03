package clipboard

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strings"
	"time"
)

// newID generates a short unique id for an entry.
func newID(at int64) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return strings.Join([]string{
		time.UnixMilli(at).Format("20060102150405"),
		hex.EncodeToString(b[:]),
	}, "-")
}

// fileURIToPath converts a file:// URI to a filesystem path. Anything else is
// returned unchanged.
func fileURIToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return strings.TrimPrefix(uri, "file://")
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return parsed.Path
	}
	return path
}

// splitURIs splits a text/x-special/gnome-copied-files or x-special/nautilus-clipboard
// payload (one URI per line) and drops the leading operation marker
// ("copy"/"cut").
func splitURIs(payload string) []string {
	var uris []string
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "copy") || strings.EqualFold(line, "cut") {
			continue
		}
		uris = append(uris, line)
	}
	return uris
}

// fileCopyPayload builds the copy-marker payload stored as File entry Text.
func fileCopyPayload(uris []string) string {
	lines := []string{"copy"}
	lines = append(lines, uris...)
	return strings.Join(lines, "\n")
}
