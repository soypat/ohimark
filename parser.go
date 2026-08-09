package ohimark

import "io"

const (
	defaultMaxDepth  = 64
	defaultMaxDelims = 1024

	maxATXLevel    = 6 // Hashes past this are text, not a heading.
	maxBlockIndent = 3 // Indent past which a line is code, not a block marker.
	codeIndent     = 4 // Columns of indent that make a line indented code.
)

// frame is one open block: the node emitted when it opened plus whatever its
// continuation rule needs. Leaf blocks share the stack with containers — only
// the innermost block can be a leaf, so closing order is unaffected.
type frame struct {
	node  Node
	col   int32 // KindItem, KindCodeBlock: the column its content starts at.
	fence int32 // KindCodeBlock: opening fence run length; 0 when indented.
	count int32 // KindList: items opened so far, which is the next ordinal.
	ch    byte  // KindCodeBlock: fence character.
	loose bool  // KindList: an item followed a blank line.
	blank bool  // KindList, KindItem: the last line inside it was blank.

	// An indented block's blank lines are content only when more indented lines
	// follow, which is not known until they do. blankAt and blankEnd bracket a
	// run held back pending that answer; see [Parser.indentedLine].
	blankAt  Pos
	blankEnd Pos
	holding  bool // A run is held and its fate still unknown.
	replay   bool // The run proved interior and is being re-read as content.
}

// mark is a cursor position that a probe can return to. [Lexer.Seek] alone
// cannot: the column is unrecoverable without rescanning from the line start,
// and the block rules are all stated in columns.
type mark struct {
	pos  int64
	col  int32
	prev rune
}

// Parser turns markdown into AST events. Containers arrive as an open node and
// later a close node; the nodes between are their content. The parser holds no
// document text: every [Node] is a span the caller reads back through the
// io.ReaderAt it supplied.
//
// Memory is bounded by three things: the window buffer, MaxDepth nestings, and
// MaxInlineDelims delimiter runs per leaf block. Nothing else grows with input.
//
// The block layer reads the [Lexer]'s rune cursor rather than its tokens.
// Container prefixes are defined in columns, and a single space token may span
// a container boundary — "  " belonging half to an item's indent and half to
// its content — which no token stream can split. Tokens serve the inline layer,
// which runs inside one leaf block's span where columns no longer matter.
//
// Parser embeds its Lexer by value, so the ordinary caller builds one object and
// runs one Next loop. Callers wanting raw tokens use a [Lexer] directly.
type Parser struct {
	l Lexer

	// stack is the open-block spine, outermost first. stack[0] is the document.
	stack []frame

	// pending holds the nodes one line produced, drained before the next line is
	// read. A line queues at most one close and one open per open block, plus a
	// leaf and its attribute, so Reset sizes this from MaxDepth.
	pending []Node
	ipend   int

	// contentEnd is the offset past the last byte that counts as content, which
	// is where a close node lands. It trails the cursor: a close is queued
	// before the current line's content is read.
	contentEnd Pos

	started bool
	done    bool
	err     error

	// delims memoizes the current leaf block's runs, escapes, entities and
	// breaks, and pairs the constructs resolution made from them. Both are
	// cleared per block and capped by MaxInlineDelims, and neither is grown
	// ahead of use: a document with no inlines never pays for the memo.
	delims []delim
	pairs  []pair

	// Emission cursor over delims during a leaf block's second pass.
	inInline   bool
	emitIdx    int32
	emitPair   int32
	emitPhase  uint8
	emitPos    Pos
	inlineStop Pos // Content limit when known up front, else negative.
	inlineEnd  Pos // Where the content ends, once the scan has found it.
	leafEnd    Pos // Where the leaf block's close node lands.

	// MaxDepth caps block nesting. <=0 uses an internal default.
	MaxDepth int
	// MaxInlineDelims caps memoized delimiter runs per leaf block. Exceeding it
	// is not an error: the parser rewinds to the block start and re-scans
	// without the memo, trading reads for memory. <=0 uses an internal default.
	MaxInlineDelims int
}

