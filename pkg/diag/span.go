package diag

import "fmt"

// Position represents a single location within a source file.
type Position struct {
	Filename string
	Line     int // 1-indexed
	Column   int // 1-indexed
	Offset   int // 0-indexed byte offset
}

func (p Position) String() string {
	if p.Filename != "" {
		return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Span represents a contiguous range of source code.
type Span struct {
	Start Position
	End   Position
}

// NewSpan creates a new Span from start and end positions.
func NewSpan(start, end Position) Span {
	return Span{
		Start: start,
		End:   end,
	}
}

// Merge combines two spans into a single span covering both.
func (s Span) Merge(other Span) Span {
	start := s.Start
	if other.Start.Offset < start.Offset {
		start = other.Start
	}

	end := s.End
	if other.End.Offset > end.Offset {
		end = other.End
	}

	return Span{
		Start: start,
		End:   end,
	}
}

func (s Span) String() string {
	if s.Start.Filename != "" {
		return fmt.Sprintf("%s:%d:%d-%d:%d", s.Start.Filename, s.Start.Line, s.Start.Column, s.End.Line, s.End.Column)
	}
	return fmt.Sprintf("%d:%d-%d:%d", s.Start.Line, s.Start.Column, s.End.Line, s.End.Column)
}
