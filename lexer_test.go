package ohimark

import (
	"errors"
	"strings"
	"testing"
)

// tokexp is one expected token with its span.
type tokexp struct {
	tok        Token
	start, end Pos
}

func tk(t Token, start, end int) tokexp { return tokexp{t, Pos(start), Pos(end)} }

var lexTests = []struct {
	name string
	src  string
	want []tokexp
}{
	{"empty", "", nil},

	// --- words ---
	{"word", "hello", []tokexp{tk(TokWord, 0, 5)}},
	{"word absorbs digits", "v2", []tokexp{tk(TokWord, 0, 2)}},
	{"word absorbs colon slash", "http://a", []tokexp{tk(TokWord, 0, 8)}},
	{"utf8 word", "héllo", []tokexp{tk(TokWord, 0, 6)}},
	{"utf8 3byte", "a€b", []tokexp{tk(TokWord, 0, 5)}},
	{"utf8 4byte", "a\U0001F600b", []tokexp{tk(TokWord, 0, 6)}},

	// --- digits ---
	{"digits", "123", []tokexp{tk(TokDigits, 0, 3)}},
	{"digits then word", "2v", []tokexp{tk(TokDigits, 0, 1), tk(TokWord, 1, 2)}},
	{"ordered dot", "1. x", []tokexp{
		tk(TokDigits, 0, 1), tk(TokDot, 1, 2), tk(TokSpace, 2, 3), tk(TokWord, 3, 4),
	}},
	{"ordered paren", "10) x", []tokexp{
		tk(TokDigits, 0, 2), tk(TokParenR, 2, 3), tk(TokSpace, 3, 4), tk(TokWord, 4, 5),
	}},

	// --- space and newlines ---
	{"space run", "  \t x", []tokexp{tk(TokSpace, 0, 4), tk(TokWord, 4, 5)}},
	{"vertical space is space", "a\v\fb", []tokexp{
		tk(TokWord, 0, 1), tk(TokSpace, 1, 3), tk(TokWord, 3, 4),
	}},
	{"lf", "a\nb", []tokexp{tk(TokWord, 0, 1), tk(TokNewline, 1, 2), tk(TokWord, 2, 3)}},
	{"crlf is one", "a\r\nb", []tokexp{tk(TokWord, 0, 1), tk(TokNewline, 1, 3), tk(TokWord, 3, 4)}},
	{"cr alone", "a\rb", []tokexp{tk(TokWord, 0, 1), tk(TokNewline, 1, 2), tk(TokWord, 2, 3)}},
	{"blank lines are separate newlines", "\n\n", []tokexp{
		tk(TokNewline, 0, 1), tk(TokNewline, 1, 2),
	}},
	{"crlf crlf", "\r\n\r\n", []tokexp{tk(TokNewline, 0, 2), tk(TokNewline, 2, 4)}},

	// --- runs ---
	{"hash run", "### x", []tokexp{
		tk(TokHash, 0, 3), tk(TokSpace, 3, 4), tk(TokWord, 4, 5),
	}},
	{"star run", "**bold**", []tokexp{
		tk(TokStar, 0, 2), tk(TokWord, 2, 6), tk(TokStar, 6, 8),
	}},
	{"underscore run", "___", []tokexp{tk(TokUnderscore, 0, 3)}},
	{"tick run", "```go", []tokexp{tk(TokTick, 0, 3), tk(TokWord, 3, 5)}},
	{"tilde run", "~~~", []tokexp{tk(TokTilde, 0, 3)}},
	{"dash run", "---", []tokexp{tk(TokDash, 0, 3)}},
	{"gt run", ">> x", []tokexp{
		tk(TokGT, 0, 2), tk(TokSpace, 2, 3), tk(TokWord, 3, 4),
	}},
	{"long run", strings.Repeat("*", 200), []tokexp{tk(TokStar, 0, 200)}},

	// --- single byte punctuation ---
	{"brackets and parens", "[a](b)", []tokexp{
		tk(TokBracketL, 0, 1), tk(TokWord, 1, 2), tk(TokBracketR, 2, 3),
		tk(TokParenL, 3, 4), tk(TokWord, 4, 5), tk(TokParenR, 5, 6),
	}},
	{"bang", "![a]", []tokexp{
		tk(TokBang, 0, 1), tk(TokBracketL, 1, 2), tk(TokWord, 2, 3), tk(TokBracketR, 3, 4),
	}},
	{"plus does not run", "++", []tokexp{tk(TokPlus, 0, 1), tk(TokPlus, 1, 2)}},
	{"autolink shape", "<http://a>", []tokexp{
		tk(TokLT, 0, 1), tk(TokWord, 1, 9), tk(TokGT, 9, 10),
	}},
	{"dot does not run", "..", []tokexp{tk(TokDot, 0, 1), tk(TokDot, 1, 2)}},

	// --- escapes ---
	{"escape star", `\*`, []tokexp{tk(TokEscape, 0, 2)}},
	{"escape backslash", `\\`, []tokexp{tk(TokEscape, 0, 2)}},
	{"escape then star", `\**`, []tokexp{tk(TokEscape, 0, 2), tk(TokStar, 2, 3)}},
	{"backslash before letter is text", `\a`, []tokexp{tk(TokWord, 0, 2)}},
	{"backslash before newline is text", "\\\n", []tokexp{
		tk(TokWord, 0, 1), tk(TokNewline, 1, 2),
	}},
	{"trailing backslash", `\`, []tokexp{tk(TokWord, 0, 1)}},
	{"backslash before utf8 is text", "\\é", []tokexp{tk(TokWord, 0, 3)}},

	// --- entities ---
	{"named entity", "&amp;", []tokexp{tk(TokEntity, 0, 5)}},
	{"decimal entity", "&#39;", []tokexp{tk(TokEntity, 0, 5)}},
	{"hex entity", "&#x2F;", []tokexp{tk(TokEntity, 0, 6)}},
	{"hex entity upper", "&#X2f;", []tokexp{tk(TokEntity, 0, 6)}},
	{"entity then word", "&amp;x", []tokexp{tk(TokEntity, 0, 5), tk(TokWord, 5, 6)}},
	{"unterminated entity is text", "&amp", []tokexp{tk(TokWord, 0, 4)}},
	{"lone ampersand", "&", []tokexp{tk(TokWord, 0, 1)}},
	{"ampersand then space", "& x", []tokexp{
		tk(TokWord, 0, 1), tk(TokSpace, 1, 2), tk(TokWord, 2, 3),
	}},
	// A failed entity falls back to a word, which then absorbs everything not
	// special — ';' included.
	{"empty entity is text", "&;", []tokexp{tk(TokWord, 0, 2)}},
	{"overlong entity is text", "&" + strings.Repeat("a", 64) + ";", []tokexp{
		tk(TokWord, 0, 66),
	}},
	{"entity with punct inside is text", "&a*;", []tokexp{
		tk(TokWord, 0, 2), tk(TokStar, 2, 3), tk(TokWord, 3, 4),
	}},

	// --- mixed ---
	{"quoted list emph", "> - a *b* c", []tokexp{
		tk(TokGT, 0, 1), tk(TokSpace, 1, 2), tk(TokDash, 2, 3), tk(TokSpace, 3, 4),
		tk(TokWord, 4, 5), tk(TokSpace, 5, 6), tk(TokStar, 6, 7), tk(TokWord, 7, 8),
		tk(TokStar, 8, 9), tk(TokSpace, 9, 10), tk(TokWord, 10, 11),
	}},
	{"heading then paragraph", "# T\n\ntext\n", []tokexp{
		tk(TokHash, 0, 1), tk(TokSpace, 1, 2), tk(TokWord, 2, 3), tk(TokNewline, 3, 4),
		tk(TokNewline, 4, 5), tk(TokWord, 5, 9), tk(TokNewline, 9, 10),
	}},
}

// windowSizes exercises the same input against a buffer that forces many refills
// and one that holds everything. Both streams must be identical: no construct
// may depend on residency.
var windowSizes = []int{MinWindow, 64 << 10}

func lexAll(t *testing.T, src string, bufSize int) []tokexp {
	t.Helper()
	var l Lexer
	if err := l.Reset(t.Name(), strings.NewReader(src), 0, make([]byte, bufSize)); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	var got []tokexp
	for range len(src) + 2 {
		tok, start, end := l.Next()
		if tok == TokEOF {
			if start != Pos(len(src)) || end != Pos(len(src)) {
				t.Errorf("TokEOF span %v..%v, want %d..%d", start, end, len(src), len(src))
			}
			return got
		}
		if tok == TokIllegal || tok == TokUndefined {
			t.Fatalf("got %s at %v..%v: %v", tok, start, end, l.Err())
		}
		got = append(got, tokexp{tok, start, end})
	}
	t.Fatalf("Next did not terminate over %q", src)
	return nil
}

func TestLexerTable(t *testing.T) {
	for _, tt := range lexTests {
		t.Run(tt.name, func(t *testing.T) {
			for _, bufSize := range windowSizes {
				got := lexAll(t, tt.src, bufSize)
				if len(got) != len(tt.want) {
					t.Fatalf("buf=%d got %d tokens, want %d\ngot:  %v\nwant: %v",
						bufSize, len(got), len(tt.want), got, tt.want)
				}
				for i := range got {
					if got[i] != tt.want[i] {
						t.Errorf("buf=%d token %d = %v, want %v", bufSize, i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

// TestLexerCoverage is the whole correctness condition: the token spans,
// concatenated in order, reproduce the source byte for byte.
func TestLexerCoverage(t *testing.T) {
	for _, tt := range lexTests {
		t.Run(tt.name, func(t *testing.T) {
			for _, bufSize := range windowSizes {
				got := lexAll(t, tt.src, bufSize)
				var at Pos
				for i, g := range got {
					if g.start != at {
						t.Fatalf("buf=%d token %d (%s) starts at %v, want %v",
							bufSize, i, g.tok, g.start, at)
					}
					if g.end <= g.start {
						t.Fatalf("buf=%d token %d (%s) has empty span %v..%v",
							bufSize, i, g.tok, g.start, g.end)
					}
					at = g.end
				}
				if at != Pos(len(tt.src)) {
					t.Fatalf("buf=%d spans cover %v bytes, want %d", bufSize, at, len(tt.src))
				}
			}
		})
	}
}

func TestLexerEOFIsSticky(t *testing.T) {
	var l Lexer
	if err := l.Reset("t", strings.NewReader("ab"), 0, nil); err != nil {
		t.Fatal(err)
	}
	if tok, _, _ := l.Next(); tok != TokWord {
		t.Fatalf("got %s, want TokWord", tok)
	}
	for i := range 5 {
		tok, start, end := l.Next()
		if tok != TokEOF || start != 2 || end != 2 {
			t.Fatalf("call %d: got %s %v..%v, want TokEOF 2..2", i, tok, start, end)
		}
	}
	if err := l.Err(); err != nil {
		t.Fatalf("Err after clean EOF = %v, want nil", err)
	}
}

func TestLexerFlankingRunes(t *testing.T) {
	// PrevRune is the rune before the token Next last returned; PeekRune the one
	// after it. Both 0 at the file edges.
	const src = "a*b"
	var l Lexer
	if err := l.Reset("t", strings.NewReader(src), 0, nil); err != nil {
		t.Fatal(err)
	}
	l.Next() // TokWord "a"
	if got := l.PrevRune(); got != 0 {
		t.Errorf("PrevRune at file start = %q, want 0", got)
	}
	if got := l.PeekRune(); got != '*' {
		t.Errorf("PeekRune after first word = %q, want '*'", got)
	}
	l.Next() // TokStar
	if got := l.PrevRune(); got != 'a' {
		t.Errorf("PrevRune before star = %q, want 'a'", got)
	}
	if got := l.PeekRune(); got != 'b' {
		t.Errorf("PeekRune after star = %q, want 'b'", got)
	}
	l.Next() // TokWord "b"
	if got := l.PeekRune(); got != 0 {
		t.Errorf("PeekRune at file end = %q, want 0", got)
	}
}

func TestLexerFlankingUTF8(t *testing.T) {
	// The reason the cursor carries runes at all: an em dash flanks differently
	// than a letter, and neither is decodable from the token stream.
	const src = "—*x"
	var l Lexer
	if err := l.Reset("t", strings.NewReader(src), 0, nil); err != nil {
		t.Fatal(err)
	}
	l.Next() // TokWord "—"
	l.Next() // TokStar
	if got := l.PrevRune(); got != '—' {
		t.Errorf("PrevRune = %q, want em dash", got)
	}
	if !isPunctRune(l.PrevRune()) {
		t.Error("em dash should classify as punctuation")
	}
}

func TestLexerSkipLine(t *testing.T) {
	for _, tt := range []struct {
		src   string
		steps []Pos // Offset returned by each successive SkipLine.
	}{
		{"a\nb\nc", []Pos{2, 4, 5}},
		{"a\r\nb", []Pos{3, 4}},
		{"\n\n", []Pos{1, 2, 2}},
		{"no newline", []Pos{10}},
		{"", []Pos{0}},
	} {
		t.Run(tt.src, func(t *testing.T) {
			var l Lexer
			if err := l.Reset("t", strings.NewReader(tt.src), 0, nil); err != nil {
				t.Fatal(err)
			}
			for i, want := range tt.steps {
				if got := l.SkipLine(); got != want {
					t.Fatalf("SkipLine %d = %v, want %v", i, got, want)
				}
			}
		})
	}
}

func TestLexerSkipLineMidLine(t *testing.T) {
	var l Lexer
	if err := l.Reset("t", strings.NewReader("abc\ndef"), 0, nil); err != nil {
		t.Fatal(err)
	}
	l.Next() // consume "abc"
	if got := l.SkipLine(); got != 4 {
		t.Fatalf("SkipLine = %v, want 4", got)
	}
	tok, start, end := l.Next()
	if tok != TokWord || start != 4 || end != 7 {
		t.Fatalf("got %s %v..%v, want TokWord 4..7", tok, start, end)
	}
}

func TestLexerColumns(t *testing.T) {
	// Tabs advance to the next 4-column stop; a multi-byte rune is one column.
	const src = "ab\tc\n€x"
	want := []int{0, 1, 2, 4, 5, 0, 1} // Column of each successive rune.
	var l Lexer
	if err := l.Reset("t", strings.NewReader(src), 0, nil); err != nil {
		t.Fatal(err)
	}
	for i, w := range want {
		if got := l.Col(); got != w {
			t.Errorf("rune %d (%q): Col = %d, want %d", i, l.PeekRune(), got, w)
		}
		l.advance()
	}
}

func TestLexerAtLineStart(t *testing.T) {
	var l Lexer
	if err := l.Reset("t", strings.NewReader("a\nb"), 0, nil); err != nil {
		t.Fatal(err)
	}
	if !l.AtLineStart() {
		t.Error("start of file should be a line start")
	}
	l.Next() // "a"
	if l.AtLineStart() {
		t.Error("after a word should not be a line start")
	}
	l.Next() // newline
	if !l.AtLineStart() {
		t.Error("after a newline should be a line start")
	}
}

func TestLexerSeek(t *testing.T) {
	const src = "hello *world*"
	var l Lexer
	if err := l.Reset("t", strings.NewReader(src), 0, make([]byte, MinWindow)); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		l.Next()
	}
	if err := l.Seek(6); err != nil {
		t.Fatal(err)
	}
	tok, start, end := l.Next()
	if tok != TokStar || start != 6 || end != 7 {
		t.Fatalf("after Seek got %s %v..%v, want TokStar 6..7", tok, start, end)
	}
	// Seeking backwards re-reads the same tokens.
	if err := l.Seek(0); err != nil {
		t.Fatal(err)
	}
	if tok, start, end := l.Next(); tok != TokWord || start != 0 || end != 5 {
		t.Fatalf("after rewind got %s %v..%v, want TokWord 0..5", tok, start, end)
	}
}

func TestLexerCursorState(t *testing.T) {
	var l Lexer
	if err := l.Reset("t", strings.NewReader("ab"), 0, nil); err != nil {
		t.Fatal(err)
	}
	if l.IsDone() || l.Pos() != 0 {
		t.Fatalf("fresh lexer: IsDone=%v Pos=%v, want false 0", l.IsDone(), l.Pos())
	}
	l.Next()
	if !l.IsDone() || l.Pos() != 2 {
		t.Fatalf("drained lexer: IsDone=%v Pos=%v, want true 2", l.IsDone(), l.Pos())
	}
	l.advance() // Advancing an empty cursor must not move it.
	if l.Pos() != 2 {
		t.Fatalf("Pos after advance past end = %v, want 2", l.Pos())
	}
	if err := l.Seek(-1); err != errNegativeOff {
		t.Fatalf("Seek(-1) = %v, want %v", err, errNegativeOff)
	}
}

func TestSpaceRuneClassification(t *testing.T) {
	for _, r := range []rune{' ', '\t', '\n', '\r', '\f', '\v', ' ', ' '} {
		if !isSpaceRune(r) {
			t.Errorf("isSpaceRune(%q) = false, want true", r)
		}
	}
	for _, r := range []rune{'a', '*', '—', 0} {
		if isSpaceRune(r) {
			t.Errorf("isSpaceRune(%q) = true, want false", r)
		}
	}
}

func TestLexerResetOffset(t *testing.T) {
	const src = "hello *world*"
	var l Lexer
	if err := l.Reset("t", strings.NewReader(src), 6, nil); err != nil {
		t.Fatal(err)
	}
	if tok, start, end := l.Next(); tok != TokStar || start != 6 || end != 7 {
		t.Fatalf("got %s %v..%v, want TokStar 6..7", tok, start, end)
	}
}

// errReader fails every read, to prove a read failure surfaces as TokIllegal
// with the error latched rather than as a silent EOF.
type errReader struct{ err error }

func (e errReader) ReadAt([]byte, int64) (int, error) { return 0, e.err }

func TestLexerReadError(t *testing.T) {
	want := errors.New("boom")
	var l Lexer
	err := l.Reset("t", errReader{want}, 0, make([]byte, MinWindow))
	if !errors.Is(err, want) && !errors.Is(l.Err(), want) {
		t.Fatalf("Reset err = %v, Err = %v, want %v", err, l.Err(), want)
	}
	tok, _, _ := l.Next()
	if tok != TokIllegal {
		t.Fatalf("got %s, want TokIllegal", tok)
	}
	if !errors.Is(l.Err(), want) {
		t.Fatalf("Err = %v, want %v", l.Err(), want)
	}
}

func TestLexerResetRejects(t *testing.T) {
	var l Lexer
	if err := l.Reset("t", nil, 0, nil); err != errNilReader {
		t.Errorf("nil reader: %v, want %v", err, errNilReader)
	}
	if err := l.Reset("t", strings.NewReader("x"), -1, nil); err != errNegativeOff {
		t.Errorf("negative off: %v, want %v", err, errNegativeOff)
	}
	if err := l.Reset("t", strings.NewReader("x"), 0, make([]byte, MinWindow-1)); err != errShortWindow {
		t.Errorf("short window: %v, want %v", err, errShortWindow)
	}
}

// TestLexerStraddle walks a multi-byte rune across every offset within the
// window so a sequence split by a refill is decoded whole at least once.
func TestLexerStraddle(t *testing.T) {
	for pad := range MinWindow + 8 {
		src := strings.Repeat("a", pad) + "€" + "b"
		var l Lexer
		if err := l.Reset("t", strings.NewReader(src), 0, make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		tok, start, end := l.Next()
		if tok != TokWord || start != 0 || end != Pos(len(src)) {
			t.Fatalf("pad=%d: got %s %v..%v, want TokWord 0..%d", pad, tok, start, end, len(src))
		}
		if tok, _, _ := l.Next(); tok != TokEOF {
			t.Fatalf("pad=%d: got %s, want TokEOF", pad, tok)
		}
	}
}

// TestLexerEntityStraddle puts an entity across the refill edge: the peek buffer
// cannot hold one, so readEntity must rewind through the window without loss.
func TestLexerEntityStraddle(t *testing.T) {
	for pad := range MinWindow + 8 {
		src := strings.Repeat("a", pad) + "&amp;z"
		var l Lexer
		if err := l.Reset("t", strings.NewReader(src), 0, make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		var got []tokexp
		for {
			tok, start, end := l.Next()
			if tok == TokEOF {
				break
			}
			if tok == TokIllegal {
				t.Fatalf("pad=%d: illegal at %v: %v", pad, start, l.Err())
			}
			got = append(got, tokexp{tok, start, end})
		}
		var want []tokexp
		if pad > 0 {
			want = append(want, tk(TokWord, 0, pad))
		}
		want = append(want, tk(TokEntity, pad, pad+5), tk(TokWord, pad+5, pad+6))
		if len(got) != len(want) {
			t.Fatalf("pad=%d: got %v, want %v", pad, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("pad=%d: token %d = %v, want %v", pad, i, got[i], want[i])
			}
		}
	}
}

// TestLexerFailedEntityStraddle is the same rewind, taken on the failure path
// where the bytes must be re-tokenized as ordinary text.
func TestLexerFailedEntityStraddle(t *testing.T) {
	for pad := range MinWindow + 8 {
		src := strings.Repeat("a", pad) + "&amp z"
		var l Lexer
		if err := l.Reset("t", strings.NewReader(src), 0, make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		var want []tokexp
		if pad > 0 {
			want = append(want, tk(TokWord, 0, pad)) // '&' is special, so it ends the word.
		}
		want = append(want,
			tk(TokWord, pad, pad+4), // "&amp" after the entity attempt rewinds.
			tk(TokSpace, pad+4, pad+5),
			tk(TokWord, pad+5, pad+6),
		)
		for i, w := range want {
			tok, start, end := l.Next()
			if (tokexp{tok, start, end}) != w {
				t.Fatalf("pad=%d: token %d = %v, want %v", pad, i, tokexp{tok, start, end}, w)
			}
		}
	}
}

// TestLexerColAfterEntityRewind pins that a rewind restores the column, which
// Seek alone does not.
func TestLexerColAfterEntityRewind(t *testing.T) {
	var l Lexer
	if err := l.Reset("t", strings.NewReader("ab&amp x"), 0, nil); err != nil {
		t.Fatal(err)
	}
	l.Next() // "ab"
	l.Next() // "&amp" — the entity attempt fails and rewinds.
	if got := l.Col(); got != 6 {
		t.Fatalf("Col = %d, want 6", got)
	}
}

func TestLexerNoAllocs(t *testing.T) {
	src := strings.Repeat("# Head\n\nsome *emph* and `code` and [a](b).\n\n", 64)
	r := strings.NewReader(src)
	buf := make([]byte, 4096)
	var l Lexer
	allocs := testing.AllocsPerRun(10, func() {
		if err := l.Reset("t", r, 0, buf); err != nil {
			t.Fatal(err)
		}
		for {
			tok, _, _ := l.Next()
			if tok == TokEOF || tok == TokIllegal {
				return
			}
		}
	})
	if allocs != 0 {
		t.Errorf("AllocsPerRun = %v, want 0", allocs)
	}
}

func FuzzLexerCoverage(f *testing.F) {
	for _, tt := range lexTests {
		f.Add(tt.src)
	}
	f.Fuzz(func(t *testing.T, src string) {
		var l Lexer
		if err := l.Reset("f", strings.NewReader(src), 0, make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		var at Pos
		for range len(src) + 2 {
			tok, start, end := l.Next()
			if tok == TokEOF {
				if at != Pos(len(src)) || start != at || end != at {
					t.Fatalf("EOF at %v..%v with %v covered, want %d", start, end, at, len(src))
				}
				return
			}
			if tok == TokIllegal {
				t.Fatalf("illegal token over %q: %v", src, l.Err())
			}
			if start != at || end <= start {
				t.Fatalf("token %s spans %v..%v, want start %v and non-empty", tok, start, end, at)
			}
			at = end
		}
		t.Fatalf("Next did not terminate over %q", src)
	})
}