// Reset points the parser at the start of r, reading through buf. Forwards to
// [Lexer.Reset]; buf has the same constraints. Tuning fields and the internal
// slices survive across resets.
func (p *Parser) Reset(source string, r io.ReaderAt, buf []byte) error {
	p.stack = p.stack[:0]
	p.delims, p.pairs = p.delims[:0], p.pairs[:0]
	p.pending, p.ipend = p.pending[:0], 0
	p.contentEnd, p.leafEnd = 0, 0
	p.inInline = false
	p.emitIdx, p.emitPair, p.emitPhase = 0, -1, phaseGap
	p.started, p.done = false, false
	p.err = nil
	if n := 2*p.maxDepth() + 8; cap(p.pending) < n {
		p.pending = make([]Node, 0, n)
	}
	if err := p.l.Reset(source, r, 0, buf); err != nil {
		return err
	}
	return nil
}

// Next returns the next node. Returns io.EOF once, after the document's close
// node, and the latched error on every call after a failure.
func (p *Parser) Next() (Node, error) {
	n, err := p.peek()
	if err != nil {
		return Node{}, err
	}
	p.ipend++
	return n, nil
}

// TryNextLiteral is [Parser.Next] plus the node's source bytes, for a node whose
// span the window still holds whole. It never reads: a span the cursor has left
// behind, or one longer than the window, fails with [ErrLiteralUnavailable] and
// leaves the node queued, so the following Next returns it. That is the only
// contract a stream can keep, where seeking back is not a read but an error.
//
// A close node spans its whole construct and a code block its whole body, so
// both fail for all but small ones. lit aliases the window buffer, which the
// next call on p invalidates.
func (p *Parser) TryNextLiteral() (n Node, lit []byte, err error) {
	n, err = p.peek()
	if err != nil {
		return Node{}, nil, err
	}
	lit, ok := p.l.Resident(n.Start, n.End)
	if !ok {
		return Node{}, nil, ErrLiteralUnavailable
	}
	p.ipend++
	return n, lit, nil
}

// peek runs the machine to the next node without consuming it.
func (p *Parser) peek() (Node, error) {
	if p.ipend < len(p.pending) {
		return p.pending[p.ipend], nil
	}
	p.pending, p.ipend = p.pending[:0], 0
	if err := p.Err(); err != nil {
		return Node{}, err
	}
	for len(p.pending) == 0 {
		if !p.step() {
			if err := p.Err(); err != nil {
				return Node{}, err
			}
			return Node{}, io.EOF
		}
	}
	return p.pending[0], nil
}

// Err returns the first error the parser or its lexer hit, or nil if the input
// merely ended.
func (p *Parser) Err() error {
	if p.err != nil {
		return p.err
	}
	return p.l.Err()
}

// Depth returns how many blocks are currently open, the document included.
func (p *Parser) Depth() int { return len(p.stack) }

// Lexer returns the embedded tokenizer. Advancing it out from under
// [Parser.Next] is the caller's problem.
func (p *Parser) Lexer() *Lexer { return &p.l }

func (p *Parser) maxDepth() int {
	if p.MaxDepth > 0 {
		return p.MaxDepth
	}
	return defaultMaxDepth
}

func (p *Parser) maxDelims() int {
	if p.MaxInlineDelims > 0 {
		return p.MaxInlineDelims
	}
	return defaultMaxDelims
}

// step runs the machine far enough to queue at least one node, and reports
// false once the document is finished.
func (p *Parser) step() bool {
	switch {
	case p.err != nil:
		return false
	case !p.started:
		p.started = true
		return p.push(frame{node: Node{Kind: KindDocument}})
	case p.done:
		return false
	case p.inInline:
		// The leaf's inlines are handed out one at a time; the block loop only
		// resumes once they run out and the leaf closes.
		if n, ok := p.inlineNext(); ok {
			p.queue(n)
			return true
		}
		p.inInline = false
		p.contentEnd = p.leafEnd
		p.closeTo(len(p.stack)-1, p.leafEnd)
		return true
	case p.l.IsDone():
		p.done = true
		p.closeTo(0, p.contentEnd)
		return true
	}
	p.parseLine()
	return true
}

// --- block structure ---
//
// Three phases per line: consume the prefixes of the blocks that continue, open
// whatever the line begins, attach the rest as content.

