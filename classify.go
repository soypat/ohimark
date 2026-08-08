package ohimark

import (
	"unicode"
	"unicode/utf8"
)

// isASCIIPunct reports the 32 ASCII punctuation bytes: exactly the escapable set.
func isASCIIPunct(b byte) bool {
	switch {
	case '!' <= b && b <= '/', ':' <= b && b <= '@',
		'[' <= b && b <= '`', '{' <= b && b <= '~':
		return true
	}
	return false
}

// tokOf maps each byte that begins a token other than TokWord to that token.
// The first four entries name a dispatch rather than a span: how many bytes a
// space run, line ending, escape or entity covers is its reader's to decide.
//
// Digits are deliberately absent: a word in progress absorbs them ("v2" is one
// TokWord), and [Lexer.Next] dispatches a leading digit to TokDigits before
// reaching readWord — all the parser needs to see an ordered list marker.
var tokOf = [256]Token{
	' ': TokSpace, '\t': TokSpace, '\v': TokSpace, '\f': TokSpace,
	'\n': TokNewline, '\r': TokNewline,
	'\\': TokEscape, '&': TokEntity,

	'#': TokHash, '*': TokStar, '_': TokUnderscore, '`': TokTick,
	'~': TokTilde, '-': TokDash, '>': TokGT,

	'+': TokPlus, '!': TokBang, '<': TokLT, '[': TokBracketL,
	']': TokBracketR, '(': TokParenL, ')': TokParenR, '.': TokDot,
}

// isSpecial reports whether b starts a token other than TokWord.
func isSpecial(b byte) bool { return tokOf[b] != TokUndefined }

// isWordRune reports whether r belongs to a TokWord. Every byte special to
// markdown is ASCII and every UTF-8 continuation byte is >= 0x80, so words
// absorb multi-byte sequences without a test.
func isWordRune(r rune) bool { return r >= utf8.RuneSelf || !isSpecial(byte(r)) }

func isDigitRune(r rune) bool { return '0' <= r && r <= '9' }

func isHexRune(r rune) bool {
	return isDigitRune(r) || ('a' <= r && r <= 'f') || ('A' <= r && r <= 'F')
}

func isAlnumRune(r rune) bool {
	return isDigitRune(r) || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}

// isSpaceRune and isPunctRune classify the runes flanking a delimiter run.
// They are the only reason this package links the unicode tables (~10KB of
// rodata); swapping them for ASCII-only equivalents is the single seam for a
// size-constrained target.
func isSpaceRune(r rune) bool {
	return r == '\t' || r == '\n' || r == '\f' || r == '\r' || unicode.IsSpace(r)
}

func isPunctRune(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}
