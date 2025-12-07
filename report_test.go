// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"go/token"
	"strings"
	"testing"
)

func TestSeverityString(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		want     string
	}{
		{
			name:     "SeverityInfo",
			severity: SeverityInfo,
			want:     "INFO",
		},
		{
			name:     "SeverityWarning",
			severity: SeverityWarning,
			want:     "WARNING",
		},
		{
			name:     "SeverityError",
			severity: SeverityError,
			want:     "ERROR",
		},
		{
			name:     "Unknown severity",
			severity: Severity(99),
			want:     "UNKNOWN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.severity.String()
			if got != tt.want {
				t.Errorf("Severity.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetermineSeverity(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
		hasIssue   bool
		want       Severity
	}{
		{
			name:       "no issue returns info",
			confidence: 1.0,
			hasIssue:   false,
			want:       SeverityInfo,
		},
		{
			name:       "high confidence issue returns error",
			confidence: 0.95,
			hasIssue:   true,
			want:       SeverityError,
		},
		{
			name:       "exactly 0.9 confidence returns error",
			confidence: 0.9,
			hasIssue:   true,
			want:       SeverityError,
		},
		{
			name:       "medium confidence issue returns warning",
			confidence: 0.8,
			hasIssue:   true,
			want:       SeverityWarning,
		},
		{
			name:       "exactly 0.7 confidence returns warning",
			confidence: 0.7,
			hasIssue:   true,
			want:       SeverityWarning,
		},
		{
			name:       "low confidence issue returns info",
			confidence: 0.5,
			hasIssue:   true,
			want:       SeverityInfo,
		},
		{
			name:       "zero confidence issue returns info",
			confidence: 0.0,
			hasIssue:   true,
			want:       SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineSeverity(tt.confidence, tt.hasIssue)
			if got != tt.want {
				t.Errorf("DetermineSeverity(%f, %v) = %v, want %v", tt.confidence, tt.hasIssue, got, tt.want)
			}
		})
	}
}

func TestFormatReport(t *testing.T) {
	tests := []struct {
		name     string
		report   Report
		contains []string // Strings that should be in the output
	}{
		{
			name: "basic error report",
			report: Report{
				Pos:          token.Pos(1),
				Message:      "test message",
				Severity:     SeverityError,
				Confidence:   1.0,
				ResourceName: "widget",
			},
			contains: []string{"[ERROR]", "test message"},
		},
		{
			name: "warning with confidence",
			report: Report{
				Pos:          token.Pos(1),
				Message:      "partial match",
				Severity:     SeverityWarning,
				Confidence:   0.75,
				MatchType:    "fuzzy",
				ResourceName: "widget",
			},
			contains: []string{"[WARNING]", "partial match", "75% confidence", "fuzzy match"},
		},
		{
			name: "report with suggestions",
			report: Report{
				Pos:      token.Pos(1),
				Message:  "missing test",
				Severity: SeverityError,
				Suggestions: []string{
					"Create test file",
					"Add TestAcc function",
				},
			},
			contains: []string{"[ERROR]", "missing test", "Suggestions:", "Create test file", "Add TestAcc function"},
		},
		{
			name: "info report no confidence shown",
			report: Report{
				Pos:        token.Pos(1),
				Message:    "info message",
				Severity:   SeverityInfo,
				Confidence: 1.0, // Full confidence should not show percentage
			},
			contains: []string{"[INFO]", "info message"},
		},
		{
			name: "zero confidence not shown",
			report: Report{
				Pos:        token.Pos(1),
				Message:    "zero confidence",
				Severity:   SeverityInfo,
				Confidence: 0.0,
			},
			contains: []string{"[INFO]", "zero confidence"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatReport(tt.report)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatReport() = %q, want to contain %q", got, want)
				}
			}
		})
	}
}

func TestFormatReportNoConfidenceForFullMatch(t *testing.T) {
	report := Report{
		Pos:        token.Pos(1),
		Message:    "test message",
		Severity:   SeverityError,
		Confidence: 1.0, // Full confidence
		MatchType:  "function_name",
	}

	got := FormatReport(report)
	if strings.Contains(got, "confidence") {
		t.Errorf("FormatReport() should not show confidence for 1.0 confidence, got: %q", got)
	}
}

func TestFormatReportShowsPartialConfidence(t *testing.T) {
	report := Report{
		Pos:        token.Pos(1),
		Message:    "test message",
		Severity:   SeverityWarning,
		Confidence: 0.8,
		MatchType:  "file_proximity",
	}

	got := FormatReport(report)
	if !strings.Contains(got, "80% confidence") {
		t.Errorf("FormatReport() should show confidence for 0.8 confidence, got: %q", got)
	}
	if !strings.Contains(got, "file_proximity match") {
		t.Errorf("FormatReport() should show match type, got: %q", got)
	}
}