// parseLine consumes exactly one source line.
func (p *Parser) parseLine() {
	matched := p.matchOpen()
	top := len(p.stack) - 1
	inCode := matched == top && p.stack[top].node.Kind == KindCodeBlock

	// A fenced block owns its lines whole: no indent scan, no marker probes.
	if inCode && p.stack[top].fence > 0 {
		p.fencedLine(top)
		return
	}

	lineStart := p.l.Pos()
	base := int32(p.l.Col())
	codeStart, blank := p.scanIndent(base)

	if inCode { // Indented code, which ends as soon as the indent does.
		if p.indentedLine(top, lineStart, codeStart, blank) {
			return
		}
		p.closeTo(top, p.contentEnd)
	}
	if blank {
		p.blankLine(matched)
		return
	}

	// Containers can nest arbitrarily on one line: "> - a" opens three.
	for {
		if p.l.PeekRune() == '>' {
			if !p.openQuote(matched) {
				return
			}
			matched = len(p.stack)
			lineStart = p.l.Pos()
			base = int32(p.l.Col())
			if codeStart, blank = p.scanIndent(base); blank {
				p.blankLine(matched)
				return
			} else if codeStart >= 0 {
				p.openIndentedCode(matched, lineStart, codeStart, base)
				return
			}
			continue
		}
		// A run of "-", "_" or "*" alone on the line is a break, not a bullet.
		if end, ok := p.atThematicBreak(); ok {
			p.closeForSibling(matched)
			p.queue(Node{Kind: KindThematicBreak, Start: lineStart, End: end})
			p.contentEnd = end
			p.l.SkipLine()
			return
		}
		if marker, ch, ok := p.atBullet(); ok {
			if !p.openItem(matched, marker, int32(ch), 0) {
				return
			}
			matched = len(p.stack)
			continue
		}
		if marker, num, ok := p.atOrdered(); ok {
			if !p.openItem(matched, marker, num, FlagOrdered) {
				return
			}
			matched = len(p.stack)
			continue
		}
		break
	}

	if isLineEndRune(p.l.PeekRune()) {
		// A marker ate the whole line: "-" alone opens an empty item, not an
		// item holding an empty paragraph.
		p.l.SkipLine()
		return
	}

	// Indent inside a container opened on this line is code, not content. A
	// paragraph can never be open here: its own scan consumed every line it
	// continued onto, which is also where "indented code may not interrupt a
	// paragraph" is decided.
	if codeStart >= 0 {
		p.openIndentedCode(matched, lineStart, codeStart, base)
		return
	}
	contentStart := p.l.Pos()
	if marker, ok := p.atATX(); ok {
		p.headingLine(matched, marker)
		return
	}
	if f, info, trim, ok := p.atFence(); ok {
		p.closeForSibling(matched)
		f.col = base
		if !p.push(f) {
			return
		}
		if info.Kind != KindUndefined {
			p.queue(info)
		}
		p.contentEnd = trim
		return
	}
	p.closeForSibling(matched)
	if !p.push(frame{node: Node{Kind: KindParagraph, Start: contentStart, End: contentStart}}) {
		return
	}
	p.beginInline(contentStart, -1, -1)
}

// continuesLeaf reports whether the line at the cursor adds to the open leaf,
// consuming its prefix and indent when it does. Every probe restores the cursor
// on a false result, and the caller undoes the rest by rewinding to the line
// start, so a line that ends the block is untouched when the block loop sees it.
//
// Indent is deliberately not a block start here: indented code may not interrupt
// a paragraph, so a deeply indented line simply continues one.
func (p *Parser) continuesLeaf(depth int) bool {
	if p.l.IsDone() || p.matchOpen() != depth {
		return false
	}
	if _, blank := p.scanIndent(int32(p.l.Col())); blank {
		return false
	}
	if p.l.PeekRune() == '>' {
		return false
	}
	if _, ok := p.atThematicBreak(); ok {
		return false
	}
	if _, _, ok := p.atBullet(); ok {
		return false
	}
	if _, _, ok := p.atOrdered(); ok {
		return false
	}
	if _, ok := p.atATX(); ok {
		return false
	}
	if _, _, _, ok := p.atFence(); ok {
		return false
	}
	return true
}

// matchOpen consumes the line prefix of every open block that continues on this
// line and returns the stack depth reached. Every frame past it must close,
// unless the line turns out to continue a paragraph.
func (p *Parser) matchOpen() int {
	i := 1 // Frame 0 is the document, which every line continues.
	for ; i < len(p.stack); i++ {
		switch f := &p.stack[i]; f.node.Kind {
		case KindBlockQuote:
			if !p.matchQuote() {
				return i
			}
		case KindList:
			// A list has no prefix of its own; its items carry one.
		case KindItem:
			if !p.matchItem(f.col) {
				return i
			}
		default:
			// A leaf, whose continuation depends on what the rest of the line
			// turns out to be rather than on a prefix.
			return i
		}
	}
	return i
}

