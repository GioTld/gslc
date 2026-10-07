package diag

import (
	"fmt"
	"strings"
)

// Severity indicates the seriousness of a diagnostic message.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityNote
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityNote:
		return "note"
	default:
		return "unknown"
	}
}

// Diagnostic represents a compiler error, warning, or informational note with source context.
type Diagnostic struct {
	Severity Severity
	Code     string
	Message  string
	Span     Span
	Hint     string
}

const (
	UndefinedIdentifier = "E2001"
	ArgumentCount       = "E2002"
	ImmutableBinding    = "E2003"
	UnsafeOperation     = "E2004"
)

type Record struct {
	Code string `json:"code"`
	Span struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"span"`
	Message string `json:"message"`
}

func (d Diagnostic) WithCode(code string) Diagnostic {
	d.Code = code
	return d
}

// Record excludes presentation details and uses half-open byte offsets.
func (d Diagnostic) Record(fallback string) Record {
	code := d.Code
	if code == "" {
		code = fallback
	}
	messages := map[string]string{
		"E1000": "syntax error", "E2000": "semantic error",
		UndefinedIdentifier: "undefined identifier", ArgumentCount: "argument count mismatch",
		ImmutableBinding: "immutable binding", UnsafeOperation: "unsafe operation",
	}
	result := Record{Code: code, Message: messages[code]}
	if result.Message == "" {
		result.Message = d.Message
	}
	result.Span.Start, result.Span.End = d.Span.Start.Offset, d.Span.End.Offset
	return result
}

// NewError creates an error diagnostic.
func NewError(span Span, msg string) Diagnostic {
	return Diagnostic{
		Severity: SeverityError,
		Message:  msg,
		Span:     span,
	}
}

// NewWarning creates a warning diagnostic.
func NewWarning(span Span, msg string) Diagnostic {
	return Diagnostic{
		Severity: SeverityWarning,
		Message:  msg,
		Span:     span,
	}
}

// WithHint attaches an advisory hint to the diagnostic.
func (d Diagnostic) WithHint(hint string) Diagnostic {
	d.Hint = hint
	return d
}

// Format renders the diagnostic into a human-readable snippet using the original source text.
func (d Diagnostic) Format(source string) string {
	var sb strings.Builder

	// Header: error: message
	sb.WriteString(fmt.Sprintf("%s: %s\n", d.Severity, d.Message))

	filename := d.Span.Start.Filename
	if filename == "" {
		filename = "<input>"
	}

	// Location indicator: --> file:line:col
	sb.WriteString(fmt.Sprintf("  --> %s:%d:%d\n", filename, d.Span.Start.Line, d.Span.Start.Column))

	lines := strings.Split(source, "\n")
	lineIdx := d.Span.Start.Line - 1

	if lineIdx >= 0 && lineIdx < len(lines) {
		lineContent := lines[lineIdx]
		lineNumStr := fmt.Sprintf("%d", d.Span.Start.Line)
		padding := strings.Repeat(" ", len(lineNumStr))

		sb.WriteString(fmt.Sprintf("   %s |\n", padding))
		sb.WriteString(fmt.Sprintf("   %s | %s\n", lineNumStr, lineContent))

		// Underline carets
		col := d.Span.Start.Column
		if col < 1 {
			col = 1
		}
		caretIndent := strings.Repeat(" ", col-1)

		underlineLen := 1
		if d.Span.End.Line == d.Span.Start.Line && d.Span.End.Column > d.Span.Start.Column {
			underlineLen = d.Span.End.Column - d.Span.Start.Column
		}
		carets := strings.Repeat("^", underlineLen)

		sb.WriteString(fmt.Sprintf("   %s | %s%s\n", padding, caretIndent, carets))
	}

	if d.Hint != "" {
		sb.WriteString(fmt.Sprintf("   = hint: %s\n", d.Hint))
	}

	return sb.String()
}
