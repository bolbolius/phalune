// Package settings implements the phalune settings application. It edits
// config.toml in place with line-preserving writes, then asks the running
// shell over IPC to hot-reload.
package settings

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Editor performs field-level edits on a TOML file while preserving all
// unrelated bytes, comments, and ordering.
type Editor struct {
	path string

	// lines holds content without line terminators. trailing records
	// whether the last line ends with a newline in the file.
	lines    []string
	trailing bool

	dirty bool
}

func NewEditor(path string) (*Editor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Editor{path: path}, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	text := string(data)
	trailing := strings.HasSuffix(text, "\n")
	if trailing {
		text = text[:len(text)-1]
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	return &Editor{path: path, lines: lines, trailing: trailing}, nil
}

// Path returns the file path this editor writes.
func (e *Editor) Path() string { return e.path }

func (e *Editor) sectionFor(row Row) (section, key string) {
	dot := strings.LastIndex(row.Key, ".")
	if dot < 0 {
		return "", row.Key
	}
	return row.Key[:dot], row.Key[dot+1:]
}

// Set writes value for row's dotted key ("section.key"). Line-preserving:
// existing lines rewrite in place; new keys append under their section and
// the section header is created when missing.
func (e *Editor) Set(row Row, value string) error {
	section, key := e.sectionFor(row)

	secIdx := -1
	if section != "" {
		secIdx = e.findSection(section)
	}

	if idx := e.findKey(secIdx, key); idx >= 0 {
		e.lines[idx] = formatEntry(row, key, value, e.lines[idx])
		e.dirty = true
		return nil
	}

	// Key missing: insert into its section, else create the section first.
	if secIdx >= 0 {
		e.insertAt(e.sectionEnd(secIdx), formatEntryRaw(row, key, value))
		e.dirty = true
		return nil
	}

	if section == "" {
		e.append(formatEntryRaw(row, key, value))
		e.dirty = true
		return nil
	}

	if len(e.lines) > 0 {
		e.append("")
	}
	e.append("[" + section + "]")
	e.append(formatEntryRaw(row, key, value))
	e.dirty = true
	return nil
}

func (e *Editor) findSection(name string) int {
	header := "[" + name + "]"
	for i, line := range e.lines {
		if strings.TrimSpace(line) == header {
			return i
		}
	}
	return -1
}

// findKey scans from the line after the section header to the next header.
func (e *Editor) findKey(secIdx int, key string) int {
	start := 0
	if secIdx >= 0 {
		start = secIdx + 1
	}
	for i := start; i < len(e.lines); i++ {
		trimmed := strings.TrimSpace(e.lines[i])
		if strings.HasPrefix(trimmed, "[") {
			if secIdx >= 0 {
				break // next section reached
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if k, _, ok := parseEntry(trimmed); ok && k == key {
			return i
		}
	}
	return -1
}

func (e *Editor) sectionEnd(secIdx int) int {
	for i := secIdx + 1; i < len(e.lines); i++ {
		trimmed := strings.TrimSpace(e.lines[i])
		if strings.HasPrefix(trimmed, "[") {
			for i > secIdx+1 && strings.TrimSpace(e.lines[i-1]) == "" {
				i--
			}
			return i
		}
	}
	return len(e.lines)
}

func (e *Editor) insertAt(idx int, line string) {
	lines := make([]string, 0, len(e.lines)+1)
	lines = append(lines, e.lines[:idx]...)
	lines = append(lines, line)
	lines = append(lines, e.lines[idx:]...)
	e.lines = lines
}

func (e *Editor) append(line string) {
	e.lines = append(e.lines, line)
	e.trailing = true
}

func parseEntry(line string) (key, value string, ok bool) {
	eq := strings.Index(line, "=")
	if eq <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:eq])
	if key == "" || strings.ContainsAny(key, " \t#") {
		return "", "", false
	}
	value = strings.TrimSpace(line[eq+1:])
	return key, value, true
}

func formatEntry(row Row, key, value, original string) string {
	indent := original[:len(original)-len(strings.TrimLeft(original, " \t"))]
	return indent + key + " = " + encodeValue(row, value)
}

func formatEntryRaw(row Row, key, value string) string {
	return key + " = " + encodeValue(row, value)
}

// encodeValue renders settings as TOML; widget lists pass through as arrays.
func encodeValue(row Row, value string) string {
	if row.Kind == KindWidgets {
		return value
	}
	return tomlValue(value)
}

func tomlValue(v string) string {
	if v == "true" || v == "false" {
		return v
	}
	if isNumeric(v) {
		return v
	}
	if len(v) >= 2 && strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
		return v
	}
	escaped := strings.ReplaceAll(v, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}

func isNumeric(v string) bool {
	if v == "" {
		return false
	}
	body := strings.TrimPrefix(v, "-")
	if body == "" {
		return false
	}
	dots := 0
	for _, r := range body {
		switch {
		case r >= '0' && r <= '9':
		case r == '.':
			dots++
			if dots > 1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Save writes the file atomically (temp file + rename), preserving mode.
func (e *Editor) Save() error {
	if len(e.lines) == 0 {
		return fmt.Errorf("refusing to write empty config")
	}

	var b strings.Builder
	for i, line := range e.lines {
		b.WriteString(line)
		if i < len(e.lines)-1 || e.trailing {
			b.WriteString("\n")
		}
	}

	mode := os.FileMode(0o644)
	if st, err := os.Stat(e.path); err == nil {
		mode = st.Mode().Perm()
	}

	tmp := e.path + ".settings-tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), mode); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, e.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	e.dirty = false
	return nil
}

// Lines returns a copy for diagnostics and tests.
func (e *Editor) Lines() []string {
	out := make([]string, len(e.lines))
	copy(out, e.lines)
	return out
}

// ReadFileText returns the raw file contents (diagnostics / undo).
func ReadFileText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var b strings.Builder
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteString("\n")
	}
	return b.String(), sc.Err()
}