// matchQuote consumes "> " when the line carries it. Up to three spaces may
// precede the marker, and one space after it belongs to the marker.
func (p *Parser) matchQuote() bool {
	i := 0
	for i < maxBlockIndent && p.l.PeekAt(i) == ' ' {
		i++
	}
	if p.l.PeekAt(i) != '>' {
		return false
	}
	for range i + 1 {
		p.l.advance()
	}
	if p.l.PeekRune() == ' ' {
		p.l.advance()
	}
	return true
}

// matchItem consumes the indent that continues an item whose content starts at
// col. A blank line does not interrupt an item; it only makes its list loose.
func (p *Parser) matchItem(col int32) bool {
	p.skipIndent(col)
	return int32(p.l.Col()) >= col || p.atLineEnd()
}

// blankLine ends the open leaf and marks the enclosing lists, whose looseness
// the next item decides. Containers themselves survive a blank line — those
// that do not are the ones whose prefix the line already failed to carry.
func (p *Parser) blankLine(matched int) {
	p.closeTo(matched, p.contentEnd)
	p.markBlank()
	p.l.SkipLine()
}

// markBlank records the blank line against every enclosing list, whose
// looseness the next item decides.
func (p *Parser) markBlank() {
	for i := range p.stack {
		switch p.stack[i].node.Kind {
		case KindList, KindItem:
			p.stack[i].blank = true
		}
	}
}

// --- containers ---

func (p *Parser) openQuote(matched int) bool {
	// A quote is the list's sibling, not its content: only an item indented to
	// the item's content column would have matched as inside it.
	p.closeForSibling(matched)
	start := p.l.Pos()
	p.l.advance() // '>'
	if p.l.PeekRune() == ' ' {
		p.l.advance()
	}
	return p.push(frame{node: Node{Kind: KindBlockQuote, Start: start, End: p.l.Pos()}})
}

// openItem opens an item, reusing the enclosing list when the marker matches it
// and starting a fresh one otherwise. attr is the bullet byte, or the start
// number when ordered.
func (p *Parser) openItem(matched int, marker Node, attr int32, flags Flags) bool {
	p.closeTo(matched, p.contentEnd)
	top := len(p.stack) - 1
	f := &p.stack[top]
	same := f.node.Kind == KindList && f.node.Flags&FlagOrdered == flags&FlagOrdered &&
		(flags&FlagOrdered != 0 || f.node.Attr == attr)
	if !same {
		// A different marker starts a different list.
		for len(p.stack) > 1 && p.stack[len(p.stack)-1].node.Kind == KindList {
			p.queue(p.pop(p.contentEnd))
		}
		list := marker
		list.Kind = KindList
		list.Flags = flags
		list.Attr = attr
		if !p.push(frame{node: list}) {
			return false
		}
	}
	f = &p.stack[len(p.stack)-1]
	if f.blank {
		f.loose = true
	}
	f.blank = false
	item := marker
	item.Kind = KindItem
	item.Flags = 0
	item.Attr = f.count
	f.count++
	return p.push(frame{node: item, col: int32(p.l.Col())})
}

// --- leaves ---

// headingLine opens an ATX heading over its one line. Its extent is known
// before its content is scanned, so the scan is bounded to the text and the
// closing sequence never reaches the inline layer.
func (p *Parser) headingLine(matched int, marker Node) {
	p.closeForSibling(matched)
	if !p.push(frame{node: marker}) {
		return
	}
	m := p.mark()
	textEnd, lineEnd := p.readHeading(marker.End)
	if lineEnd < marker.End {
		lineEnd = marker.End
	}
	p.reset(m)
	p.beginInline(marker.End, textEnd, lineEnd)
	// The scan stopped at the content's end; the rest of the line is the
	// closing sequence, and pass two needs no cursor.
	p.l.SkipLine()
}

func (p *Parser) openIndentedCode(matched int, lineStart, codeStart Pos, base int32) {
	p.closeForSibling(matched)
	n := Node{Kind: KindCodeBlock, Start: lineStart, End: codeStart}
	if !p.push(frame{node: n, col: base}) {
		return
	}
	p.rawLine(codeStart)
}

