package ohimark

import "errors"

var (
	errNilReader     = errors.New("ohimark: nil reader")
	errNegativeOff   = errors.New("ohimark: negative offset")
	errShortWindow   = errors.New("ohimark: window buffer under MinWindow")
	errDepthExceeded = errors.New("ohimark: container nesting exceeds MaxDepth")
)

// ErrLiteralUnavailable reports that a node's bytes are not in the parser's
// window, so [Parser.TryNextLiteral] could not hand them back without a read.
var ErrLiteralUnavailable = errors.New("ohimark: node literal not resident in window")

// SyntaxError describes a lexing or parsing failure at an exact byte offset.
type SyntaxError struct {
	Source string
	Off    Pos
	Msg    string
}

func (e *SyntaxError) Error() string {
	return "ohimark: " + e.Source + " " + e.Off.String() + ": " + e.Msg
}
