package matugen

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// Output matches the JSON dump structure of `matugen image -j hex`.
type Output struct {
	Colors map[string]struct {
		Dark    struct{ Color string } `json:"dark"`
		Default struct{ Color string } `json:"default"`
		Light   struct{ Color string } `json:"light"`
	} `json:"colors"`
	IsDarkMode bool   `json:"is_dark_mode"`
	Mode       string `json:"mode"`
}

// Extract runs matugen on the given image path and extracts Material You colors.
func Extract(imagePath string, dark bool) (map[string]string, error) {
	if imagePath == "" {
		return nil, fmt.Errorf("no image path provided")
	}

	mode := "dark"
	if !dark {
		mode = "light"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "matugen", "image",
		"--prefer", "saturation",
		"--mode", mode,
		"--dry-run",
		"-j", "hex",
		imagePath,
	)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("execute matugen (%s): %w", strings.TrimSpace(string(exitErr.Stderr)), err)
		}
		return nil, fmt.Errorf("execute matugen: %w", err)
	}

	var mOutput Output
	if err := json.Unmarshal(out, &mOutput); err != nil {
		return nil, fmt.Errorf("unmarshal matugen json: %w", err)
	}

	colors := make(map[string]string)
	pickColor := func(key string) string {
		if entry, ok := mOutput.Colors[key]; ok {
			if dark {
				if entry.Dark.Color != "" {
					return entry.Dark.Color
				}
			} else {
				if entry.Light.Color != "" {
					return entry.Light.Color
				}
			}
			return entry.Default.Color
		}
		return ""
	}

	// Surface hierarchy
	surface := pickColor("surface_container_lowest")
	if surface == "" {
		surface = pickColor("surface")
	}
	if surface == "" {
		surface = pickColor("background")
	}

	surfaceRaised := pickColor("surface_container")
	if surfaceRaised == "" {
		surfaceRaised = pickColor("surface_variant")
	}

	surfaceSolid := pickColor("surface_container_low")
	if surfaceSolid == "" {
		surfaceSolid = surface
	}

	// Text
	text := pickColor("on_surface")
	if text == "" {
		text = pickColor("on_background")
	}

	textDim := pickColor("on_surface_variant")
	textFaint := pickColor("outline")

	// Accent
	accent := pickColor("primary")
	accentHover := pickColor("primary_container")
	accentActive := pickColor("inverse_primary")
	accentContrast := pickColor("on_primary")

	// Status
	errorColor := pickColor("error")
	errorBright := pickColor("error_container")

	// Assign tokens
	if surface != "" {
		colors["surface"] = surface
	}
	if surfaceRaised != "" {
		colors["surface-raised"] = surfaceRaised
	}
	if surfaceSolid != "" {
		colors["surface-solid"] = surfaceSolid
	}
	if text != "" {
		colors["text"] = text
	}
	if textDim != "" {
		colors["text-dim"] = textDim
	}
	if textFaint != "" {
		colors["text-faint"] = textFaint
	}
	if dark {
		colors["text-bright"] = "#ffffff"
	} else {
		colors["text-bright"] = "#000000"
	}

	if accent != "" {
		colors["accent"] = accent
	}
	if accentHover != "" {
		colors["accent-hover"] = accentHover
	}
	if accentActive != "" {
		colors["accent-active"] = accentActive
	}
	if accentContrast != "" {
		colors["accent-contrast"] = accentContrast
	}

	if errorColor != "" {
		colors["error"] = errorColor
	}
	if errorBright != "" {
		colors["error-bright"] = errorBright
	}

	slog.Debug("matugen: extracted palette from wallpaper", "image", imagePath, "colors", len(colors))
	return colors, nil
}