// indentedLine handles one line of an open indented code block, reporting
// whether the block survives it.
//
// A blank line is content when more indented lines follow and trailing padding
// when they do not, and nothing on the line itself says which. So a blank run is
// consumed but held: if the block then continues, the cursor goes back to where
// the run began and re-reads it as content; if the block ends, the run is
// dropped, having never been emitted. Holding costs two offsets rather than a
// node per line, so an arbitrarily long run still fits in the frame, and the
// re-read is over bytes the window is likely to still hold.
func (p *Parser) indentedLine(i int, lineStart, codeStart Pos, blank bool) bool {
	f := &p.stack[i]
	if f.replay && lineStart >= f.blankEnd {
		f.replay = false // The held run is behind the cursor again.
	}
	switch {
	case blank && f.replay:
		// Whatever is left after the indent, which for a blank line is its
		// ending alone.
		start := codeStart
		if start < 0 {
			start = p.l.Pos()
		}
		p.rawLine(start)
		return true

	case blank:
		if !f.holding {
			f.blankAt, f.holding = lineStart, true
		}
		p.markBlank() // Looseness still counts these, emitted or not.
		p.l.SkipLine()
		f.blankEnd = p.l.Pos()
		return true

	case codeStart >= 0 && f.holding:
		// Indented content followed, so the run was interior after all. A blank
		// run starts at a line start, where the column is 0, so Seek restores
		// the cursor exactly.
		f.holding, f.replay = false, true
		p.err = p.l.Seek(f.blankAt)
		return true

	case codeStart >= 0:
		p.rawLine(codeStart)
		return true
	}
	// The block ends here, so a held run was trailing padding. Dropping it needs
	// nothing done: it was never emitted, and never advanced contentEnd, so the
	// close still lands on the last real byte.
	return false
}

// fencedLine handles one line of an open fenced block: either the fence that
// closes it or a verbatim content line.
func (p *Parser) fencedLine(i int) {
	f := &p.stack[i]
	if end, ok := p.atCloseFence(f.ch, f.fence); ok {
		p.contentEnd = end
		p.closeTo(i, end)
		p.l.SkipLine()
		return
	}
	p.rawLine(p.l.Pos())
}

// rawLine emits one line of verbatim content, line ending included.
func (p *Parser) rawLine(start Pos) {
	_, end, next := p.readLine()
	if next > start {
		p.queue(Node{Kind: KindRaw, Start: start, End: next})
	}
	if end > p.contentEnd {
		p.contentEnd = end
	}
}

// --- line-start probes ---
//
// Each reports whether the construct starts here and, on a false result, leaves
// the cursor where it found it. Probing consumes and rewinds rather than peeks:
// a heading needs seven runes of lookahead and a thematic break needs the whole
// line, both past the lexer's peek buffer, and a rewind inside the window costs
// no read.

// atThematicBreak matches three or more of "-", "_" or "*" alone on the line.
func (p *Parser) atThematicBreak() (end Pos, ok bool) {
	ch := p.l.PeekRune()
	if ch != '-' && ch != '_' && ch != '*' {
		return 0, false
	}
	m := p.mark()
	n := 0
	for {
		switch r := p.l.PeekRune(); {
		case r == ch:
			n++
			p.l.advance()
			end = p.l.Pos()
		case isIndentRune(r):
			p.l.advance()
		case isLineEndRune(r):
			if n >= 3 {
				return end, true
			}
			p.reset(m)
			return 0, false
		default:
			p.reset(m)
			return 0, false
		}
	}
}

// atATX matches one to six hashes followed by a space or the line's end.
func (p *Parser) atATX() (marker Node, ok bool) {
	if p.l.PeekRune() != '#' {
		return Node{}, false
	}
	m := p.mark()
	start := p.l.Pos()
	n := int32(0)
	for p.l.PeekRune() == '#' {
		n++
		p.l.advance()
	}
	r := p.l.PeekRune()
	if n > maxATXLevel || !(isIndentRune(r) || isLineEndRune(r)) {
		p.reset(m)
		return Node{}, false
	}
	p.skipAll()
	return Node{Kind: KindHeading, Attr: n, Start: start, End: p.l.Pos()}, true
}

