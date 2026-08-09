package ohimark

import (
	"strings"
	"testing"
)

// Attribute values carried by emphasis and code spans: the delimiter byte.
const (
	attrStar  = 42
	attrUnder = 95
	attrTick  = 96
)

// inlineTests covers the inline layer. They run through every check the block
// table does, and seed the parser fuzzer.
var inlineTests = []struct {
	name string
	src  string
	want []string
}{
	// --- text, escapes, entities ---
	{"plain text", "abc\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..3 "abc"`,
		`paragraph close 0..3 "abc"`,
		`document close 0..4 "abc\n"`,
	}},
	{"escape", `\*a` + "\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`escape 0..2 "\\*"`,
		`text 2..3 "a"`,
		`paragraph close 0..3 "\\*a"`,
		`document close 0..4 "\\*a\n"`,
	}},
	{"escaped stars are not emphasis", `\*a\*` + "\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`escape 0..2 "\\*"`,
		`text 2..3 "a"`,
		`escape 3..5 "\\*"`,
		`paragraph close 0..5 "\\*a\\*"`,
		`document close 0..6 "\\*a\\*\n"`,
	}},
	{"entity", "&amp;x\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`entity 0..5 "&amp;"`,
		`text 5..6 "x"`,
		`paragraph close 0..6 "&amp;x"`,
		`document close 0..7 "&amp;x\n"`,
	}},

	// --- code spans ---
	{"code span", bt + "a" + bt + "\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		"codespan open 0..1 \"`\" attr=96",
		`text 1..2 "a"`,
		"codespan close 0..3 \"`a`\" attr=96",
		"paragraph close 0..3 \"`a`\"",
		"document close 0..4 \"`a`\\n\"",
	}},
	{"code span double tick", bt + bt + "a" + bt + bt + "\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		"codespan open 0..2 \"``\" attr=96",
		`text 2..3 "a"`,
		"codespan close 0..5 \"``a``\" attr=96",
		"paragraph close 0..5 \"``a``\"",
		"document close 0..6 \"``a``\\n\"",
	}},
	{"unmatched tick is text", bt + "a\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		"text 0..2 \"`a\"",
		"paragraph close 0..2 \"`a\"",
		"document close 0..3 \"`a\\n\"",
	}},
	{"code span holds markers verbatim", bt + "*a*" + bt + "\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		"codespan open 0..1 \"`\" attr=96",
		`text 1..4 "*a*"`,
		"codespan close 0..5 \"`*a*`\" attr=96",
		"paragraph close 0..5 \"`*a*`\"",
		"document close 0..6 \"`*a*`\\n\"",
	}},
	{"unequal tick runs do not pair", bt + "a" + bt + bt + "\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		"text 0..4 \"`a``\"",
		"paragraph close 0..4 \"`a``\"",
		"document close 0..5 \"`a``\\n\"",
	}},

	// --- emphasis ---
	{"emph star", "*a*\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`emph open 0..1 "*" attr=42`,
		`text 1..2 "a"`,
		`emph close 0..3 "*a*" attr=42`,
		`paragraph close 0..3 "*a*"`,
		`document close 0..4 "*a*\n"`,
	}},
	{"strong star", "**a**\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`strong open 0..2 "**" attr=42`,
		`text 2..3 "a"`,
		`strong close 0..5 "**a**" attr=42`,
		`paragraph close 0..5 "**a**"`,
		`document close 0..6 "**a**\n"`,
	}},
	{"emph inside strong", "***a***\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`emph open 0..1 "*" attr=42`,
		`strong open 1..3 "**" attr=42`,
		`text 3..4 "a"`,
		`strong close 1..6 "**a**" attr=42`,
		`emph close 0..7 "***a***" attr=42`,
		`paragraph close 0..7 "***a***"`,
		`document close 0..8 "***a***\n"`,
	}},
	{"emph underscore", "_a_\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`emph open 0..1 "_" attr=95`,
		`text 1..2 "a"`,
		`emph close 0..3 "_a_" attr=95`,
		`paragraph close 0..3 "_a_"`,
		`document close 0..4 "_a_\n"`,
	}},
	// Underscore does not emphasize inside a word, so snake_case survives.
	{"intraword underscore is text", "a_b_c\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..5 "a_b_c"`,
		`paragraph close 0..5 "a_b_c"`,
		`document close 0..6 "a_b_c\n"`,
	}},
	{"intraword star emphasizes", "a*b*c\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`emph open 1..2 "*" attr=42`,
		`text 2..3 "b"`,
		`emph close 1..4 "*b*" attr=42`,
		`text 4..5 "c"`,
		`paragraph close 0..5 "a*b*c"`,
		`document close 0..6 "a*b*c\n"`,
	}},
	{"unmatched star is text", "*a\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..2 "*a"`,
		`paragraph close 0..2 "*a"`,
		`document close 0..3 "*a\n"`,
	}},
	{"opener with no closer keeps leftovers", "**a*\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "*"`,
		`emph open 1..2 "*" attr=42`,
		`text 2..3 "a"`,
		`emph close 1..4 "*a*" attr=42`,
		`paragraph close 0..4 "**a*"`,
		`document close 0..5 "**a*\n"`,
	}},
	{"emphasis spans lines", "*a\nb*\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`emph open 0..1 "*" attr=42`,
		`text 1..2 "a"`,
		`softbreak 2..3 "\n"`,
		`text 3..4 "b"`,
		`emph close 0..5 "*a\nb*" attr=42`,
		`paragraph close 0..5 "*a\nb*"`,
		`document close 0..6 "*a\nb*\n"`,
	}},

	// --- links, images, autolinks ---
	{"link", "[a](b)\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`link open 0..1 "["`,
		`dest 4..5 "b"`,
		`text 1..2 "a"`,
		`link close 0..6 "[a](b)"`,
		`paragraph close 0..6 "[a](b)"`,
		`document close 0..7 "[a](b)\n"`,
	}},
	{"image", "![a](b)\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`image open 0..2 "!["`,
		`dest 5..6 "b"`,
		`text 2..3 "a"`,
		`image close 0..7 "![a](b)"`,
		`paragraph close 0..7 "![a](b)"`,
		`document close 0..8 "![a](b)\n"`,
	}},
	{"link with title", `[a](b "t")` + "\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`link open 0..1 "["`,
		`dest 4..5 "b"`,
		`title 7..8 "t"`,
		`text 1..2 "a"`,
		`link close 0..10 "[a](b \"t\")"`,
		`paragraph close 0..10 "[a](b \"t\")"`,
		`document close 0..11 "[a](b \"t\")\n"`,
	}},
	{"link with empty dest", "[a]()\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`link open 0..1 "["`,
		`text 1..2 "a"`,
		`link close 0..5 "[a]()"`,
		`paragraph close 0..5 "[a]()"`,
		`document close 0..6 "[a]()\n"`,
	}},
	{"emphasis inside link text", "[*a*](b)\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`link open 0..1 "["`,
		`dest 6..7 "b"`,
		`emph open 1..2 "*" attr=42`,
		`text 2..3 "a"`,
		`emph close 1..4 "*a*" attr=42`,
		`link close 0..8 "[*a*](b)"`,
		`paragraph close 0..8 "[*a*](b)"`,
		`document close 0..9 "[*a*](b)\n"`,
	}},
	{"bracket without tail is text", "[a] b\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..5 "[a] b"`,
		`paragraph close 0..5 "[a] b"`,
		`document close 0..6 "[a] b\n"`,
	}},
	{"autolink", "<http://a>\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`autolink 0..10 "<http://a>"`,
		`paragraph close 0..10 "<http://a>"`,
		`document close 0..11 "<http://a>\n"`,
	}},
	{"email autolink", "<a@b.co>\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`autolink 0..8 "<a@b.co>"`,
		`paragraph close 0..8 "<a@b.co>"`,
		`document close 0..9 "<a@b.co>\n"`,
	}},
	{"angle without scheme is text", "<a b>\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..5 "<a b>"`,
		`paragraph close 0..5 "<a b>"`,
		`document close 0..6 "<a b>\n"`,
	}},

	// --- breaks ---
	{"soft break", "a\nb\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`softbreak 1..2 "\n"`,
		`text 2..3 "b"`,
		`paragraph close 0..3 "a\nb"`,
		`document close 0..4 "a\nb\n"`,
	}},
	{"hard break from two spaces", "a  \nb\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`hardbreak 1..4 "  \n"`,
		`text 4..5 "b"`,
		`paragraph close 0..5 "a  \nb"`,
		`document close 0..6 "a  \nb\n"`,
	}},
	{"one trailing space is a soft break", "a \nb\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`softbreak 1..3 " \n"`,
		`text 3..4 "b"`,
		`paragraph close 0..4 "a \nb"`,
		`document close 0..5 "a \nb\n"`,
	}},
	{"hard break from backslash", "a\\\nb\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`hardbreak 1..3 "\\\n"`,
		`text 3..4 "b"`,
		`paragraph close 0..4 "a\\\nb"`,
		`document close 0..5 "a\\\nb\n"`,
	}},
	// An escaped backslash is content, so the line ends with no break marker.
	{"escaped backslash is not a break", `a\\` + "\nb\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`escape 1..3 "\\\\"`,
		`softbreak 3..4 "\n"`,
		`text 4..5 "b"`,
		`paragraph close 0..5 "a\\\\\nb"`,
		`document close 0..6 "a\\\\\nb\n"`,
	}},
	// A break's span reaches the next line's first content byte, so the stripped
	// container prefix belongs to no node.
	{"break skips a container prefix", "> a\n> b\n", []string{
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

	// --- inlines in other leaves ---
	{"heading holds inlines", "# *a*\n", []string{
		`document open 0..0 ""`,
		`heading open 0..2 "# " attr=1`,
		`emph open 2..3 "*" attr=42`,
		`text 3..4 "a"`,
		`emph close 2..5 "*a*" attr=42`,
		`heading close 0..5 "# *a*" attr=1`,
		`document close 0..6 "# *a*\n"`,
	}},
	// A heading's closing sequence is not content, so no emphasis reaches it.
	{"heading closing sequence is not inline", "# *a* #\n", []string{
		`document open 0..0 ""`,
		`heading open 0..2 "# " attr=1`,
		`emph open 2..3 "*" attr=42`,
		`text 3..4 "a"`,
		`emph close 2..5 "*a*" attr=42`,
		`heading close 0..7 "# *a* #" attr=1`,
		`document close 0..8 "# *a* #\n"`,
	}},
	// The rule that a run which both opens and closes may only pair when the two
	// lengths do not sum to a multiple of three. Without it the outer "*" would
	// pair with the first "**" and the nesting would come out inverted.
	{"rule of three", "*foo**bar**baz*\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`emph open 0..1 "*" attr=42`,
		`text 1..4 "foo"`,
		`strong open 4..6 "**" attr=42`,
		`text 6..9 "bar"`,
		`strong close 4..11 "**bar**" attr=42`,
		`text 11..14 "baz"`,
		`emph close 0..15 "*foo**bar**baz*" attr=42`,
		`paragraph close 0..15 "*foo**bar**baz*"`,
		`document close 0..16 "*foo**bar**baz*\n"`,
	}},
	{"nested runs stay nested", "*a**b*c*\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`emph open 0..1 "*" attr=42`,
		`text 1..5 "a**b"`,
		`emph close 0..6 "*a**b*" attr=42`,
		`text 6..8 "c*"`,
		`paragraph close 0..8 "*a**b*c*"`,
		`document close 0..9 "*a**b*c*\n"`,
	}},
	// A run bridged by a match may not reach past it afterwards. Left live, the
	// run at 3 would pair with the one at 5 while the run at 2 pairs with the
	// one at 4, and the two emphasis spans would cross instead of nesting.
	{"a bridged run cannot match outward", "***_*_\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..2 "**"`,
		`emph open 2..3 "*" attr=42`,
		`text 3..4 "_"`,
		`emph close 2..5 "*_*" attr=42`,
		`text 5..6 "_"`,
		`paragraph close 0..6 "***_*_"`,
		`document close 0..7 "***_*_\n"`,
	}},
	{"link title in single quotes", "[a](b 'c')\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`link open 0..1 "["`,
		`dest 4..5 "b"`,
		`title 7..8 "c"`,
		`text 1..2 "a"`,
		`link close 0..10 "[a](b 'c')"`,
		`paragraph close 0..10 "[a](b 'c')"`,
		`document close 0..11 "[a](b 'c')\n"`,
	}},
	{"unterminated tail is text", "[a](b\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..5 "[a](b"`,
		`paragraph close 0..5 "[a](b"`,
		`document close 0..6 "[a](b\n"`,
	}},
	{"balanced parens in dest", "[a](b(c))\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`link open 0..1 "["`,
		`dest 4..8 "b(c)"`,
		`text 1..2 "a"`,
		`link close 0..9 "[a](b(c))"`,
		`paragraph close 0..9 "[a](b(c))"`,
		`document close 0..10 "[a](b(c))\n"`,
	}},
	{"escaped paren in dest", `[a](b\)c)` + "\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`link open 0..1 "["`,
		`dest 4..8 "b\\)c"`,
		`text 1..2 "a"`,
		`link close 0..9 "[a](b\\)c)"`,
		`paragraph close 0..9 "[a](b\\)c)"`,
		`document close 0..10 "[a](b\\)c)\n"`,
	}},
	// A link's text may not hold another link, so the inner one wins and the
	// outer brackets fall back to text.
	{"nested links keep the inner one", "[a [b](c)](d)\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..3 "[a "`,
		`link open 3..4 "["`,
		`dest 7..8 "c"`,
		`text 4..5 "b"`,
		`link close 3..9 "[b](c)"`,
		`text 9..13 "](d)"`,
		`paragraph close 0..13 "[a [b](c)](d)"`,
		`document close 0..14 "[a [b](c)](d)\n"`,
	}},
	// An image may sit inside a link, so matching one leaves outer brackets live.
	{"image inside link", "[![a](b)](c)\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`link open 0..1 "["`,
		`dest 10..11 "c"`,
		`image open 1..3 "!["`,
		`dest 6..7 "b"`,
		`text 3..4 "a"`,
		`image close 1..8 "![a](b)"`,
		`link close 0..12 "[![a](b)](c)"`,
		`paragraph close 0..12 "[![a](b)](c)"`,
		`document close 0..13 "[![a](b)](c)\n"`,
	}},
	{"empty angles are text", "<>\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..2 "<>"`,
		`paragraph close 0..2 "<>"`,
		`document close 0..3 "<>\n"`,
	}},
	// Each construct that may interrupt a paragraph ends the inline scan, so the
	// block loop sees the line untouched.
	{"bullet interrupts a paragraph", "a\n- b\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`paragraph close 0..1 "a"`,
		`list open 2..4 "- " attr=45`,
		`item open 2..4 "- "`,
		`paragraph open 4..4 ""`,
		`text 4..5 "b"`,
		`paragraph close 4..5 "b"`,
		`item close 2..5 "- b"`,
		`list close 2..5 "- b" attr=45`,
		`document close 0..6 "a\n- b\n"`,
	}},
	{"ordered marker interrupts a paragraph", "a\n1. b\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`paragraph close 0..1 "a"`,
		`list open 2..5 "1. " attr=1 ordered`,
		`item open 2..5 "1. "`,
		`paragraph open 5..5 ""`,
		`text 5..6 "b"`,
		`paragraph close 5..6 "b"`,
		`item close 2..6 "1. b"`,
		`list close 2..6 "1. b" attr=1 ordered`,
		`document close 0..7 "a\n1. b\n"`,
	}},
	{"fence interrupts a paragraph", "a\n~~~\nx\n~~~\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`paragraph close 0..1 "a"`,
		`codeblock open 2..6 "~~~\n" attr=3 fenced`,
		`raw 6..8 "x\n"`,
		`codeblock close 2..11 "~~~\nx\n~~~" attr=3 fenced`,
		`document close 0..12 "a\n~~~\nx\n~~~\n"`,
	}},
	{"blockquote interrupts a paragraph", "a\n> b\n", []string{
		`document open 0..0 ""`,
		`paragraph open 0..0 ""`,
		`text 0..1 "a"`,
		`paragraph close 0..1 "a"`,
		`blockquote open 2..4 "> "`,
		`paragraph open 4..4 ""`,
		`text 4..5 "b"`,
		`paragraph close 4..5 "b"`,
		`blockquote close 2..5 "> b"`,
		`document close 0..6 "a\n> b\n"`,
	}},
	{"code block content is not inline", "    *a*\n", []string{
		`document open 0..0 ""`,
		`codeblock open 0..4 "    "`,
		`raw 4..8 "*a*\n"`,
		`codeblock close 0..7 "    *a*"`,
		`document close 0..8 "    *a*\n"`,
	}},
}

