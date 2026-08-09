package ohimark

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// Node streams are written out one node per line as
//
//	kind [open|close] start..end "span" [attr=N] [ordered] [loose] [fenced]
//
// The quoted span is derived from the offsets, so a line that reads correctly
// also proves the offsets are right.
func writeNode(sb *strings.Builder, src string, n Node) {
	sb.WriteString(n.Kind.String())
	switch {
	case n.IsOpen():
		sb.WriteString(" open")
	case n.IsClose():
		sb.WriteString(" close")
	}
	fmt.Fprintf(sb, " %d..%d %q", n.Start, n.End, src[n.Start:n.End])
	if n.Attr != 0 {
		fmt.Fprintf(sb, " attr=%d", n.Attr)
	}
	for _, f := range []struct {
		bit  Flags
		name string
	}{{FlagOrdered, "ordered"}, {FlagLoose, "loose"}, {FlagFenced, "fenced"}} {
		if n.Flags&f.bit != 0 {
			sb.WriteString(" " + f.name)
		}
	}
	sb.WriteByte('\n')
}

// dumpNodes drains a parser over src, checking the structural invariants as it
// goes, and returns the rendered stream.
func dumpNodes(t *testing.T, src string, bufSize int) string {
	t.Helper()
	var p Parser
	if err := p.Reset(t.Name(), strings.NewReader(src), make([]byte, bufSize)); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	var sb strings.Builder
	var open []Node // Balance check: every close must pair with the last open.
	limit := 8*len(src) + 16
	for i := 0; ; i++ {
		n, err := p.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("Next: %v\nso far:\n%s", err, sb.String())
		}
		if i > limit {
			t.Fatalf("Next did not terminate over %q\nso far:\n%s", src, sb.String())
		}
		if n.Start < 0 || n.End > Pos(len(src)) || n.Start > n.End {
			t.Fatalf("node %d (%s) has span %v..%v outside 0..%d", i, n.Kind, n.Start, n.End, len(src))
		}
		if n.IsOpen() && n.IsClose() {
			t.Fatalf("node %d (%s) is both open and close", i, n.Kind)
		}
		if n.Kind.IsLeaf() && (n.IsOpen() || n.IsClose()) {
			t.Fatalf("leaf node %d (%s) carries an open/close flag", i, n.Kind)
		}
		// A list holds items and nothing else; everything else holds no items.
		if len(open) > 0 && (n.IsOpen() || n.Kind.IsLeaf()) {
			if parent := open[len(open)-1].Kind; (parent == KindList) != (n.Kind == KindItem) {
				t.Fatalf("node %d: %s directly inside %s", i, n.Kind, parent)
			}
		}
		switch {
		case n.IsOpen():
			open = append(open, n)
		case n.IsClose():
			if len(open) == 0 {
				t.Fatalf("node %d: %s close with nothing open", i, n.Kind)
			}
			last := open[len(open)-1]
			open = open[:len(open)-1]
			if last.Kind != n.Kind {
				t.Fatalf("node %d: %s close pairs with %s open", i, n.Kind, last.Kind)
			}
			if n.Start != last.Start || n.End < last.End {
				t.Fatalf("node %d: %s close %v..%v does not contain its open %v..%v",
					i, n.Kind, n.Start, n.End, last.Start, last.End)
			}
		}
		writeNode(&sb, src, n)
	}
	if len(open) != 0 {
		t.Fatalf("%d containers left open over %q:\n%s", len(open), src, sb.String())
	}
	if d := p.Depth(); d != 0 {
		t.Errorf("Depth after drain = %d, want 0", d)
	}
	if err := p.Err(); err != nil {
		t.Errorf("Err after clean drain = %v, want nil", err)
	}
	return sb.String()
}

const bt = "`" // Fenced-code cases need backticks, which raw literals cannot hold.

