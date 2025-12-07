// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"fmt"
	"go/token"
)

// Severity indicates the importance of a report.
type Severity int

const (
	// SeverityInfo indicates an informational finding.
	SeverityInfo Severity = iota
	// SeverityWarning indicates a potential issue.
	SeverityWarning
	// SeverityError indicates a definite issue.
	SeverityError
)

// String returns the string representation of a Severity.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Report represents a single analysis finding.
type Report struct {
	// Pos is the position in the source code where the issue was found.
	Pos token.Pos
	// Message is the human-readable description of the issue.
	Message string
	// Severity indicates the importance of this finding.
	Severity Severity
	// Confidence is the confidence level of the match (0.0-1.0).
	Confidence float64
	// MatchType describes how the match was determined.
	MatchType string
	// ResourceName is the name of the resource this finding relates to.
	ResourceName string
	// Suggestions provides actionable suggestions to fix the issue.
	Suggestions []string
}

// DetermineSeverity calculates severity based on confidence and whether there's an issue.
// High confidence (>=0.9) issues are errors, medium confidence (>=0.7) are warnings,
// and low confidence or non-issues are informational.
func DetermineSeverity(confidence float64, hasIssue bool) Severity {
	if !hasIssue {
		return SeverityInfo
	}

	switch {
	case confidence >= 0.9:
		return SeverityError
	case confidence >= 0.7:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// FormatReport formats a report for output.
// It includes the severity, message, confidence level, match type, and suggestions.
func FormatReport(r Report) string {
	msg := fmt.Sprintf("[%s] %s", r.Severity, r.Message)

	if r.Confidence > 0 && r.Confidence < 1.0 {
		msg += fmt.Sprintf(" (%.0f%% confidence, %s match)", r.Confidence*100, r.MatchType)
	}

	if len(r.Suggestions) > 0 {
		msg += "\n  Suggestions:"
		for _, s := range r.Suggestions {
			msg += "\n    - " + s
		}
	}

	return msg
}
