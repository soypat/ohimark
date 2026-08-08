package ohimark

//go:generate stringer -linecomment -type=Token,Kind -output=stringers.go

// Token is a lexical token of markdown syntax. Tokens carry no structural
// meaning: TokHash is a run of '#' whether or not it heads a heading.
//
// Run length is the span length, never the identity. One TokStar, not
// Star/Star2/Star3: runs are arbitrarily long and the emphasis rules need the
// count and its value mod 3.
type Token uint8

const (
	TokUndefined Token = iota // undefined
	TokIllegal                // illegal
	TokEOF                    // EOF

	TokWord    // <word>
	TokDigits  // <digits>
	TokSpace   // <space>
	TokNewline // <newline>

	// Delimiter runs. Length is End-Start.
	runBeg
	TokHash       // #
	TokStar       // *
	TokUnderscore // _
	TokTick       // `
	TokTilde      // ~
	TokDash       // -
	TokGT         // >
	runEnd

	// Single-byte punctuation. Most are structural only in a specific parser
	// context — TokDot after TokDigits is an ordered marker, TokDot in prose is
	// not. That costs nothing: a token the parser does not record as a delimiter
	// falls into the gap between delimiters, and gaps merge into one KindText.
	TokPlus     // +
	TokBang     // !
	TokLT       // <
	TokBracketL // [
	TokBracketR // ]
	TokParenL   // (
	TokParenR   // )
	TokDot      // .

	// Composite. The span covers every byte, leading '\' or '&' included.
	TokEscape // <escape>
	TokEntity // <entity>
)

// IsRun reports whether tok is a length-carrying delimiter run.
func (tok Token) IsRun() bool { return tok > runBeg && tok < runEnd }

// IsSpace reports whether tok separates content without being content.
func (tok Token) IsSpace() bool {
	return tok == TokSpace || tok == TokNewline || tok == TokEOF
}