var parseTests = []struct {
	name string
	src  string
	want []string
}{
	{"empty", "", []string{
		`document open 0..0 ""`,
		`document close 0..0 ""`,
	}},
	{"blank lines only", "\n\n\n", []string{
		`document open 0..0 ""`,
		`document close 0..3 "\n\n\n"`,
	}},

	// --- paragraphs ---
	{"paragraph", "hello\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..5 "hello"`,
		`paragraph close 0..5 "hello"`,
		`document close 0..6 "hello\n"`,
	}},
	{"paragraph unterminated", "hello", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..5 "hello"`,
		`paragraph close 0..5 "hello"`,
		`document close 0..5 "hello"`,
	}},
	{"paragraph multiline", "a\nb\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`softbreak 1..2 "\n"`,
		`text 2..3 "b"`,
		`paragraph close 0..3 "a\nb"`,
		`document close 0..4 "a\nb\n"`,
	}},
	{"two paragraphs", "a\n\nb\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`paragraph close 0..1 "a"`,
		`paragraph open 3..3 ""`,
		`text 3..4 "b"`,
		`paragraph close 3..4 "b"`,
		`document close 0..5 "a\n\nb\n"`,
	}},
	{"leading blanks", "\n\nx\n", []string{
		`document open 0..0 ""`,
		`paragraph open 2..2 ""`,
		`text 2..3 "x"`,
		`paragraph close 2..3 "x"`,
		`document close 0..4 "\n\nx\n"`,
	}},
	{"trailing spaces stripped", "a  \n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`paragraph close 0..1 "a"`,
		`document close 0..4 "a  \n"`,
	}},
	{"leading indent under four", "  a\n", []string{
		`document open 0..0 ""`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`document close 0..4 "  a\n"`,
	}},
	{"crlf", "a\r\nb\r\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`softbreak 1..3 "\r\n"`,
		`text 3..4 "b"`,
		`paragraph close 0..4 "a\r\nb"`,
		`document close 0..6 "a\r\nb\r\n"`,
	}},
	{"inline markers resolve", "a *b* c\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..2 "a "`,
		`emph open 2..3 "*" attr=42`,
		`text 3..4 "b"`,
		`emph close 2..5 "*b*" attr=42`,
		`text 5..7 " c"`,
		`paragraph close 0..7 "a *b* c"`,
		`document close 0..8 "a *b* c\n"`,
	}},

	// --- ATX headings ---
	{"heading", "# Title\n", []string{
		`document open 0..0 ""`,
		`heading open 0..2 "# " attr=1`,
		`text 2..7 "Title"`,
		`heading close 0..7 "# Title" attr=1`,
		`document close 0..8 "# Title\n"`,
	}},
	{"heading level six", "###### x", []string{
		`document open 0..0 ""`,
		`heading open 0..7 "###### " attr=6`,
		`text 7..8 "x"`,
		`heading close 0..8 "###### x" attr=6`,
		`document close 0..8 "###### x"`,
	}},
	{"seven hashes is a paragraph", "####### x\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..9 "####### x"`,
		`paragraph close 0..9 "####### x"`,
		`document close 0..10 "####### x\n"`,
	}},
	{"heading needs a space", "#hashtag\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..8 "#hashtag"`,
		`paragraph close 0..8 "#hashtag"`,
		`document close 0..9 "#hashtag\n"`,
	}},
	{"heading closing sequence", "## T ##\n", []string{
		`document open 0..0 ""`,
		`heading open 0..3 "## " attr=2`,
		`text 3..4 "T"`,
		`heading close 0..7 "## T ##" attr=2`,
		`document close 0..8 "## T ##\n"`,
	}},
	{"heading empty", "#\n", []string{
		`document open 0..0 ""`,
		`heading open 0..1 "#" attr=1`,
		`heading close 0..1 "#" attr=1`,
		`document close 0..2 "#\n"`,
	}},
	{"heading indented", "   # a\n", []string{
		`document open 0..0 ""`,
		`heading open 3..5 "# " attr=1`,
		`text 5..6 "a"`,
		`heading close 3..6 "# a" attr=1`,
		`document close 0..7 "   # a\n"`,
	}},

	// --- thematic breaks ---
	{"thematic break", "---\n", []string{
		`document open 0..0 ""`,
		`thematicbreak 0..3 "---"`,
		`document close 0..4 "---\n"`,
	}},
	{"thematic break stars", "***\n", []string{
		`document open 0..0 ""`,
		`thematicbreak 0..3 "***"`,
		`document close 0..4 "***\n"`,
	}},
	{"thematic break spaced beats bullet", "- - -\n", []string{
		`document open 0..0 ""`,
		`thematicbreak 0..5 "- - -"`,
		`document close 0..6 "- - -\n"`,
	}},
	{"setext is not supported", "a\n---\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`paragraph close 0..1 "a"`,
		`thematicbreak 2..5 "---"`,
		`document close 0..6 "a\n---\n"`,
	}},

	// --- blockquotes ---
	{"blockquote", "> a\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`blockquote close 0..3 "> a"`,
		`document close 0..4 "> a\n"`,
	}},
	{"blockquote two lines", "> a\n> b\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`softbreak 3..6 "\n> "`,
		`text 6..7 "b"`,
		`paragraph close 2..7 "a\n> b"`,
		`blockquote close 0..7 "> a\n> b"`,
		`document close 0..8 "> a\n> b\n"`,
	}},
	{"blockquote without space", ">a\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..1 ">"`,
		`paragraph open 1..1 ""`,
		`text 1..2 "a"`,
		`paragraph close 1..2 "a"`,
		`blockquote close 0..2 ">a"`,
		`document close 0..3 ">a\n"`,
	}},
	{"nested blockquote", "> > a\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`blockquote open 2..4 "> "`,
		`paragraph open 4..4 ""`,
		`text 4..5 "a"`,
		`paragraph close 4..5 "a"`,
		`blockquote close 2..5 "> a"`,
		`blockquote close 0..5 "> > a"`,
		`document close 0..6 "> > a\n"`,
	}},
	{"no lazy continuation", "> a\nb\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`blockquote close 0..3 "> a"`,
		`paragraph open 4..4 ""`,
		`text 4..5 "b"`,
		`paragraph close 4..5 "b"`,
		`document close 0..6 "> a\nb\n"`,
	}},
	{"heading in blockquote", "> # T\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`heading open 2..4 "# " attr=1`,
		`text 4..5 "T"`,
		`heading close 2..5 "# T" attr=1`,
		`blockquote close 0..5 "> # T"`,
		`document close 0..6 "> # T\n"`,
	}},

	// --- lists ---
	{"bullet list", "- a\n- b\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`item close 0..3 "- a"`,
		`item open 4..6 "- " attr=1`,
		`paragraph open 6..6 ""`,
		`text 6..7 "b"`,
		`paragraph close 6..7 "b"`,
		`item close 4..7 "- b" attr=1`,
		`list close 0..7 "- a\n- b" attr=45`,
		`document close 0..8 "- a\n- b\n"`,
	}},
	{"bullet star", "* a\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "* " attr=42`,
		`item open 0..2 "* "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`item close 0..3 "* a"`,
		`list close 0..3 "* a" attr=42`,
		`document close 0..4 "* a\n"`,
	}},
	{"bullet plus", "+ a\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "+ " attr=43`,
		`item open 0..2 "+ "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`item close 0..3 "+ a"`,
		`list close 0..3 "+ a" attr=43`,
		`document close 0..4 "+ a\n"`,
	}},
	{"bullet change starts a new list", "- a\n* b\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`item close 0..3 "- a"`,
		`list close 0..3 "- a" attr=45`,
		`list open 4..6 "* " attr=42`,
		`item open 4..6 "* "`,
		`paragraph open 6..6 ""`,
		`text 6..7 "b"`,
		`paragraph close 6..7 "b"`,
		`item close 4..7 "* b"`,
		`list close 4..7 "* b" attr=42`,
		`document close 0..8 "- a\n* b\n"`,
	}},
	{"ordered list", "1. a\n2. b\n", []string{
		`document open 0..0 ""`,
		`list open 0..3 "1. " attr=1 ordered`,
		`item open 0..3 "1. "`,
		`paragraph open 3..3 ""`,
		`text 3..4 "a"`,
		`paragraph close 3..4 "a"`,
		`item close 0..4 "1. a"`,
		`item open 5..8 "2. " attr=1`,
		`paragraph open 8..8 ""`,
		`text 8..9 "b"`,
		`paragraph close 8..9 "b"`,
		`item close 5..9 "2. b" attr=1`,
		`list close 0..9 "1. a\n2. b" attr=1 ordered`,
		`document close 0..10 "1. a\n2. b\n"`,
	}},
	{"ordered list start five paren", "5) x\n", []string{
		`document open 0..0 ""`,
		`list open 0..3 "5) " attr=5 ordered`,
		`item open 0..3 "5) "`,
		`paragraph open 3..3 ""`,
		`text 3..4 "x"`,
		`paragraph close 3..4 "x"`,
		`item close 0..4 "5) x"`,
		`list close 0..4 "5) x" attr=5 ordered`,
		`document close 0..5 "5) x\n"`,
	}},
	{"loose list", "- a\n\n- b\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`item close 0..3 "- a"`,
		`item open 5..7 "- " attr=1`,
		`paragraph open 7..7 ""`,
		`text 7..8 "b"`,
		`paragraph close 7..8 "b"`,
		`item close 5..8 "- b" attr=1`,
		`list close 0..8 "- a\n\n- b" attr=45 loose`,
		`document close 0..9 "- a\n\n- b\n"`,
	}},
	{"nested list", "- a\n  - b\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`list open 6..8 "- " attr=45`,
		`item open 6..8 "- "`,
		`paragraph open 8..8 ""`,
		`text 8..9 "b"`,
		`paragraph close 8..9 "b"`,
		`item close 6..9 "- b"`,
		`list close 6..9 "- b" attr=45`,
		`item close 0..9 "- a\n  - b"`,
		`list close 0..9 "- a\n  - b" attr=45`,
		`document close 0..10 "- a\n  - b\n"`,
	}},
	{"item continuation line", "- a\n  b\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`softbreak 3..6 "\n  "`,
		`text 6..7 "b"`,
		`paragraph close 2..7 "a\n  b"`,
		`item close 0..7 "- a\n  b"`,
		`list close 0..7 "- a\n  b" attr=45`,
		`document close 0..8 "- a\n  b\n"`,
	}},
	{"list in blockquote", "> - a\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`list open 2..4 "- " attr=45`,
		`item open 2..4 "- "`,
		`paragraph open 4..4 ""`,
		`text 4..5 "a"`,
		`paragraph close 4..5 "a"`,
		`item close 2..5 "- a"`,
		`list close 2..5 "- a" attr=45`,
		`blockquote close 0..5 "> - a"`,
		`document close 0..6 "> - a\n"`,
	}},

	// --- code blocks ---
	{"fenced code", bt + bt + bt + "go\ncode\n" + bt + bt + bt + "\n", []string{
		`document open 0..0 ""`,
		"codeblock open 0..6 \"```go\\n\" attr=3 fenced",
		`info 3..5 "go"`,
		`raw 6..11 "code\n"`,
		"codeblock close 0..14 \"```go\\ncode\\n```\" attr=3 fenced",
		"document close 0..15 \"```go\\ncode\\n```\\n\"",
	}},
	{"fenced code no info", bt + bt + bt + "\nx\n" + bt + bt + bt + "\n", []string{
		`document open 0..0 ""`,
		"codeblock open 0..4 \"```\\n\" attr=3 fenced",
		`raw 4..6 "x\n"`,
		"codeblock close 0..9 \"```\\nx\\n```\" attr=3 fenced",
		"document close 0..10 \"```\\nx\\n```\\n\"",
	}},
	{"fenced code unterminated", bt + bt + bt + "\nx\n", []string{
		`document open 0..0 ""`,
		"codeblock open 0..4 \"```\\n\" attr=3 fenced",
		`raw 4..6 "x\n"`,
		"codeblock close 0..5 \"```\\nx\" attr=3 fenced",
		"document close 0..6 \"```\\nx\\n\"",
	}},
	{"fenced code holds markdown verbatim", bt + bt + bt + "\n# not a heading\n" + bt + bt + bt + "\n", []string{
		`document open 0..0 ""`,
		"codeblock open 0..4 \"```\\n\" attr=3 fenced",
		`raw 4..20 "# not a heading\n"`,
		"codeblock close 0..23 \"```\\n# not a heading\\n```\" attr=3 fenced",
		"document close 0..24 \"```\\n# not a heading\\n```\\n\"",
	}},
	{"tilde fence", "~~~\nx\n~~~\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "~~~\n" attr=3 fenced`,
		`raw 4..6 "x\n"`,
		`codeblock close 0..9 "~~~\nx\n~~~" attr=3 fenced`,
		`document close 0..10 "~~~\nx\n~~~\n"`,
	}},
	{"indented code", "    code\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..9 "code\n"`,
		`codeblock close 0..8 "    code"`,
		`document close 0..9 "    code\n"`,
	}},
	{"indented code two lines", "    a\n    b\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..6 "a\n"`,
		`raw 10..12 "b\n"`,
		`codeblock close 0..11 "    a\n    b"`,
		`document close 0..12 "    a\n    b\n"`,
	}},
	{"fenced code in blockquote", "> ~~~\n> x\n> ~~~\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`codeblock open 2..6 "~~~\n" attr=3 fenced`,
		`raw 8..10 "x\n"`,
		`codeblock close 2..15 "~~~\n> x\n> ~~~" attr=3 fenced`,
		`blockquote close 0..15 "> ~~~\n> x\n> ~~~"`,
		`document close 0..16 "> ~~~\n> x\n> ~~~\n"`,
	}},
}

