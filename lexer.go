package ohimark

import (
	"io"
	"unicode/utf8"

	"github.com/soypat/lexorg"
)

// MinWindow is the smallest window buffer [Lexer.Reset] accepts. Must exceed
// utf8.UTFMax so a sequence is always holdable whole.
const MinWindow = 64

const (
	defaultWindowSize = 4096
	tabStop           = 4
	// peeklen is the rune lookahead depth. "\r\n" and "\x" need 2; the rest is
	// headroom. Entities outrun any such buffer, so readEntity rewinds instead.
	peeklen = 4
)

// Lexer tokenizes markdown. It is context-free: a '*' inside a fenced code
// block is TokStar just as in a paragraph, and deciding it is literal there is
// the [Parser]'s job. Holding no state that grows with input is the point.
//
// The cursor holds decoded runes, not bytes. Tokenizing never needs the decode
// — every byte special to markdown is ASCII — but emphasis does: whether '*'
// opens or closes turns on the Unicode class of the flanking characters. Runes
// make [Lexer.PrevRune] and [Lexer.PeekRune] field reads instead of decodes,
// the backwards one over bytes already passed.
//
// Errors are sticky.
type Lexer struct {
	w   lexorg.WindowReader
	src string

	// peek[0] is the current rune, size[i] its encoded length, pos peek[0]'s
	// absolute offset. The sizes are what yield a token's span without a second
	// cursor to keep in step with the window's.
	peek [peeklen]rune
	size [peeklen]int8
	n    int8
	pos  int64

	// eof stops fill retrying. Not just an optimization: lexorg.WindowReader.Fill
	// zeroes its cursor even when the fill is refused, rewinding Offset() into
	// the previous window, so a retry past EOF re-reads stale bytes.
	eof bool

	// prev is the rune before peek[0]; tokPrev the one before the token Next
	// last returned. They differ because a token is many runes wide.
	prev    rune
	tokPrev rune

	col int32 // 0-based column of peek[0], tabs expanded.

	// bufcap is the fill size last supplied. lexorg.WindowReader forwards none
	// of Window's buffer accessors, so there is no asking whether it has one.
	bufcap int32

	err error
}

// Reset points the lexer at offset off of r, reading through buf. buf must be
// at least [MinWindow] bytes; nil keeps the previous buffer, or allocates a
// default one on first use. Bytes buffered by an earlier Reset over the same r
// are reused, which is what makes [Lexer.Seek] cheap.
func (l *Lexer) Reset(source string, r io.ReaderAt, off int64, buf []byte) error {
	switch {
	case r == nil:
		return errNilReader
	case off < 0:
		return errNegativeOff
	case buf != nil && len(buf) < MinWindow:
		return errShortWindow
	}
	if buf == nil && l.bufcap == 0 {
		buf = make([]byte, defaultWindowSize) // lexorg never allocates.
	}
	if buf != nil {
		l.bufcap = int32(len(buf))
	}
	l.w.Reset(r, buf, off)
	l.src = source
	l.err = nil
	l.reposition(off)
	return l.Err()
}

// Seek moves the cursor to p without rebinding the reader. A p inside the
// resident bytes costs no read.
//
// The column is unrecoverable without scanning from the line start, so Seek
// reports p as column 0.
func (l *Lexer) Seek(p Pos) error {
	if p < 0 {
		return errNegativeOff
	}
	l.rewind(int64(p), 0, 0)
	return l.Err()
}

// rewind is Seek carrying the column and previous rune back with it, for a scan
// that gave up and must retokenize bytes it already walked.
func (l *Lexer) rewind(off int64, col int32, prev rune) {
	// Resetting to the bound reader keeps the resident bytes, so a rewind inside
	// them costs no read. It also clears the window's recorded EOF, which is
	// what lets the cursor move back over the end of the file.
	l.w.Reset(l.w.ReaderAt(), nil, off)
	l.reposition(off)
	l.col, l.prev = col, prev
}

// reposition drops the peek buffer and refills from off.
func (l *Lexer) reposition(off int64) {
	l.pos = off
	l.n = 0
	l.col = 0
	l.eof = false
	l.prev, l.tokPrev = 0, 0
	l.fill()
}

