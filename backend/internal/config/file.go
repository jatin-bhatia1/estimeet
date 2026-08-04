package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// DefaultFile is read from the working directory when ESTIMEET_CONFIG_FILE is
// not set. A missing file is not an error: every setting still has a default.
const DefaultFile = "estimeet.conf"

// loadFile seeds the process environment from a `KEY = value` file so an
// operator can keep the footer links, the contact address and everything else
// in one place instead of a shell script that has to be remembered.
//
// Real environment variables always win, which keeps containers and CI - where
// settings arrive as `-e` flags or secrets - in charge of anything the file
// happens to mention.
func loadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		key, value, ok := parseLine(scanner.Text())
		if !ok {
			continue
		}
		if key == "" {
			return fmt.Errorf("%s line %d: a setting needs a name", path, line)
		}
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("%s line %d: %w", path, line, err)
		}
	}
	return scanner.Err()
}

// parseLine reads one `KEY = value` line, ignoring blanks and # comments.
// Values may be quoted to keep leading or trailing spaces, or a trailing #.
func parseLine(raw string) (key, value string, ok bool) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")

	name, rest, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	value = strings.TrimSpace(rest)

	switch {
	case len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"',
		len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'':
		value = value[1 : len(value)-1]
	default:
		// Unquoted values may carry a trailing comment.
		if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
	}
	return strings.TrimSpace(name), value, true
}
