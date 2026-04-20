package sampler

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// LoadDotEnv reads a dotenv-style file and sets any variables that are not
// already present in the process environment (existing env vars always win, so
// shell/CI overrides work as users expect).
//
// Supported syntax:
//   - KEY=value
//   - KEY="value with spaces"
//   - KEY='value'
//   - KEY= (empty string)
//   - export KEY=value
//   - # comments and blank lines are ignored
//   - trailing whitespace after unquoted values is trimmed
//   - inline comments on unquoted values (` # ...`) are stripped
//
// Missing files are not an error (returns nil). Parse errors on individual
// lines are reported via log (if non-nil) and skipped.
func LoadDotEnv(path string, log Logger) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	return loadDotEnvFrom(f, path, log, osEnvSetter{})
}

// envSetter is factored out so tests can capture calls without mutating os.Environ.
type envSetter interface {
	Lookup(key string) (string, bool)
	Set(key, value string) error
}

type osEnvSetter struct{}

func (osEnvSetter) Lookup(k string) (string, bool) { return os.LookupEnv(k) }
func (osEnvSetter) Set(k, v string) error          { return os.Setenv(k, v) }

func loadDotEnvFrom(r io.Reader, path string, log Logger, env envSetter) error {
	scanner := bufio.NewScanner(r)
	// Allow lines up to 1 MiB (default is 64 KiB, which is plenty but the
	// upgrade is cheap and avoids surprises with long API keys).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimLeft(line, " \t")

		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			if log != nil {
				log(fmt.Sprintf("Warning: %s:%d: skipping line without KEY=VALUE: %q", path, lineNum, line))
			}
			continue
		}

		key := strings.TrimSpace(line[:eq])
		if !isValidEnvKey(key) {
			if log != nil {
				log(fmt.Sprintf("Warning: %s:%d: skipping invalid variable name %q", path, lineNum, key))
			}
			continue
		}

		rawValue := line[eq+1:]
		value, err := parseDotEnvValue(rawValue)
		if err != nil {
			if log != nil {
				log(fmt.Sprintf("Warning: %s:%d: %v", path, lineNum, err))
			}
			continue
		}

		if _, ok := env.Lookup(key); ok {
			continue
		}
		if err := env.Set(key, value); err != nil {
			return fmt.Errorf("setenv %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func isValidEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// parseDotEnvValue trims surrounding quotes, strips inline comments on
// unquoted values, and handles empty values.
func parseDotEnvValue(raw string) (string, error) {
	v := strings.TrimLeft(raw, " \t")
	if v == "" {
		return "", nil
	}

	switch v[0] {
	case '"':
		end := strings.LastIndexByte(v, '"')
		if end <= 0 {
			return "", fmt.Errorf("unterminated double-quoted value: %q", raw)
		}
		return v[1:end], nil
	case '\'':
		end := strings.LastIndexByte(v, '\'')
		if end <= 0 {
			return "", fmt.Errorf("unterminated single-quoted value: %q", raw)
		}
		return v[1:end], nil
	}

	if idx := strings.Index(v, " #"); idx >= 0 {
		v = v[:idx]
	} else if idx := strings.Index(v, "\t#"); idx >= 0 {
		v = v[:idx]
	}
	return strings.TrimRight(v, " \t"), nil
}
