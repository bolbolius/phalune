package launcher

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

type App struct {
	ID          string
	Name        string
	GenericName string
	Comment     string
	Icon        string
	Exec        string
	CleanExec   string
	Terminal    bool
	Categories  []string
	Keywords    []string
}

var execCodeRegex = regexp.MustCompile(`%[fFuUnNkKmicdvs]`)

func CleanExec(execStr string) string {
	cleaned := execCodeRegex.ReplaceAllString(execStr, "")
	return strings.TrimSpace(cleaned)
}

func ApplicationDirs() []string {
	var dirs []string

	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		dirs = append(dirs, filepath.Join(dataHome, "applications"))
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "applications"))
	}

	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}

	for _, d := range strings.Split(dataDirs, ":") {
		d = strings.TrimSpace(d)
		if d != "" {
			dirs = append(dirs, filepath.Join(d, "applications"))
		}
	}

	return dirs
}

func ScanApplications() ([]App, error) {
	seen := make(map[string]bool)
	var apps []App

	for _, dir := range ApplicationDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
				continue
			}

			id := entry.Name()
			if seen[id] {
				continue
			}

			filePath := filepath.Join(dir, id)
			app, ok := parseDesktopFile(filePath, id)
			if !ok {
				continue
			}

			seen[id] = true
			apps = append(apps, app)
		}
	}

	return apps, nil
}

func parseDesktopFile(path string, id string) (App, bool) {
	file, err := os.Open(path)
	if err != nil {
		return App{}, false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inDesktopEntry := false

	var app App
	app.ID = id
	noDisplay := false
	hidden := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := line[1 : len(line)-1]
			inDesktopEntry = section == "Desktop Entry"
			continue
		}

		if !inDesktopEntry {
			continue
		}

		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		switch key {
		case "Type":
			if val != "Application" {
				return App{}, false
			}
		case "Name":
			if app.Name == "" {
				app.Name = val
			}
		case "GenericName":
			if app.GenericName == "" {
				app.GenericName = val
			}
		case "Comment":
			if app.Comment == "" {
				app.Comment = val
			}
		case "Icon":
			app.Icon = val
		case "Exec":
			app.Exec = val
			app.CleanExec = CleanExec(val)
		case "Terminal":
			app.Terminal = strings.EqualFold(val, "true")
		case "NoDisplay":
			noDisplay = strings.EqualFold(val, "true")
		case "Hidden":
			hidden = strings.EqualFold(val, "true")
		case "Categories":
			for _, c := range strings.Split(val, ";") {
				c = strings.TrimSpace(c)
				if c != "" {
					app.Categories = append(app.Categories, c)
				}
			}
		case "Keywords":
			for _, k := range strings.Split(val, ";") {
				k = strings.TrimSpace(k)
				if k != "" {
					app.Keywords = append(app.Keywords, k)
				}
			}
		}
	}

	if noDisplay || hidden || app.Name == "" || app.Exec == "" {
		return App{}, false
	}

	return app, true
}

func LaunchApp(app App, preferredTerm ...string) error {
	rawCmd := app.CleanExec
	if rawCmd == "" {
		rawCmd = app.Exec
	}

	parts, err := splitCommandLine(rawCmd)
	if err != nil || len(parts) == 0 {
		return fmt.Errorf("invalid command line %q", rawCmd)
	}

	if app.Terminal {
		term := ""
		if len(preferredTerm) > 0 && preferredTerm[0] != "" {
			term = preferredTerm[0]
		}
		if term == "" {
			term = os.Getenv("TERMINAL")
		}
		if term == "" {
			for _, candidate := range []string{"alacritty", "kitty", "foot", "wezterm", "gnome-terminal", "xterm"} {
				if _, err := exec.LookPath(candidate); err == nil {
					term = candidate
					break
				}
			}
		}
		if term != "" {
			parts = append([]string{term, "-e"}, parts...)
		}
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	return cmd.Start()
}

func splitCommandLine(cmd string) ([]string, error) {
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]

		if escaped {
			current.WriteByte(c)
			escaped = false
			continue
		}

		if c == '\\' && !inSingle {
			escaped = true
			continue
		}

		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}

		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if (c == ' ' || c == '\t') && !inSingle && !inDouble {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteByte(c)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts, nil
}
