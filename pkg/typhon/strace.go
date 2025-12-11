package typhon

import (
	"bufio"
	"io"
	"regexp"
	"sort"
	"strings"
)

// straceLineRegex matches a standard strace line (e.g., `openat(AT_FDCWD, "file", ...) = 3`)
// We capture the syscall name at the beginning.
// Regex: ^[a-zA-Z_0-9]+(?=\()
var straceLineRegex = regexp.MustCompile(`^([a-zA-Z_0-9]+)\(`)

var ignoredSyscalls = map[string]bool{
	"+++": true, // Process exit/signal markers
	"---": true, // Signal markers
}

// AnalyzeStrace parses strace output and returns a sorted list of unique syscalls used.
func AnalyzeStrace(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	syscalls := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Handle unfinished syscalls (e.g. `futex(..., <unfinished ...>`)
		// Often they appear again as `<... futex resumed> ) = 0`
		// We care about the start.

		// Check for signal/exit markers
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}

		matches := straceLineRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			name := matches[1]
			if !ignoredSyscalls[name] {
				syscalls[name] = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var result []string
	for s := range syscalls {
		result = append(result, s)
	}
	sort.Strings(result)

	return result, nil
}
