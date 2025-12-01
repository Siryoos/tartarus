package erebus

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Scanner defines the interface for vulnerability scanning
type Scanner interface {
	Scan(ctx context.Context, path string) error
}

// Severity represents vulnerability severity levels
type Severity string

const (
	SeverityUnknown  Severity = "UNKNOWN"
	SeverityLow      Severity = "LOW"
	SeverityMedium   Severity = "MEDIUM"
	SeverityHigh     Severity = "HIGH"
	SeverityCritical Severity = "CRITICAL"
)

// ScanResult represents structured scan findings
type ScanResult struct {
	Target          string                 `json:"Target"`
	Vulnerabilities []VulnerabilityDetails `json:"Vulnerabilities"`
}

// VulnerabilityDetails represents a single vulnerability
type VulnerabilityDetails struct {
	VulnerabilityID  string   `json:"VulnerabilityID"`
	PkgName          string   `json:"PkgName"`
	InstalledVersion string   `json:"InstalledVersion"`
	FixedVersion     string   `json:"FixedVersion"`
	Severity         Severity `json:"Severity"`
	Title            string   `json:"Title"`
}

// TrivyScanner implements Scanner using the trivy CLI
type TrivyScanner struct {
	BinaryPath     string     // Path to trivy binary, defaults to "trivy"
	MinSeverity    Severity   // Minimum severity to fail on (CRITICAL, HIGH, MEDIUM, LOW)
	Severities     []Severity // List of severities to scan for
	ExitCodeOnFail bool       // Whether to use --exit-code flag
	OutputFormat   string     // Output format: json, table, etc.
}

func NewTrivyScanner() *TrivyScanner {
	return &TrivyScanner{
		BinaryPath:     "trivy",
		MinSeverity:    SeverityCritical,
		Severities:     []Severity{SeverityCritical},
		ExitCodeOnFail: true,
		OutputFormat:   "json",
	}
}

// NewTrivyScannerWithSeverities creates a scanner with custom severity levels
func NewTrivyScannerWithSeverities(severities []Severity) *TrivyScanner {
	return &TrivyScanner{
		BinaryPath:     "trivy",
		Severities:     severities,
		ExitCodeOnFail: true,
		OutputFormat:   "json",
	}
}

func (s *TrivyScanner) Scan(ctx context.Context, path string) error {
	// Build severity list
	severityList := make([]string, len(s.Severities))
	for i, sev := range s.Severities {
		severityList[i] = string(sev)
	}

	args := []string{
		"fs",
		"--severity", strings.Join(severityList, ","),
		"--format", s.OutputFormat,
		"--no-progress",
		"--quiet",
		path,
	}

	if s.ExitCodeOnFail {
		args = append([]string{"fs", "--exit-code", "1"}, args[1:]...)
	}

	cmd := exec.CommandContext(ctx, s.BinaryPath, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Parse JSON output for detailed error info if available
		if s.OutputFormat == "json" && len(output) > 0 {
			var results struct {
				Results []ScanResult `json:"Results"`
			}
			if parseErr := json.Unmarshal(output, &results); parseErr == nil {
				vulnCount := 0
				for _, result := range results.Results {
					vulnCount += len(result.Vulnerabilities)
				}
				return fmt.Errorf("vulnerability scan failed: found %d vulnerabilities in %s\nOutput: %s", vulnCount, path, string(output))
			}
		}
		return fmt.Errorf("vulnerability scan failed: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// ScanWithResults performs a scan and returns structured results
func (s *TrivyScanner) ScanWithResults(ctx context.Context, path string) ([]ScanResult, error) {
	// Force JSON output for parsing
	originalFormat := s.OutputFormat
	s.OutputFormat = "json"
	defer func() { s.OutputFormat = originalFormat }()

	args := []string{
		"fs",
		"--severity", strings.Join(severitiesList(s.Severities), ","),
		"--format", "json",
		"--no-progress",
		"--quiet",
		path,
	}

	cmd := exec.CommandContext(ctx, s.BinaryPath, args...)
	output, err := cmd.CombinedOutput()

	var results struct {
		Results []ScanResult `json:"Results"`
	}

	if len(output) > 0 {
		if parseErr := json.Unmarshal(output, &results); parseErr != nil {
			return nil, fmt.Errorf("failed to parse scan results: %w", parseErr)
		}
	}

	if err != nil {
		return results.Results, fmt.Errorf("scan completed with errors: %w", err)
	}

	return results.Results, nil
}

func severitiesList(severities []Severity) []string {
	result := make([]string, len(severities))
	for i, sev := range severities {
		result[i] = string(sev)
	}
	return result
}

// MockScanner for testing
type MockScanner struct {
	ShouldFail bool
}

func (m *MockScanner) Scan(ctx context.Context, path string) error {
	if m.ShouldFail {
		return fmt.Errorf("mock scan failed")
	}
	return nil
}