func init() { parseTests = append(parseTests, inlineTests...) }

// TestInlineTextCoverage is the inline counterpart to the lexer's token
// coverage: within one leaf block, the emitted inline spans and the delimiters
// they sit between tile the block's content, so no byte is dropped or doubled.
func TestInlineTextCoverage(t *testing.T) {
	for _, tt := range inlineTests {
		t.Run(tt.name, func(t *testing.T) {
			var p Parser
			if err := p.Reset("t", strings.NewReader(tt.src), make([]byte, MinWindow)); err != nil {
				t.Fatal(err)
			}
			var at Pos
			for {
				n, err := p.Next()
				if err != nil {
					break
				}
				if !n.Kind.IsInline() || n.IsClose() {
					continue // Closes reach back over content already walked.
				}
				if n.Start < at {
					t.Fatalf("%s at %v..%v overlaps content emitted through %v",
						n.Kind, n.Start, n.End, at)
				}
				at = n.End
			}
		})
	}
}

func TestInlineNoAllocs(t *testing.T) {
	src := strings.Repeat("some *emph* and **strong** with `code` plus [a](b) and <http://x>\n"+
		"a second line with \\* escapes and &amp; entities\n\n", 32)
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
	drain() // Warm up: grow the memo before measuring.
	if allocs := testing.AllocsPerRun(10, drain); allocs != 0 {
		t.Errorf("AllocsPerRun = %v, want 0", allocs)
	}
}