// Next returns the next token and its span. Spans tile the source exactly: no
// gaps, no overlaps. TokEOF at end of input, TokIllegal on a read failure; both
// have an empty span at the cursor.
func (l *Lexer) Next() (tok Token, start, end Pos) {
	// Captured before consuming: by the time this returns, prev has walked to the
	// token's last rune, which is not what the flanking rules ask about.
	l.tokPrev = l.prev
	start = Pos(l.pos)
	if l.n == 0 {
		if l.Err() != nil {
			return TokIllegal, start, start
		}
		return TokEOF, start, start
	}
	r := l.peek[0]
	switch {
	case isDigitRune(r):
		return TokDigits, start, l.readDigits()
	case isWordRune(r):
		return TokWord, start, l.readWord()
	}
	switch tok = tokOf[byte(r)]; tok {
	case TokSpace:
		return tok, start, l.readSpace()
	case TokNewline:
		return tok, start, l.readNewline()
	case TokEscape:
		if end, ok := l.readEscape(); ok {
			return tok, start, end
		}
	case TokEntity:
		if end, ok := l.readEntity(); ok {
			return tok, start, end
		}
	case TokPlus, TokBang, TokLT, TokBracketL, TokBracketR, TokParenL, TokParenR, TokDot:
		l.advance()
		return tok, start, Pos(l.pos)
	default:
		return tok, start, l.readRun(r)
	}
	// A backslash or ampersand that began no construct is ordinary text, and
	// readWord takes the first rune whatever it is.
	return TokWord, start, l.readWord()
}

// Err returns the lexer error, or nil if the input merely ended.
func (l *Lexer) Err() error {
	if l.err != nil {
		return l.err
	}
	// lexorg records io.EOF verbatim despite what Window.Err documents.
	if err := l.w.Err(); err != io.EOF {
		return err
	}
	return nil
}

// Pos returns the absolute byte offset of the current rune.
func (l *Lexer) Pos() Pos { return Pos(l.pos) }

// Col returns the 0-based column of the current rune, tabs expanded to 4-column
// stops. A multi-byte rune is one column.
func (l *Lexer) Col() int { return int(l.col) }

// AtLineStart reports whether the current rune begins a line. Column based, so
// it also reports true after a Reset or Seek that landed mid-line.
func (l *Lexer) AtLineStart() bool { return l.col == 0 }

// IsDone reports whether the lexer has no current rune.
func (l *Lexer) IsDone() bool { return l.n == 0 }

// PrevRune returns the rune before the token [Lexer.Next] last returned, and
// [Lexer.PeekRune] the one after it — what the emphasis flanking rules ask
// about a delimiter run. Both are 0 at the edges of the file, which those rules
// read as whitespace.
func (l *Lexer) PrevRune() rune { return l.tokPrev }

// PeekRune returns the current rune. See [Lexer.PrevRune].
func (l *Lexer) PeekRune() rune { return l.PeekAt(0) }

// PeekAt returns the i'th rune ahead of the cursor, or 0 past end of input.
// i must be less than peeklen.
func (l *Lexer) PeekAt(i int) rune {
	if i >= int(l.n) {
		return 0
	}
	return l.peek[i]
}

// fill tops the peek buffer back up.
func (l *Lexer) fill() {
	for !l.eof && l.n < peeklen {
		r, size, err := l.w.ReadRune()
		if err != nil {
			l.eof = true
			if err != io.EOF && l.err == nil {
				l.err = err
			}
			return
		}
		l.peek[l.n] = r
		l.size[l.n] = int8(size)
		l.n++
	}
}

// advance consumes the current rune.
func (l *Lexer) advance() {
	if l.n == 0 {
		return
	}
	l.prev = l.peek[0]
	l.pos += int64(l.size[0])
	l.advanceCol(l.peek[0])
	copy(l.peek[:], l.peek[1:])
	copy(l.size[:], l.size[1:])
	l.n--
	l.fill()
}

