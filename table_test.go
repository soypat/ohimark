package ohimark

import (
	"io"
	"strings"
	"testing"
)

var tableTests = []struct {
	name string
	src  string
	want []string
}{
	{"table basic", "| a | b |\n| :- | -: |\n| c | d |\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=2`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 "" attr=1`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a" attr=1`,
		`tablecell open 6..6 "" attr=2`,
		`text 6..7 "b"`,
		`tablecell close 6..7 "b" attr=2`,
		`tablerow close 0..9 "| a | b |"`,
		`tablerow open 22..22 "" attr=1`,
		`tablecell open 24..24 "" attr=1`,
		`text 24..25 "c"`,
		`tablecell close 24..25 "c" attr=1`,
		`tablecell open 28..28 "" attr=2`,
		`text 28..29 "d"`,
		`tablecell close 28..29 "d" attr=2`,
		`tablerow close 22..31 "| c | d |" attr=1`,
		`table close 0..31 "| a | b |\n| :- | -: |\n| c | d |" attr=2`,
		`document close 0..32 "| a | b |\n| :- | -: |\n| c | d |\n"`,
	}},
	{"table no outer pipes", "a | b\n:-: | ---\nc | d", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=2`,
		`tablerow open 0..0 ""`,
		`tablecell open 0..0 "" attr=3`,
		`text 0..1 "a"`,
		`tablecell close 0..1 "a" attr=3`,
		`tablecell open 4..4 ""`,
		`text 4..5 "b"`,
		`tablecell close 4..5 "b"`,
		`tablerow close 0..5 "a | b"`,
		`tablerow open 16..16 "" attr=1`,
		`tablecell open 16..16 "" attr=3`,
		`text 16..17 "c"`,
		`tablecell close 16..17 "c" attr=3`,
		`tablecell open 20..20 ""`,
		`text 20..21 "d"`,
		`tablecell close 20..21 "d"`,
		`tablerow close 16..21 "c | d" attr=1`,
		`table close 0..21 "a | b\n:-: | ---\nc | d" attr=2`,
		`document close 0..21 "a | b\n:-: | ---\nc | d"`,
	}},
	{"table header only", "| a |\n|---|\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablerow close 0..5 "| a |"`,
		`table close 0..11 "| a |\n|---|" attr=1`,
		`document close 0..12 "| a |\n|---|\n"`,
	}},
	{"table short row unpadded, long row cut", "| a | b |\n|---|---|\n| c |\n| d | e | f |\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=2`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablecell open 6..6 ""`,
		`text 6..7 "b"`,
		`tablecell close 6..7 "b"`,
		`tablerow close 0..9 "| a | b |"`,
		`tablerow open 20..20 "" attr=1`,
		`tablecell open 22..22 ""`,
		`text 22..23 "c"`,
		`tablecell close 22..23 "c"`,
		`tablerow close 20..25 "| c |" attr=1`,
		`tablerow open 26..26 "" attr=2`,
		`tablecell open 28..28 ""`,
		`text 28..29 "d"`,
		`tablecell close 28..29 "d"`,
		`tablecell open 32..32 ""`,
		`text 32..33 "e"`,
		`tablecell close 32..33 "e"`,
		`tablerow close 26..39 "| d | e | f |" attr=2`,
		`table close 0..39 "| a | b |\n|---|---|\n| c |\n| d | e | f |" attr=2`,
		`document close 0..40 "| a | b |\n|---|---|\n| c |\n| d | e | f |\n"`,
	}},
	{"table empty cells", "||\n|-|\n| |\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 1..1 ""`,
		`tablecell close 1..1 ""`,
		`tablerow close 0..2 "||"`,
		`tablerow open 7..7 "" attr=1`,
		`tablecell open 9..9 ""`,
		`tablecell close 9..9 ""`,
		`tablerow close 7..10 "| |" attr=1`,
		`table close 0..10 "||\n|-|\n| |" attr=1`,
		`document close 0..11 "||\n|-|\n| |\n"`,
	}},
	{"table space and tab padding", "|\ta\t|  b |\n| --- |:---:|\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=2`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablecell open 7..7 "" attr=3`,
		`text 7..8 "b"`,
		`tablecell close 7..8 "b" attr=3`,
		`tablerow close 0..10 "|\ta\t|  b |"`,
		`table close 0..24 "|\ta\t|  b |\n| --- |:---:|" attr=2`,
		`document close 0..25 "|\ta\t|  b |\n| --- |:---:|\n"`,
	}},
	// GFM spec example 200. An escaped pipe stays live inside a code span, so a
	// renderer skipping the escape's backslash prints "|" there too.
	{"table escaped pipes", "| f\\|oo  |\n| ------ |\n| b `\\|` az |\n| b **\\|** im |\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "f"`,
		`escape 3..5 "\\|"`,
		`text 5..7 "oo"`,
		`tablecell close 2..7 "f\\|oo"`,
		`tablerow close 0..10 "| f\\|oo  |"`,
		`tablerow open 22..22 "" attr=1`,
		`tablecell open 24..24 ""`,
		`text 24..26 "b "`,
		"codespan open 26..27 \"`\" attr=96",
		`escape 27..29 "\\|"`,
		"codespan close 26..30 \"`\\\\|`\" attr=96",
		`text 30..33 " az"`,
		"tablecell close 24..33 \"b `\\\\|` az\"",
		"tablerow close 22..35 \"| b `\\\\|` az |\" attr=1",
		`tablerow open 36..36 "" attr=2`,
		`tablecell open 38..38 ""`,
		`text 38..40 "b "`,
		`strong open 40..42 "**" attr=42`,
		`escape 42..44 "\\|"`,
		`strong close 40..46 "**\\|**" attr=42`,
		`text 46..49 " im"`,
		`tablecell close 38..49 "b **\\|** im"`,
		`tablerow close 36..51 "| b **\\|** im |" attr=2`,
		"table close 0..51 \"| f\\\\|oo  |\\n| ------ |\\n| b `\\\\|` az |\\n| b **\\\\|** im |\" attr=1",
		"document close 0..52 \"| f\\\\|oo  |\\n| ------ |\\n| b `\\\\|` az |\\n| b **\\\\|** im |\\n\"",
	}},
	{"table pipe splits code span", "| `a\\|b` | `c|d` |\n|-|-|-|\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=3`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		"codespan open 2..3 \"`\" attr=96",
		`text 3..4 "a"`,
		`escape 4..6 "\\|"`,
		`text 6..7 "b"`,
		"codespan close 2..8 \"`a\\\\|b`\" attr=96",
		"tablecell close 2..8 \"`a\\\\|b`\"",
		`tablecell open 11..11 ""`,
		"text 11..13 \"`c\"",
		"tablecell close 11..13 \"`c\"",
		`tablecell open 14..14 ""`,
		"text 14..16 \"d`\"",
		"tablecell close 14..16 \"d`\"",
		"tablerow close 0..18 \"| `a\\\\|b` | `c|d` |\"",
		"table close 0..26 \"| `a\\\\|b` | `c|d` |\\n|-|-|-|\" attr=3",
		"document close 0..27 \"| `a\\\\|b` | `c|d` |\\n|-|-|-|\\n\"",
	}},
	{"table pipe splits link tail", "| [x](y|z) |\n|-|-|\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=2`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..7 "[x](y"`,
		`tablecell close 2..7 "[x](y"`,
		`tablecell open 8..8 ""`,
		`text 8..10 "z)"`,
		`tablecell close 8..10 "z)"`,
		`tablerow close 0..12 "| [x](y|z) |"`,
		`table close 0..18 "| [x](y|z) |\n|-|-|" attr=2`,
		`document close 0..19 "| [x](y|z) |\n|-|-|\n"`,
	}},
	{"table emphasis flanks at cell edges", "| *(a** |\n| - |\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`emph open 2..3 "*" attr=42`,
		`text 3..5 "(a"`,
		`emph close 2..6 "*(a*" attr=42`,
		`text 6..7 "*"`,
		`tablecell close 2..7 "*(a**"`,
		`tablerow close 0..9 "| *(a** |"`,
		`table close 0..15 "| *(a** |\n| - |" attr=1`,
		`document close 0..16 "| *(a** |\n| - |\n"`,
	}},
	// --- what is not a table ---
	{"table count mismatch", "| a | b |\n| - |\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..9 "| a | b |"`,
		`softbreak 9..10 "\n"`,
		`text 10..15 "| - |"`,
		`paragraph close 0..15 "| a | b |\n| - |"`,
		`document close 0..16 "| a | b |\n| - |\n"`,
	}},
	{"table interrupts paragraph", "foo\n| a |\n| - |\nbar\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..3 "foo"`,
		`paragraph close 0..3 "foo"`,
		`table open 4..4 "" attr=1`,
		`tablerow open 4..4 ""`,
		`tablecell open 6..6 ""`,
		`text 6..7 "a"`,
		`tablecell close 6..7 "a"`,
		`tablerow close 4..9 "| a |"`,
		`tablerow open 16..16 "" attr=1`,
		`tablecell open 16..16 ""`,
		`text 16..19 "bar"`,
		`tablecell close 16..19 "bar"`,
		`tablerow close 16..19 "bar" attr=1`,
		`table close 4..19 "| a |\n| - |\nbar" attr=1`,
		`document close 0..20 "foo\n| a |\n| - |\nbar\n"`,
	}},
	// --- what ends a table ---
	{"table ends at blank line", "| a |\n| - |\n| b |\n\nc\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablerow close 0..5 "| a |"`,
		`tablerow open 12..12 "" attr=1`,
		`tablecell open 14..14 ""`,
		`text 14..15 "b"`,
		`tablecell close 14..15 "b"`,
		`tablerow close 12..17 "| b |" attr=1`,
		`table close 0..17 "| a |\n| - |\n| b |" attr=1`,
		`paragraph open 19..19 ""`,
		`text 19..20 "c"`,
		`paragraph close 19..20 "c"`,
		`document close 0..21 "| a |\n| - |\n| b |\n\nc\n"`,
	}},
	{"table ends at quote", "| a |\n|-|\n> q\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablerow close 0..5 "| a |"`,
		`table close 0..9 "| a |\n|-|" attr=1`,
		`blockquote open 10..12 "> "`,
		`paragraph open 12..12 ""`,
		`text 12..13 "q"`,
		`paragraph close 12..13 "q"`,
		`blockquote close 10..13 "> q"`,
		`document close 0..14 "| a |\n|-|\n> q\n"`,
	}},
	{"table ends at heading", "| a |\n|-|\n# h\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablerow close 0..5 "| a |"`,
		`table close 0..9 "| a |\n|-|" attr=1`,
		`heading open 10..12 "# " attr=1`,
		`text 12..13 "h"`,
		`heading close 10..13 "# h" attr=1`,
		`document close 0..14 "| a |\n|-|\n# h\n"`,
	}},
	{"table ends at thematic break", "| a |\n|-|\n---\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablerow close 0..5 "| a |"`,
		`table close 0..9 "| a |\n|-|" attr=1`,
		`thematicbreak 10..13 "---"`,
		`document close 0..14 "| a |\n|-|\n---\n"`,
	}},
	{"table ends at indented code", "| a |\n| - |\n    code\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablerow close 0..5 "| a |"`,
		`table close 0..11 "| a |\n| - |" attr=1`,
		`codeblock open 12..16 "    "`,
		`raw 16..21 "code\n"`,
		`codeblock close 12..20 "    code"`,
		`document close 0..21 "| a |\n| - |\n    code\n"`,
	}},
	{"table ends at bare pipe", "| a |\n| - |\n|\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablerow close 0..5 "| a |"`,
		`table close 0..11 "| a |\n| - |" attr=1`,
		`paragraph open 12..12 ""`,
		`text 12..13 "|"`,
		`paragraph close 12..13 "|"`,
		`document close 0..14 "| a |\n| - |\n|\n"`,
	}},
	{"table dashes alone", "abc\n---\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..3 "abc"`,
		`paragraph close 0..3 "abc"`,
		`thematicbreak 4..7 "---"`,
		`document close 0..8 "abc\n---\n"`,
	}},
	{"table colon without pipe", "abc\n:--\n", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 0..0 "" attr=1`,
		`text 0..3 "abc"`,
		`tablecell close 0..3 "abc" attr=1`,
		`tablerow close 0..3 "abc"`,
		`table close 0..7 "abc\n:--" attr=1`,
		`document close 0..8 "abc\n:--\n"`,
	}},
	{"table bullet delimiter", "| a |\n- | -\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..5 "| a |"`,
		`paragraph close 0..5 "| a |"`,
		`list open 6..8 "- " attr=45`,
		`item open 6..8 "- "`,
		`paragraph open 8..8 ""`,
		`text 8..11 "| -"`,
		`paragraph close 8..11 "| -"`,
		`item close 6..11 "- | -"`,
		`list close 6..11 "- | -" attr=45`,
		`document close 0..12 "| a |\n- | -\n"`,
	}},
	{"table indented delimiter", "| a |\n    | - |\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..5 "| a |"`,
		`softbreak 5..10 "\n    "`,
		`text 10..15 "| - |"`,
		`paragraph close 0..15 "| a |\n    | - |"`,
		`document close 0..16 "| a |\n    | - |\n"`,
	}},
	// --- containers ---
	{"table in blockquote", "> | a |\n> | - |\n> | b |\n| c |\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`table open 2..2 "" attr=1`,
		`tablerow open 2..2 ""`,
		`tablecell open 4..4 ""`,
		`text 4..5 "a"`,
		`tablecell close 4..5 "a"`,
		`tablerow close 2..7 "| a |"`,
		`tablerow open 18..18 "" attr=1`,
		`tablecell open 20..20 ""`,
		`text 20..21 "b"`,
		`tablecell close 20..21 "b"`,
		`tablerow close 18..23 "| b |" attr=1`,
		`table close 2..23 "| a |\n> | - |\n> | b |" attr=1`,
		`blockquote close 0..23 "> | a |\n> | - |\n> | b |"`,
		`paragraph open 24..24 ""`,
		`text 24..29 "| c |"`,
		`paragraph close 24..29 "| c |"`,
		`document close 0..30 "> | a |\n> | - |\n> | b |\n| c |\n"`,
	}},
	// No lazy continuation, so the delimiter row is not the quote's; cmark-gfm
	// would read it as a lazy paragraph line. Either way there is no table.
	{"table delimiter outside quote", "> | a |\n| - |\n", []string{
		`document open 0..0 ""`,
		`blockquote open 0..2 "> "`,
		`paragraph open 2..2 ""`,
		`text 2..7 "| a |"`,
		`paragraph close 2..7 "| a |"`,
		`blockquote close 0..7 "> | a |"`,
		`paragraph open 8..8 ""`,
		`text 8..13 "| - |"`,
		`paragraph close 8..13 "| - |"`,
		`document close 0..14 "> | a |\n| - |\n"`,
	}},
	{"table in list item", "- | a |\n  | - |\n  | b |\n", []string{
		`document open 0..0 ""`,
		`list open 0..2 "- " attr=45`,
		`item open 0..2 "- "`,
		`table open 2..2 "" attr=1`,
		`tablerow open 2..2 ""`,
		`tablecell open 4..4 ""`,
		`text 4..5 "a"`,
		`tablecell close 4..5 "a"`,
		`tablerow close 2..7 "| a |"`,
		`tablerow open 18..18 "" attr=1`,
		`tablecell open 20..20 ""`,
		`text 20..21 "b"`,
		`tablecell close 20..21 "b"`,
		`tablerow close 18..23 "| b |" attr=1`,
		`table close 2..23 "| a |\n  | - |\n  | b |" attr=1`,
		`item close 0..23 "- | a |\n  | - |\n  | b |"`,
		`list close 0..23 "- | a |\n  | - |\n  | b |" attr=45`,
		`document close 0..24 "- | a |\n  | - |\n  | b |\n"`,
	}},
	{"table crlf unterminated", "| a |\r\n| - |\r\n| b |", []string{
		`document open 0..0 ""`,
		`table open 0..0 "" attr=1`,
		`tablerow open 0..0 ""`,
		`tablecell open 2..2 ""`,
		`text 2..3 "a"`,
		`tablecell close 2..3 "a"`,
		`tablerow close 0..5 "| a |"`,
		`tablerow open 14..14 "" attr=1`,
		`tablecell open 16..16 ""`,
		`text 16..17 "b"`,
		`tablecell close 16..17 "b"`,
		`tablerow close 14..19 "| b |" attr=1`,
		`table close 0..19 "| a |\r\n| - |\r\n| b |" attr=1`,
		`document close 0..19 "| a |\r\n| - |\r\n| b |"`,
	}},
}