// atBullet matches "-", "*" or "+" followed by a space or the line's end. The
// caller must rule out a thematic break first.
func (p *Parser) atBullet() (marker Node, ch byte, ok bool) {
	r := p.l.PeekRune()
	if r != '-' && r != '*' && r != '+' {
		return Node{}, 0, false
	}
	if nx := p.l.PeekAt(1); !(isIndentRune(nx) || isLineEndRune(nx)) {
		return Node{}, 0, false
	}
	start := p.l.Pos()
	p.l.advance()
	p.skipAll()
	return Node{Start: start, End: p.l.Pos()}, byte(r), true
}

// atOrdered matches digits followed by "." or ")" and a space or the line's end.
func (p *Parser) atOrdered() (marker Node, start int32, ok bool) {
	if !isDigitRune(p.l.PeekRune()) {
		return Node{}, 0, false
	}
	m := p.mark()
	begin := p.l.Pos()
	num, n := int32(0), 0
	for isDigitRune(p.l.PeekRune()) && n < 9 {
		num = num*10 + (p.l.PeekRune() - '0')
		n++
		p.l.advance()
	}
	if r := p.l.PeekRune(); r != '.' && r != ')' {
		p.reset(m)
		return Node{}, 0, false
	}
	p.l.advance()
	if nx := p.l.PeekRune(); !(isIndentRune(nx) || isLineEndRune(nx)) {
		p.reset(m)
		return Node{}, 0, false
	}
	p.skipAll()
	return Node{Start: begin, End: p.l.Pos()}, num, true
}

// atFence matches an opening code fence, consuming through the line ending: the
// open node's span covers the whole opening line, so the body starts where it
// ends. trim is that line's last content byte, where the block closes if the
// fence is never terminated.
func (p *Parser) atFence() (f frame, info Node, trim Pos, ok bool) {
	ch := p.l.PeekRune()
	if ch != '`' && ch != '~' {
		return frame{}, Node{}, 0, false
	}
	m := p.mark()
	start := p.l.Pos()
	n := int32(0)
	for p.l.PeekRune() == ch {
		n++
		p.l.advance()
	}
	if n < 3 {
		p.reset(m)
		return frame{}, Node{}, 0, false
	}
	p.skipAll()
	infoStart := p.l.Pos()
	infoEnd := infoStart
	for {
		r := p.l.PeekRune()
		if isLineEndRune(r) {
			break
		}
		if ch == '`' && r == '`' {
			// A backtick fence's info string may hold no backtick, or "`a`" at
			// the start of a line would open a block instead of a code span.
			p.reset(m)
			return frame{}, Node{}, 0, false
		}
		p.l.advance()
		if !isIndentRune(r) {
			infoEnd = p.l.Pos()
		}
	}
	trim = infoEnd
	if trim == infoStart {
		trim = start + Pos(n)
	}
	node := Node{Kind: KindCodeBlock, Flags: FlagFenced, Attr: n, Start: start, End: p.l.SkipLine()}
	if infoEnd > infoStart {
		info = Node{Kind: KindInfo, Start: infoStart, End: infoEnd}
	}
	return frame{node: node, fence: n, ch: byte(ch)}, info, trim, true
}

// atCloseFence matches a fence of at least n of ch alone on the line.
func (p *Parser) atCloseFence(ch byte, n int32) (end Pos, ok bool) {
	m := p.mark()
	for i := 0; i < maxBlockIndent && p.l.PeekRune() == ' '; i++ {
		p.l.advance()
	}
	if p.l.PeekRune() != rune(ch) {
		p.reset(m)
		return 0, false
	}
	cnt := int32(0)
	for p.l.PeekRune() == rune(ch) {
		cnt++
		p.l.advance()
	}
	end = p.l.Pos()
	p.skipAll()
	if cnt < n || !isLineEndRune(p.l.PeekRune()) {
		p.reset(m)
		return 0, false
	}
	return end, true
}

// --- cursor helpers ---

func (p *Parser) mark() mark      { return mark{p.l.pos, p.l.col, p.l.prev} }
func (p *Parser) reset(m mark)    { p.l.rewind(m.pos, m.col, m.prev) }
func (p *Parser) atLineEnd() bool { p.skipAll(); return isLineEndRune(p.l.PeekRune()) }

// skipAll consumes every space and tab at the cursor.
func (p *Parser) skipAll() {
	for isIndentRune(p.l.PeekRune()) {
		p.l.advance()
	}
}

// skipIndent consumes spaces and tabs while the cursor stays left of limit.
func (p *Parser) skipIndent(limit int32) {
	for int32(p.l.Col()) < limit && isIndentRune(p.l.PeekRune()) {
		p.l.advance()
	}
}