// advanceCol moves the column past r. Tabs jump to the next stop, line endings
// restart; every other rune is one column however many bytes it encodes to.
func (l *Lexer) advanceCol(r rune) {
	switch r {
	case '\n', '\r':
		l.col = 0
	case '\t':
		l.col += tabStop - l.col%tabStop
	default:
		l.col++
	}
}

func (l *Lexer) peekIs(i int, r rune) bool { return l.PeekAt(i) == r }

// SkipLine advances past the current line ending and returns the next line's
// first offset, or EOF. Lets the parser jump a fenced body without tokenizing
// bytes that are verbatim by definition.
func (l *Lexer) SkipLine() Pos {
	for l.n > 0 {
		r := l.peek[0]
		l.advance()
		if r == '\n' {
			break
		}
		if r == '\r' {
			if l.peekIs(0, '\n') {
				l.advance()
			}
			break
		}
	}
	return Pos(l.pos)
}

// readRun consumes a maximal run of r, returning the offset past it.
func (l *Lexer) readRun(r rune) Pos {
	for l.n > 0 && l.peek[0] == r {
		l.advance()
	}
	return Pos(l.pos)
}

// readWord consumes a maximal run of runes special to nothing. Digits are not
// special, so a word in progress absorbs them. The first rune goes in
// unconditionally: [Lexer.Next] also lands here for a backslash or ampersand
// that began no construct.
func (l *Lexer) readWord() Pos {
	l.advance()
	for l.n > 0 && isWordRune(l.peek[0]) {
		l.advance()
	}
	return Pos(l.pos)
}

// readDigits consumes a maximal run of '0'..'9'.
func (l *Lexer) readDigits() Pos {
	for l.n > 0 && isDigitRune(l.peek[0]) {
		l.advance()
	}
	return Pos(l.pos)
}

// readSpace consumes a maximal run of whitespace that does not end a line.
func (l *Lexer) readSpace() Pos {
	for l.n > 0 && l.peek[0] < utf8.RuneSelf && tokOf[byte(l.peek[0])] == TokSpace {
		l.advance()
	}
	return Pos(l.pos)
}

// readNewline consumes one line ending; "\r\n" counts as one.
func (l *Lexer) readNewline() Pos {
	cr := l.peek[0] == '\r'
	l.advance()
	if cr && l.peekIs(0, '\n') {
		l.advance()
	}
	return Pos(l.pos)
}

// readEscape consumes a backslash plus one ASCII punctuation rune. False for a
// backslash before anything else, which is literal text.
func (l *Lexer) readEscape() (end Pos, ok bool) {
	r := l.PeekAt(1)
	if r >= utf8.RuneSelf || !isASCIIPunct(byte(r)) {
		return Pos(l.pos), false
	}
	l.advance()
	l.advance()
	return Pos(l.pos), true
}

// maxEntityName bounds a named entity: "CounterClockwiseContourIntegral" is the
// longest CommonMark knows, at 31 bytes.
const maxEntityName = 31

// readEntity consumes "&name;", "&#dd;" or "&#xhh;". An entity outruns the peek
// buffer, so this consumes under a length bound and rewinds on failure — free,
// since those bytes are still resident.
func (l *Lexer) readEntity() (end Pos, ok bool) {
	mark, col, prev := l.pos, l.col, l.prev
	if l.readEntityBody() {
		return Pos(l.pos), true
	}
	l.rewind(mark, col, prev)
	return Pos(mark), false
}

func (l *Lexer) readEntityBody() bool {
	l.advance() // '&'
	digit, max := isAlnumRune, maxEntityName
	if l.peekIs(0, '#') {
		l.advance()
		digit, max = isDigitRune, 7
		if l.peekIs(0, 'x') || l.peekIs(0, 'X') {
			l.advance()
			digit, max = isHexRune, 6
		}
	}
	n := 0
	for n < max && l.n > 0 && digit(l.peek[0]) {
		l.advance()
		n++
	}
	if n == 0 || !l.peekIs(0, ';') {
		return false
	}
	l.advance()
	return true
}

// errAt latches a *SyntaxError; the first error wins.
func (l *Lexer) errAt(pos Pos, msg string) {
	if l.err == nil {
		l.err = &SyntaxError{Source: l.src, Off: pos, Msg: msg}
	}
}