func init() { parseTests = append(parseTests, tableTests...) }

// TestTableCellContainment pins the inline scan's bound: everything between a
// cell's open and close lies inside the cell, so no construct reaches across a
// pipe into the next one.
func TestTableCellContainment(t *testing.T) {
	for _, tt := range tableTests {
		t.Run(tt.name, func(t *testing.T) {
			var cell []Node // Nodes since the open cell, which closes last.
			in := false
			for _, n := range nextNodes(t, tt.src, MinWindow) {
				switch {
				case n.Kind == KindTableCell && n.IsOpen():
					in, cell = true, cell[:0]
				case n.Kind == KindTableCell:
					for _, c := range cell {
						if c.Start < n.Start || c.End > n.End {
							t.Fatalf("%s %v..%v escapes cell %v..%v", c.Kind, c.Start, c.End, n.Start, n.End)
						}
					}
					in = false
				case in:
					cell = append(cell, n)
				}
			}
		})
	}
}

// TestTableMaxColumns proves the column cap is a fallback and not a failure:
// past it the lines read as a paragraph, and the cap counts columns exactly.
func TestTableMaxColumns(t *testing.T) {
	const cols = 300 // Past the default cap.
	src := strings.Repeat("|a", cols) + "|\n" + strings.Repeat("|-", cols) + "|\n"
	for _, tt := range []struct {
		max  int
		want Kind
	}{{0, KindParagraph}, {cols - 1, KindParagraph}, {cols, KindTable}} {
		var p Parser
		p.MaxTableColumns = tt.max
		if err := p.Reset("t", strings.NewReader(src), make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		p.Next() // Document.
		n, err := p.Next()
		if err != nil {
			t.Fatal(err)
		}
		if n.Kind != tt.want {
			t.Fatalf("MaxTableColumns=%d: first block %s, want %s", tt.max, n.Kind, tt.want)
		}
		if n.Kind == KindTable && n.Attr != cols {
			t.Fatalf("MaxTableColumns=%d: table Attr = %d, want %d", tt.max, n.Attr, cols)
		}
		for {
			if _, err := p.Next(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("MaxTableColumns=%d: %v", tt.max, err)
			}
		}
	}
}

// FuzzTable checks what the balance fuzzer cannot see: rows count up from the
// header, no row outgrows its table, alignments stay in range, and nothing in a
// cell escapes it or its line.
func FuzzTable(f *testing.F) {
	for _, tt := range tableTests {
		f.Add(tt.src)
	}
	f.Add("A\x00\n|-") // NUL once read as a line end mid-cell.
	f.Fuzz(func(t *testing.T, src string) {
		var p Parser
		p.MaxDepth = 16
		p.MaxTableColumns = 8
		if err := p.Reset("f", strings.NewReader(src), make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		var table, row, cell Node // The open ones; tables do not nest.
		var rows, cells int32
		var inCell []Node
		for {
			n, err := p.Next()
			if err != nil {
				return // A capped or malformed document may legitimately fail.
			}
			switch {
			case n.Kind == KindTable && n.IsOpen():
				if n.Attr <= 0 || n.Attr > 8 {
					t.Fatalf("table Attr %d outside 1..8 over %q", n.Attr, src)
				}
				table, rows = n, 0
			case n.Kind == KindTableRow && n.IsOpen():
				if n.Attr != rows {
					t.Fatalf("row Attr %d, want %d over %q", n.Attr, rows, src)
				}
				row, rows, cells = n, rows+1, 0
			case n.Kind == KindTableCell && n.IsOpen():
				if cells++; cells > table.Attr {
					t.Fatalf("row %v holds over %d cells over %q", row.Start, table.Attr, src)
				}
				if n.Attr < 0 || n.Attr > AlignCenter {
					t.Fatalf("cell Attr %d over %q", n.Attr, src)
				}
				cell, inCell = n, inCell[:0]
			case n.Kind == KindTableCell:
				for _, c := range inCell {
					if c.Start < n.Start || c.End > n.End {
						t.Fatalf("%s %v..%v escapes cell %v..%v over %q", c.Kind, c.Start, c.End, n.Start, n.End, src)
					}
				}
				if strings.ContainsAny(src[n.Start:n.End], "\r\n") {
					t.Fatalf("cell %v..%v crosses a line over %q", n.Start, n.End, src)
				}
				cell = Node{}
			case cell.Kind == KindTableCell:
				inCell = append(inCell, n)
			}
		}
	})
}