// scanIndent consumes the line's leading whitespace. codeStart is where an
// indented code block's content would begin, or -1 when the line does not reach
// codeIndent columns past base.
func (p *Parser) scanIndent(base int32) (codeStart Pos, blank bool) {
	p.skipIndent(base + codeIndent)
	codeStart = p.l.Pos()
	if int32(p.l.Col()) < base+codeIndent {
		codeStart = -1
	}
	p.skipAll() // Whatever is left is content, unless the line is blank.
	return codeStart, isLineEndRune(p.l.PeekRune())
}

// readLine consumes the rest of the line, reporting the offset past its last
// non-whitespace byte, the offset past its last byte before the line ending,
// and the offset the next line starts at.
func (p *Parser) readLine() (trim, end, next Pos) {
	trim = p.l.Pos()
	for {
		r := p.l.PeekRune()
		if isLineEndRune(r) {
			break
		}
		p.l.advance()
		if !isIndentRune(r) {
			trim = p.l.Pos()
		}
	}
	end = p.l.Pos()
	return trim, end, p.l.SkipLine()
}

// readHeading consumes an ATX heading's content, reporting where its text ends
// and where the line does. They differ by a closing sequence: a trailing run of
// hashes preceded by whitespace, which is part of the heading but not its text.
func (p *Parser) readHeading(from Pos) (textEnd, lineEnd Pos) {
	textEnd, lineEnd = from, from
	hashEnd := Pos(-1) // Text end recorded when the trailing hash run began.
	prevSpace := true  // A closing sequence must be preceded by whitespace.
	for {
		r := p.l.PeekRune()
		if isLineEndRune(r) {
			break
		}
		switch {
		case r == '#':
			if hashEnd < 0 && prevSpace {
				hashEnd = textEnd
			}
			prevSpace = false
		case isIndentRune(r):
			prevSpace = true
		default:
			hashEnd = -1
			prevSpace = false
		}
		p.l.advance()
		if !isIndentRune(r) {
			lineEnd = p.l.Pos()
			textEnd = lineEnd
		}
	}
	if hashEnd >= 0 {
		textEnd = hashEnd
	}
	return textEnd, lineEnd
}

func isIndentRune(r rune) bool  { return r == ' ' || r == '\t' }
func isLineEndRune(r rune) bool { return r == 0 || r == '\n' || r == '\r' }

// --- stack ---

// push opens a block, queueing its open node. Fails past MaxDepth.
func (p *Parser) push(f frame) bool {
	if len(p.stack) >= p.maxDepth() {
		p.err = errDepthExceeded
		return false
	}
	f.node.Flags |= FlagOpen
	p.stack = append(p.stack, f)
	p.queue(f.node)
	return true
}

// pop closes the innermost block. The close carries the same kind, attribute
// and kind-specific flags as the open, so a consumer sees a list's ordering on
// both events; only the open/close bits and FlagLoose differ.
func (p *Parser) pop(end Pos) Node {
	f := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	n := f.node
	n.Flags = n.Flags&^FlagOpen | FlagClose
	if f.loose {
		n.Flags |= FlagLoose
	}
	// The document is not a construct with a marker but the input itself, so it
	// closes over every byte, trailing blank lines included.
	if n.Kind == KindDocument {
		end = p.l.Pos()
	}
	n.End = max(end, n.End)
	return n
}

// closeTo pops down to depth, queueing each close node.
func (p *Parser) closeTo(depth int, end Pos) {
	for len(p.stack) > depth {
		p.queue(p.pop(end))
	}
}

// closeForSibling pops what the line did not continue, plus any list left
// innermost. A list holds items and nothing else, so any other block starting
// here is its sibling and ends it.
func (p *Parser) closeForSibling(matched int) {
	p.closeTo(matched, p.contentEnd)
	for len(p.stack) > 1 && p.stack[len(p.stack)-1].node.Kind == KindList {
		p.queue(p.pop(p.contentEnd))
	}
}

func (p *Parser) queue(n Node) { p.pending = append(p.pending, n) }

// errAt latches a *SyntaxError; the first error wins.
func (p *Parser) errAt(pos Pos, msg string) {
	if p.err == nil {
		p.err = &SyntaxError{Source: p.l.src, Off: pos, Msg: msg}
	}
}
