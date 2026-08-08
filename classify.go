package ohimark

import "unicode"

func isDigit(b byte) bool { return '0' <= b && b <= '9' }

func isLineEnd(b byte) bool { return b == '\n' || b == '\r' }

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

// isASCIIPunct reports the 32 ASCII punctuation bytes: exactly the escapable set.
func isASCIIPunct(b byte) bool {
	switch {
	case '!' <= b && b <= '/', ':' <= b && b <= '@',
		'[' <= b && b <= '`', '{' <= b && b <= '~':
		return true
	}
	return false
}

// special maps each byte that begins a token other than TokWord.
//
// Digits are deliberately absent: a word in progress absorbs them ("v2" is one
// TokWord), and [Lexer.Next] dispatches a leading digit to TokDigits before
// reaching readWord — all the parser needs to see an ordered list marker.
var special = [256]bool{
	' ': true, '\t': true, '\n': true, '\r': true, '\v': true, '\f': true,
	'#': true, '*': true, '_': true, '`': true, '~': true, '-': true, '>': true,
	'+': true, '!': true, '<': true, '[': true, ']': true, '(': true, ')': true,
	'.': true, '\\': true, '&': true,
}

// isSpecial reports whether b starts a token other than TokWord.
func isSpecial(b byte) bool { return special[b] }

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