// TestInlineDelimCap proves the memo cap is a degradation and not a failure:
// past it the block still parses, still balances, and still covers every byte.
func TestInlineDelimCap(t *testing.T) {
	src := strings.Repeat("*a* ", 200) + "\n"
	for _, cap := range []int{1, 2, 8, 64, 1024} {
		var p Parser
		p.MaxInlineDelims = cap
		if err := p.Reset("t", strings.NewReader(src), make([]byte, MinWindow)); err != nil {
			t.Fatal(err)
		}
		var at Pos
		depth := 0
		for {
			n, err := p.Next()
			if err != nil {
				break
			}
			if n.IsOpen() {
				depth++
			} else if n.IsClose() {
				depth--
			}
			if depth < 0 {
				t.Fatalf("cap=%d: unbalanced at %v", cap, n.Start)
			}
			if n.Kind.IsInline() && !n.IsClose() {
				if n.Start < at {
					t.Fatalf("cap=%d: %s at %v overlaps through %v", cap, n.Kind, n.Start, at)
				}
				at = n.End
			}
		}
		if depth != 0 {
			t.Fatalf("cap=%d: %d blocks left open", cap, depth)
		}
		if err := p.Err(); err != nil {
			t.Fatalf("cap=%d: %v", cap, err)
		}
	}
}