// parseEdgeTests covers the branches the main table leaves untouched, plus the
// places this dialect knowingly parts ways with CommonMark.
var parseEdgeTests = []struct {
	name string
	src  string
	want []string
}{
	{"blockquote indented", "  > a\n", []string{
		`document open 0..0 ""`,
		`blockquote open 2..4 "> "`,
		`paragraph open 4..4 ""`,
		`text 4..5 "a"`,
		`paragraph close 4..5 "a"`,
		`blockquote close 2..5 "> a"`,
		`document close 0..6 "  > a\n"`,
	}},
	{"blank line inside blockquote", "> a\n> \n> b\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`paragraph open 9..9 ""`,
		`text 9..10 "b"`,
		`paragraph close 9..10 "b"`,
		`blockquote close 0..10 "> a\n> \n> b"`,
		`document close 0..11 "> a\n> \n> b\n"`,
	}},
	// A blank line between indented lines is interior content, so it belongs to
	// one code block rather than splitting it into two.
	{"blank line inside indented code", "    a\n\n    b\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..6 "a\n"`,
		`raw 6..7 "\n"`,
		`raw 11..13 "b\n"`,
		`codeblock close 0..12 "    a\n\n    b"`,
		`document close 0..13 "    a\n\n    b\n"`,
	}},
	// An interior blank line still has its indent stripped, so it contributes
	// only its line ending.
	{"blank line with spaces inside indented code", "    a\n  \n    b\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..6 "a\n"`,
		`raw 8..9 "\n"`,
		`raw 13..15 "b\n"`,
		`codeblock close 0..14 "    a\n  \n    b"`,
		`document close 0..15 "    a\n  \n    b\n"`,
	}},
	// Trailing blank lines are not content, so they are dropped and the block
	// closes at its last real byte.
	{"trailing blank after indented code", "    a\n\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..6 "a\n"`,
		`codeblock close 0..5 "    a"`,
		`document close 0..7 "    a\n\n"`,
	}},
	{"indented code ended by a paragraph", "    a\n\nb\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..6 "a\n"`,
		`codeblock close 0..5 "    a"`,
		`paragraph open 7..7 ""`,
		`text 7..8 "b"`,
		`paragraph close 7..8 "b"`,
		`document close 0..9 "    a\n\nb\n"`,
	}},
	// An interior run and a trailing run in the same block: the first is content,
	// the second is dropped.
	{"interior then trailing blanks", "    a\n\n    b\n\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..6 "a\n"`,
		`raw 6..7 "\n"`,
		`raw 11..13 "b\n"`,
		`codeblock close 0..12 "    a\n\n    b"`,
		`document close 0..14 "    a\n\n    b\n\n"`,
	}},
	{"many blank lines inside indented code", "    a\n\n\n\n    b\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..6 "a\n"`,
		`raw 6..7 "\n"`,
		`raw 7..8 "\n"`,
		`raw 8..9 "\n"`,
		`raw 13..15 "b\n"`,
		`codeblock close 0..14 "    a\n\n\n\n    b"`,
		`document close 0..15 "    a\n\n\n\n    b\n"`,
	}},
	{"tab indents code", "\tcode\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..1 "\t"`,
		`raw 1..6 "code\n"`,
		`codeblock close 0..5 "\tcode"`,
		`document close 0..6 "\tcode\n"`,
	}},
	{"backtick in fence info is a paragraph", bt + bt + bt + "a" + bt + "b\nx\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		"text 0..6 \"```a`b\"",
		`softbreak 6..7 "\n"`,
		`text 7..8 "x"`,
		"paragraph close 0..8 \"```a`b\\nx\"",
		"document close 0..9 \"```a`b\\nx\\n\"",
	}},
	{"short closing fence does not close", bt + bt + bt + bt + "\nx\n" + bt + bt + bt + "\n", []string{
		`document open 0..0 ""`,
		"codeblock open 0..5 \"````\\n\" attr=4 fenced",
		`raw 5..7 "x\n"`,
		"raw 7..11 \"```\\n\"",
		"codeblock close 0..10 \"````\\nx\\n```\" attr=4 fenced",
		"document close 0..11 \"````\\nx\\n```\\n\"",
	}},
	{"longer closing fence closes", bt + bt + bt + "\nx\n" + strings.Repeat(bt, 5) + "\n", []string{
		`document open 0..0 ""`,
		"codeblock open 0..4 \"```\\n\" attr=3 fenced",
		`raw 4..6 "x\n"`,
		"codeblock close 0..11 \"```\\nx\\n`````\" attr=3 fenced",
		"document close 0..12 \"```\\nx\\n`````\\n\"",
	}},
	{"overlong ordered number is a paragraph", "1234567890. x\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..13 "1234567890. x"`,
		`paragraph close 0..13 "1234567890. x"`,
		`document close 0..14 "1234567890. x\n"`,
	}},
	{"underscore thematic break", "_ _ _ _\n", []string{
		`document open 0..0 ""`,
		`thematicbreak 0..7 "_ _ _ _"`,
		`document close 0..8 "_ _ _ _\n"`,
	}},
	// A marker that runs to the line's end opens an empty item, not an empty
	// paragraph inside one.
	{"empty list item", "-\n- a\n", []string{
		`document open 0..0 ""`,
		`list open 0..1 "-" attr=45`,
		`item open 0..1 "-"`,
		`item close 0..1 "-"`,
		`item open 2..4 "- " attr=1`,
		`paragraph open 4..4 ""`,
		`text 4..5 "a"`,
		`paragraph close 4..5 "a"`,
		`item close 2..5 "- a" attr=1`,
		`list close 0..5 "-\n- a" attr=45`,
		`document close 0..6 "-\n- a\n"`,
	}},
	// A quote not indented to the item's content column is the list's sibling,
	// so it ends the list rather than nesting inside it.
	{"blockquote ends a list", "- a\n\n> q\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`item close 0..3 "- a"`,
		`list close 0..3 "- a" attr=45`,
		`blockquote open 5..7 "> "`,
		`paragraph open 7..7 ""`,
		`text 7..8 "q"`,
		`paragraph close 7..8 "q"`,
		`blockquote close 5..8 "> q"`,
		`document close 0..9 "- a\n\n> q\n"`,
	}},
	{"blockquote inside an item", "- > q\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`blockquote open 2..4 "> "`,
		`paragraph open 4..4 ""`,
		`text 4..5 "q"`,
		`paragraph close 4..5 "q"`,
		`blockquote close 2..5 "> q"`,
		`item close 0..5 "- > q"`,
		`list close 0..5 "- > q" attr=45`,
		`document close 0..6 "- > q\n"`,
	}},
	{"two dashes are a paragraph", "--\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..2 "--"`,
		`paragraph close 0..2 "--"`,
		`document close 0..3 "--\n"`,
	}},
	// A leaf cannot sit inside a list, so starting one ends the list.
	{"thematic break ends a list", "- a\n- - -\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`item close 0..3 "- a"`,
		`list close 0..3 "- a" attr=45`,
		`thematicbreak 4..9 "- - -"`,
		`document close 0..10 "- a\n- - -\n"`,
	}},
	{"heading ends a list", "- a\n# h\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`item close 0..3 "- a"`,
		`list close 0..3 "- a" attr=45`,
		`heading open 4..6 "# " attr=1`,
		`text 6..7 "h"`,
		`heading close 4..7 "# h" attr=1`,
		`document close 0..8 "- a\n# h\n"`,
	}},
	{"heading in list item", "- # h\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`heading open 2..4 "# " attr=1`,
		`text 4..5 "h"`,
		`heading close 2..5 "# h" attr=1`,
		`item close 0..5 "- # h"`,
		`list close 0..5 "- # h" attr=45`,
		`document close 0..6 "- # h\n"`,
	}},
	// Indented code may not interrupt a paragraph, so this indent continues one.
	{"indent does not interrupt a paragraph", "- a\n      c\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`softbreak 3..10 "\n      "`,
		`text 10..11 "c"`,
		`paragraph close 2..11 "a\n      c"`,
		`item close 0..11 "- a\n      c"`,
		`list close 0..11 "- a\n      c" attr=45`,
		`document close 0..12 "- a\n      c\n"`,
	}},
	// With the paragraph closed by a blank line, the same indent is code. Its
	// four columns are counted from the item's content column, not the line.
	{"indented code in list item", "- a\n\n      c\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`paragraph open 2..2 ""`,
		`text 2..3 "a"`,
		`paragraph close 2..3 "a"`,
		`codeblock open 7..11 "    "`,
		`raw 11..13 "c\n"`,
		`codeblock close 7..12 "    c"`,
		`item close 0..12 "- a\n\n      c"`,
		`list close 0..12 "- a\n\n      c" attr=45`,
		`document close 0..13 "- a\n\n      c\n"`,
	}},
}

