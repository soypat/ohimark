package ohimark

import (
	"strings"
	"testing"
)

func TestKindPredicates(t *testing.T) {
	for _, tt := range []struct {
		k                                     Kind
		container, block, inline, leaf, named bool
	}{
		{KindUndefined, false, false, false, false, true},
		{KindDocument, false, false, false, false, true},
		{KindBlockQuote, true, true, false, false, true},
		{KindList, true, true, false, false, true},
		{KindItem, true, true, false, false, true},
		{KindParagraph, false, true, false, false, true},
		{KindHeading, false, true, false, false, true},
		{KindCodeBlock, false, true, false, false, true},
		{KindThematicBreak, false, true, false, true, true},
		{KindEmph, false, false, true, false, true},
		{KindStrong, false, false, true, false, true},
		{KindLink, false, false, true, false, true},
		{KindImage, false, false, true, false, true},
		{KindCodeSpan, false, false, true, false, true},
		{KindText, false, false, true, true, true},
		{KindEscape, false, false, true, true, true},
		{KindEntity, false, false, true, true, true},
		{KindAutolink, false, false, true, true, true},
		{KindSoftBreak, false, false, true, true, true},
		{KindHardBreak, false, false, true, true, true},
		{KindInfo, false, false, false, true, true},
		{KindDest, false, false, false, true, true},
		{KindTitle, false, false, false, true, true},
		{KindRaw, false, false, false, true, true},
	} {
		if got := tt.k.IsContainer(); got != tt.container {
			t.Errorf("%s.IsContainer() = %v, want %v", tt.k, got, tt.container)
		}
		if got := tt.k.IsBlock(); got != tt.block {
			t.Errorf("%s.IsBlock() = %v, want %v", tt.k, got, tt.block)
		}
		if got := tt.k.IsInline(); got != tt.inline {
			t.Errorf("%s.IsInline() = %v, want %v", tt.k, got, tt.inline)
		}
		if got := tt.k.IsLeaf(); got != tt.leaf {
			t.Errorf("%s.IsLeaf() = %v, want %v", tt.k, got, tt.leaf)
		}
		// A container is a block, and nothing is both a block and an inline.
		if tt.k.IsContainer() && !tt.k.IsBlock() {
			t.Errorf("%s is a container but not a block", tt.k)
		}
		if tt.k.IsBlock() && tt.k.IsInline() {
			t.Errorf("%s is both a block and an inline", tt.k)
		}
		// The sentinels bounding the ranges must not leak into String.
		if name := tt.k.String(); (name != "") != tt.named || strings.Contains(name, "(") {
			t.Errorf("%s.String() = %q, want a name: %v", tt.k, name, tt.named)
		}
	}
}

func TestTokenPredicates(t *testing.T) {
	runs := []Token{TokHash, TokStar, TokUnderscore, TokTick, TokTilde, TokDash, TokGT}
	for _, tok := range runs {
		if !tok.IsRun() {
			t.Errorf("%s.IsRun() = false, want true", tok)
		}
	}
	for _, tok := range []Token{
		TokUndefined, TokIllegal, TokEOF, TokWord, TokDigits, TokSpace, TokNewline,
		TokPlus, TokBang, TokLT, TokBracketL, TokBracketR, TokParenL, TokParenR,
		TokDot, TokEscape, TokEntity,
	} {
		if tok.IsRun() {
			t.Errorf("%s.IsRun() = true, want false", tok)
		}
	}
	for _, tok := range []Token{TokSpace, TokNewline, TokEOF} {
		if !tok.IsSpace() {
			t.Errorf("%s.IsSpace() = false, want true", tok)
		}
	}
	for _, tok := range append(runs, TokWord, TokDigits, TokEscape, TokEntity) {
		if tok.IsSpace() {
			t.Errorf("%s.IsSpace() = true, want false", tok)
		}
	}
}

func TestNodeSpan(t *testing.T) {
	n := Node{Kind: KindText, Start: 4, End: 11}
	if n.Len() != 7 {
		t.Errorf("Len = %d, want 7", n.Len())
	}
	if n.IsOpen() || n.IsClosed() {
		t.Error("a bare node should be neither open nor closed")
	}
	n.Flags = FlagOpen
	if !n.IsOpen() || n.IsClosed() {
		t.Error("FlagOpen should read as open only")
	}
	n.Flags = FlagClose
	if n.IsOpen() || !n.IsClosed() {
		t.Error("FlagClose should read as closed only")
	}
}

func TestPosString(t *testing.T) {
	for _, tt := range []struct {
		pos  Pos
		want string
	}{{0, "@0x0"}, {10, "@0xa"}, {255, "@0xff"}, {1 << 20, "@0x100000"}} {
		if got := tt.pos.String(); got != tt.want {
			t.Errorf("Pos(%d).String() = %q, want %q", tt.pos, got, tt.want)
		}
		if got := string(tt.pos.AppendString([]byte("x:"))); got != "x:"+tt.want {
			t.Errorf("AppendString = %q, want %q", got, "x:"+tt.want)
		}
	}
}

func TestPosToLineCol(t *testing.T) {
	const src = "one\ntwo\nthree\n"
	aux := make([]byte, 64)
	for _, tt := range []struct {
		pos              Pos
		line, col, lenth int
	}{
		{0, 1, 1, 3},  // 'o' of "one"
		{2, 1, 3, 3},  // 'e' of "one"
		{4, 2, 1, 3},  // 't' of "two"
		{8, 3, 1, 5},  // 't' of "three"
		{12, 3, 5, 5}, // 'e' of "three"
	} {
		line, col, length, err := tt.pos.ToLineCol(strings.NewReader(src), aux)
		if err != nil {
			t.Fatalf("Pos(%d): %v", tt.pos, err)
		}
		if line != tt.line || col != tt.col || length != tt.lenth {
			t.Errorf("Pos(%d).ToLineCol = %d:%d len %d, want %d:%d len %d",
				tt.pos, line, col, length, tt.line, tt.col, tt.lenth)
		}
	}
}
