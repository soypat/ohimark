package ohimark

// Inline content is resolved in two passes over one leaf block. Pass one scans
// forward recording delimiter runs, escapes, entities and breaks. Pass two walks
// that record emitting nodes; the gaps between records become KindText.
//
// Nothing is re-read: a match is known from positions alone, so `*foo*` needs
// the paragraph scanned to its end before EmphOpen can be emitted, but never
// needs its bytes twice. Breaks are recorded too, which lets pass two resume at
// a break's end rather than re-deriving the next line's stripped prefix.

// delim is one memoized delimiter run, escape, entity or line break.
type delim struct {
	start Pos
	end   Pos   // For a break, the first content byte of the next line.
	match int32 // Index of the paired delim, or -1.
	n     int32 // Run length in bytes.
	// before and after are the flanking runes, captured from [Lexer.PrevRune]
	// and [Lexer.PeekRune] as the run went by. 8 bytes that save the resolve
	// pass from decoding: by then the cursor is at the block's end.
	before, after rune
	tok           Token // TokStar, TokTick, TokBracketL, TokEscape, TokNewline...
	flags         delimFlags
}

type delimFlags uint8

const (
	dCanOpen delimFlags = 1 << iota
	dCanClose
	dActive // Not yet consumed by a match.
	dHard   // Break: the line ended with two or more spaces.
	dImage  // Bracket preceded by '!'.
)

// inlineBegin arms the passes over the leaf block's content from start.
//
// TODO: implement.
func (p *Parser) inlineBegin(start Pos) { panic("ohimark: todo") }

// inlineScan is pass one. False when MaxInlineDelims was exceeded, which sends
// the caller to the no-memo path.
//
// TODO: implement.
func (p *Parser) inlineScan() (ok bool) { panic("ohimark: todo") }

// inlineResolve pairs delimiters in spec order: backtick runs by equal length
// first, since a code span swallows everything between its ticks; then brackets,
// since a link's text may not contain another link; then emphasis over the rest.
//
// TODO: implement.
func (p *Parser) inlineResolve() { panic("ohimark: todo") }

// inlineNext is pass two: emit the next node, or false when content is
// exhausted.
//
// TODO: implement.
func (p *Parser) inlineNext() (Node, bool) { panic("ohimark: todo") }

// pushDelim appends to the memo, false once the cap is hit.
//
// TODO: implement.
func (p *Parser) pushDelim(d delim) bool { panic("ohimark: todo") }

// flanking applies CommonMark's left- and right-flanking rules to d.before and
// d.after via [isSpaceRune] and [isPunctRune]. A zero rune is a file edge, which
// both rules read as whitespace.
//
// TODO: implement.
func (p *Parser) flanking(d *delim) (canOpen, canClose bool) { panic("ohimark: todo") }

// scanLinkTail reads `(dest "title")` after a matched ']', producing the Dest
// and Title nodes queued behind the Link or Image open node.
//
// TODO: implement.
func (p *Parser) scanLinkTail(from Pos) (dest, title Node, end Pos, ok bool) {
	panic("ohimark: todo")
}

// scanAutolink reads <scheme:rest> or <user@host> as one node.
//
// TODO: implement.
func (p *Parser) scanAutolink(from Pos) (n Node, ok bool) { panic("ohimark: todo") }