// The edge cases run through every check the main table does.
func init() { parseTests = append(parseTests, parseEdgeTests...) }

// bufSizes exercises every case against a buffer that forces many refills and
// one that holds everything. Both streams must be identical.
var bufSizes = []int{MinWindow, 64 << 10}

func TestParserTable(t *testing.T) {
	for _, tt := range parseTests {
		t.Run(tt.name, func(t *testing.T) {
			want := strings.Join(tt.want, "\n") + "\n"
			for _, bufSize := range bufSizes {
				got := dumpNodes(t, tt.src, bufSize)
				if got != want {
					t.Errorf("buf=%d over %q:\n%s", bufSize, tt.src, diff(got, want))
				}
			}
		})
	}
}

func diff(got, want string) string {
	g, w := strings.Split(strings.TrimRight(got, "\n"), "\n"), strings.Split(strings.TrimRight(want, "\n"), "\n")
	var sb strings.Builder
	for i := 0; i < len(g) || i < len(w); i++ {
		gl, wl := "", ""
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		mark := "  "
		if gl != wl {
			mark = "! "
		}
		fmt.Fprintf(&sb, "%sgot  %s\n%swant %s\n", mark, gl, mark, wl)
	}
	return sb.String()
}

// TestParserSpanCoverage is the parser's counterpart to the lexer's token
// coverage: leaf spans never overlap and never run backwards, so the content
// nodes tile the source in emission order.
func TestParserSpanCoverage(t *testing.T) {
	for _, tt := range parseTests {
		t.Run(tt.name, func(t *testing.T) {
			var p Parser
			if err := p.Reset("t", strings.NewReader(tt.src), make([]byte, MinWindow)); err != nil {
				t.Fatal(err)
			}
			var at Pos
			for {
				n, err := p.Next()
				if err == io.EOF {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				// Attribute leaves are emitted ahead of their parent's content,
				// so only true content leaves advance the cursor.
				switch n.Kind {
				case KindText, KindRaw, KindThematicBreak:
					if n.Start < at {
						t.Fatalf("%s at %v..%v overlaps content already emitted through %v",
							n.Kind, n.Start, n.End, at)
					}
					at = n.End
				}
			}
		})
	}
}

func TestParserMaxDepth(t *testing.T) {
	src := strings.Repeat("> ", 32) + "x\n"
	var p Parser
	p.MaxDepth = 4
	if err := p.Reset("t", strings.NewReader(src), make([]byte, MinWindow)); err != nil {
		t.Fatal(err)
	}
	for {
		_, err := p.Next()
		if err == io.EOF {
			t.Fatal("drained without hitting MaxDepth")
		} else if err != nil {
			if !errors.Is(err, errDepthExceeded) {
				t.Fatalf("Next err = %v, want %v", err, errDepthExceeded)
			}
			break
		}
		if p.Depth() > p.MaxDepth {
			t.Fatalf("Depth = %d, over MaxDepth %d", p.Depth(), p.MaxDepth)
		}
	}
	if !errors.Is(p.Err(), errDepthExceeded) {
		t.Fatalf("Err = %v, want %v", p.Err(), errDepthExceeded)
	}
	// The error is sticky.
	if _, err := p.Next(); !errors.Is(err, errDepthExceeded) {
		t.Fatalf("Next after failure = %v, want %v", err, errDepthExceeded)
	}
}

// TestParserDepthPressure runs every case against caps too small for it. Each
// must either drain cleanly or fail with errDepthExceeded — never panic, never
// spin, and never open a block past the cap.
func TestParserDepthPressure(t *testing.T) {
	for depth := 1; depth <= 4; depth++ {
		for _, tt := range parseTests {
			var p Parser
			p.MaxDepth = depth
			if err := p.Reset("t", strings.NewReader(tt.src), make([]byte, MinWindow)); err != nil {
				t.Fatal(err)
			}
			limit := 8*len(tt.src) + 16
			for i := 0; ; i++ {
				_, err := p.Next()
				if err == io.EOF {
					break
				} else if err != nil {
					if !errors.Is(err, errDepthExceeded) {
						t.Fatalf("depth=%d %s: %v", depth, tt.name, err)
					}
					break
				}
				if i > limit {
					t.Fatalf("depth=%d %s: Next did not terminate", depth, tt.name)
				}
				if p.Depth() > depth {
					t.Fatalf("depth=%d %s: Depth = %d", depth, tt.name, p.Depth())
				}
			}
		}
	}
}

func TestParserEOFIsSticky(t *testing.T) {
	var p Parser
	if err := p.Reset("t", strings.NewReader("x\n"), nil); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := p.Next(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		if _, err := p.Next(); err != io.EOF {
			t.Fatalf("call %d after EOF = %v, want io.EOF", i, err)
		}
	}
	if err := p.Err(); err != nil {
		t.Fatalf("Err after clean drain = %v, want nil", err)
	}
}

func TestParserReuse(t *testing.T) {
	// A parser reset onto new input must not carry the previous document's
	// container spine or block state.
	var p Parser
	for _, tt := range parseTests {
		want := strings.Join(tt.want, "\n") + "\n"
		if err := p.Reset("t", strings.NewReader(tt.src), make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		for {
			n, err := p.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			writeNode(&sb, tt.src, n)
		}
		if sb.String() != want {
			t.Fatalf("%s after reuse:\n%s", tt.name, diff(sb.String(), want))
		}
	}
}

func TestParserLexer(t *testing.T) {
	// The embedded lexer is the one the parser drives, so it tracks the parse.
	var p Parser
	if err := p.Reset("t", strings.NewReader("# a\n"), nil); err != nil {
		t.Fatal(err)
	}
	l := p.Lexer()
	if l != &p.l {
		t.Fatal("Lexer did not return the embedded lexer")
	}
	if l.Pos() != 0 {
		t.Fatalf("Pos before parsing = %v, want 0", l.Pos())
	}
	for {
		if _, err := p.Next(); err != nil {
			break
		}
	}
	if l.Pos() != 4 || !l.IsDone() {
		t.Fatalf("after drain Pos = %v IsDone = %v, want 4 true", l.Pos(), l.IsDone())
	}
}

func TestParserReadError(t *testing.T) {
	want := errors.New("boom")
	var p Parser
	p.Reset("t", errReader{want}, make([]byte, MinWindow))
	for range 8 {
		_, err := p.Next()
		if err == nil {
			continue
		}
		if !errors.Is(err, want) {
			t.Fatalf("Next err = %v, want %v", err, want)
		}
		return
	}
	t.Fatalf("read error never surfaced: %v", p.Err())
}

func TestParserNoAllocs(t *testing.T) {
	src := strings.Repeat("# Head\n\ntext here\n\n- a\n- b\n\n> quoted\n\n    code\n\n", 32)
	r := strings.NewReader(src)
	buf := make([]byte, 4096)
	var p Parser
	drain := func() {
		if err := p.Reset("t", r, buf); err != nil {
			t.Fatal(err)
		}
		for {
			if _, err := p.Next(); err != nil {
				return
			}
		}
	}
	drain() // Warm up: grow the container spine before measuring.
	if allocs := testing.AllocsPerRun(10, drain); allocs != 0 {
		t.Errorf("AllocsPerRun = %v, want 0", allocs)
	}
}

// nextNodes drains a parser with Next alone, which is what TryNextLiteral must
// agree with node for node.
func nextNodes(t *testing.T, src string, bufSize int) []Node {
	t.Helper()
	var p Parser
	if err := p.Reset(t.Name(), strings.NewReader(src), make([]byte, bufSize)); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	var nodes []Node
	for {
		n, err := p.Next()
		if err == io.EOF {
			return nodes
		} else if err != nil {
			t.Fatalf("Next: %v", err)
		}
		nodes = append(nodes, n)
	}
}

// TestParserTryNextLiteral runs every case through TryNextLiteral over a window
// holding the whole source, where no span can be missing: the node stream must
// match Next's and every literal must be the span's bytes.
func TestParserTryNextLiteral(t *testing.T) {
	const bufSize = 64 << 10
	for _, tt := range parseTests {
		t.Run(tt.name, func(t *testing.T) {
			want := nextNodes(t, tt.src, bufSize)
			var p Parser
			if err := p.Reset(t.Name(), strings.NewReader(tt.src), make([]byte, bufSize)); err != nil {
				t.Fatal(err)
			}
			for i := 0; ; i++ {
				n, lit, err := p.TryNextLiteral()
				if err == io.EOF {
					if i != len(want) {
						t.Fatalf("got %d nodes, want %d", i, len(want))
					}
					return
				} else if err != nil {
					t.Fatalf("node %d: TryNextLiteral: %v", i, err)
				}
				if i >= len(want) {
					t.Fatalf("node %d (%s) past the %d Next reported", i, n.Kind, len(want))
				}
				if n != want[i] {
					t.Fatalf("node %d = %+v, want %+v", i, n, want[i])
				}
				if got := tt.src[n.Start:n.End]; string(lit) != got {
					t.Fatalf("node %d (%s) literal = %q, want %q", i, n.Kind, lit, got)
				}
			}
		})
	}
}

// TestParserTryNextLiteralUnavailable pins the failure contract: a span the
// window cannot hold costs a zero Node and leaves it queued, so the following
// Next hands back the very node that failed.
func TestParserTryNextLiteralUnavailable(t *testing.T) {
	// A paragraph well past MinWindow, so its text node cannot be resident.
	src := "# h\n\n" + strings.Repeat("word ", 64) + "\n\ntail\n"
	want := nextNodes(t, src, MinWindow)

	var p Parser
	if err := p.Reset(t.Name(), strings.NewReader(src), make([]byte, MinWindow)); err != nil {
		t.Fatal(err)
	}
	misses := 0
	for i := 0; ; i++ {
		n, lit, err := p.TryNextLiteral()
		switch {
		case errors.Is(err, ErrLiteralUnavailable):
			misses++
			if n != (Node{}) || lit != nil {
				t.Fatalf("node %d: failed TryNextLiteral returned %+v %q, want zero", i, n, lit)
			}
			// The node was not consumed, so Next must still produce it.
			n, err = p.Next()
			if err != nil {
				t.Fatalf("node %d: Next after miss: %v", i, err)
			}
		case err == io.EOF:
			if i != len(want) {
				t.Fatalf("got %d nodes, want %d", i, len(want))
			}
			if misses == 0 {
				t.Fatal("no span outran the window, so the failure path went untested")
			}
			return
		case err != nil:
			t.Fatalf("node %d: TryNextLiteral: %v", i, err)
		default:
			if got := src[n.Start:n.End]; string(lit) != got {
				t.Fatalf("node %d (%s) literal = %q, want %q", i, n.Kind, lit, got)
			}
		}
		if i >= len(want) {
			t.Fatalf("node %d (%s) past the %d Next reported", i, n.Kind, len(want))
		}
		if n != want[i] {
			t.Fatalf("node %d = %+v, want %+v", i, n, want[i])
		}
	}
}

func TestParserTryNextLiteralNoAllocs(t *testing.T) {
	src := strings.Repeat("# Head\n\ntext here\n\n- a\n- b\n\n> quoted\n\n    code\n\n", 32)
	r := strings.NewReader(src)
	buf := make([]byte, 4096)
	var p Parser
	drain := func() {
		if err := p.Reset("t", r, buf); err != nil {
			t.Fatal(err)
		}
		for {
			if _, _, err := p.TryNextLiteral(); err != nil {
				return
			}
		}
	}
	drain() // Warm up: grow the container spine before measuring.
	if allocs := testing.AllocsPerRun(10, drain); allocs != 0 {
		t.Errorf("AllocsPerRun = %v, want 0", allocs)
	}
}

func FuzzParserBalance(f *testing.F) {
	for _, tt := range parseTests {
		f.Add(tt.src)
	}
	for _, tt := range lexTests {
		f.Add(tt.src)
	}
	f.Fuzz(func(t *testing.T, src string) {
		var p Parser
		p.MaxDepth = 16
		if err := p.Reset("f", strings.NewReader(src), make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		var open []Node
		limit := 8*len(src) + 16
		for i := 0; ; i++ {
			n, err := p.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				return // A capped or malformed document may legitimately fail.
			}
			if i > limit {
				t.Fatalf("Next did not terminate over %q", src)
			}
			if n.Start < 0 || n.End > Pos(len(src)) || n.Start > n.End {
				t.Fatalf("%s span %v..%v outside 0..%d", n.Kind, n.Start, n.End, len(src))
			}
			if len(open) > 0 && (n.IsOpen() || n.Kind.IsLeaf()) {
				if parent := open[len(open)-1].Kind; (parent == KindList) != (n.Kind == KindItem) {
					t.Fatalf("%s directly inside %s over %q", n.Kind, parent, src)
				}
			}
			switch {
			case n.IsOpen():
				open = append(open, n)
			case n.IsClose():
				if len(open) == 0 {
					t.Fatalf("%s close with nothing open over %q", n.Kind, src)
				}
				last := open[len(open)-1]
				open = open[:len(open)-1]
				if last.Kind != n.Kind || n.Start != last.Start || n.End < last.End {
					t.Fatalf("%s close %v..%v mispairs with %s open %v..%v over %q",
						n.Kind, n.Start, n.End, last.Kind, last.Start, last.End, src)
				}
			}
		}
		if len(open) != 0 {
			t.Fatalf("%d containers left open over %q", len(open), src)
		}
		if p.Depth() != 0 {
			t.Fatalf("Depth = %d after drain over %q", p.Depth(), src)
		}
	})
}
