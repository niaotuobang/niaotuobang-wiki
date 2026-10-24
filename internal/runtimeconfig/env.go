// Package runtimeconfig loads only an explicitly selected environment file.
package runtimeconfig

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var envKey = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func LoadEnvFile(path string) error {
	info, e := os.Stat(path)
	if e != nil {
		return fmt.Errorf("cannot read selected env file")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("env file must be a regular file readable only by its owner (0600)")
	}
	f, e := os.Open(path)
	if e != nil {
		return fmt.Errorf("cannot open selected env file")
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 128*1024)
	seen := map[string]bool{}
	line := 0
	for scan.Scan() {
		line++
		s := strings.TrimSpace(scan.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimPrefix(s, "export ")
		key, value, ok := strings.Cut(s, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || !envKey.MatchString(key) || seen[key] {
			return fmt.Errorf("invalid or repeated env key at line %d", line)
		}
		seen[key] = true
		if strings.HasPrefix(value, "\"") {
			value, e = strconv.Unquote(value)
			if e != nil {
				return fmt.Errorf("invalid quoted env value at line %d", line)
			}
		} else if strings.HasPrefix(value, "'") {
			if len(value) < 2 || value[len(value)-1] != '\'' {
				return fmt.Errorf("invalid quoted env value at line %d", line)
			}
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			if e = os.Setenv(key, value); e != nil {
				return fmt.Errorf("cannot set env key at line %d", line)
			}
		}
	}
	if scan.Err() != nil {
		return fmt.Errorf("cannot parse selected env file")
	}
	return nil
}
func EnvFileArgument(args []string) (string, error) {
	path := ""
	for i := 0; i < len(args); i++ {
		s := args[i]
		if strings.HasPrefix(s, "--env-file=") {
			if path != "" {
				return "", fmt.Errorf("env-file may only be specified once")
			}
			path = strings.TrimPrefix(s, "--env-file=")
		} else if s == "--env-file" {
			if path != "" || i+1 == len(args) {
				return "", fmt.Errorf("env-file requires one path")
			}
			i++
			path = args[i]
		}
	}
	return path, nil
}
