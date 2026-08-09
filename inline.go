package ohimark

// Inline content is resolved in two passes over one leaf block. Pass one scans
// the block's lines forward, recording delimiter runs, escapes, entities,
// autolinks and breaks. Pass two walks that record emitting nodes; the bytes
// between records become KindText.
//
// Pass one is eager and pass two touches no bytes at all — it reads only the
// recorded spans. So the lexer is free the moment the scan returns, which is
// what lets a heading skip to its line ending while its inlines are still being
// handed out one Next at a time.
//
// Nothing is re-read: a match is known from positions alone, so "*foo*" needs
// the paragraph scanned to its end before EmphOpen can be emitted, but never
// needs its bytes twice. Breaks are recorded too, which lets pass two resume at
// a break's end rather than re-deriving the next line's stripped prefix.

// role is what a record stands for, which its token alone does not say: a
// TokParenL record is a link's tail, not a delimiter.
type role uint8

const (
	roleRun          role = iota // Emphasis or backtick run; length carries meaning.
	roleLeaf                     // Escape, entity or autolink: one node, as recorded.
	roleBreak                    // Line ending between two content lines.
	roleOpenBracket              // "[" or "!["
	roleCloseBracket             // "]"
	roleTail                     // "(dest \"title\")" trailing a matched "]".
	roleDest
	roleTitle
)

// delim is one record from pass one.
//
// A run is consumed from both ends: bytes leave an opener from its end and a
// closer from its start, so openUsed and closeUsed together say which of its
// bytes are still literal text. The pair lists hold the nodes anchored here, in
// the byte order they occupy — closes first, then the leftover text, then opens.
type delim struct {
	start, end Pos

	openHead, openTail   int32 // Pairs this record opens, in position order.
	closeHead, closeTail int32 // Pairs it closes, likewise.
	openUsed, closeUsed  int32

	tok   Token // The delimiter character, or what kind of leaf this is.
	role  role
	flags delimFlags
}

func (d *delim) length() int32 { return int32(d.end - d.start) }

// avail reports how many of the run's bytes no match has taken yet.
func (d *delim) avail() int32 { return d.length() - d.openUsed - d.closeUsed }

// idle reports whether no match touched this record, so its bytes are literal
// and it can be skipped outright — letting the surrounding text stay one node.
func (d *delim) idle() bool {
	return d.openHead < 0 && d.closeHead < 0 && d.openUsed == 0 && d.closeUsed == 0
}

type delimFlags uint8

const (
	dCanOpen delimFlags = 1 << iota
	dCanClose
	dActive // Not yet consumed by a match.
	dHard   // Break: the line ended with two spaces or a backslash.
	dImage  // Bracket preceded by '!'.
)

// pair is one resolved construct: the bytes its open node covers and the bytes
// its close node ends on.
//
// A pair sits in two lists at once — its opener's and its closer's — so it
// needs a link for each. One shared link would make the two lists overwrite
// each other into a cycle.
type pair struct {
	openStart, openEnd  Pos
	closeEnd            Pos // Where the close node ends; it starts at openStart.
	nextOpen, nextClose int32
	attr                int32
	kind                Kind
}

// Emission phases within one record.
const (
	phaseGap uint8 = iota
	phaseClose
	phaseLeft
	phaseOpen
)

// beginInline scans the leaf block's content and arms the emission pass. stop
// bounds the content when the caller already knows where it ends, as a heading
// does; a negative stop means the scan discovers the end itself. leafEnd is
// where the block's close node lands, negative to mean the content's end.
func (p *Parser) beginInline(start, stop, leafEnd Pos) {
	p.delims = p.delims[:0]
	p.pairs = p.pairs[:0]
	p.inlineStop = stop
	p.inlineEnd = start
	p.emitIdx, p.emitPair, p.emitPhase = 0, -1, phaseGap
	p.emitPos = start
	p.inlineScan(len(p.stack) - 1)
	if leafEnd >= 0 {
		p.leafEnd = leafEnd
	} else {
		p.leafEnd = p.inlineEnd
	}
	p.inlineResolve()
	p.inInline = true
}

// inlineScan is pass one. depth is the leaf's own frame index, which every
// continuation line must still reach.
func (p *Parser) inlineScan(depth int) {
	last := p.l.Pos() // End of the last token that was not trailing space.
	prevTok := TokUndefined
	var prevStart, prevEnd Pos
	for {
		if p.inlineStop >= 0 && p.l.Pos() >= p.inlineStop {
			break
		}
		tok, start, end := p.l.Next()
		if tok == TokEOF || tok == TokIllegal {
			break
		}
		switch tok {
		case TokNewline:
			if !p.scanBreak(depth, last, prevTok, prevStart, prevEnd) {
				return
			}
			last = p.l.Pos()
			prevTok, prevStart, prevEnd = TokUndefined, 0, 0
			continue

		case TokEscape, TokEntity:
			p.pushDelim(delim{role: roleLeaf, tok: tok, start: start, end: end})

		case TokTick, TokStar, TokUnderscore:
			d := delim{role: roleRun, tok: tok, start: start, end: end, flags: dActive}
			if tok != TokTick {
				p.flanking(&d)
			}
			p.pushDelim(d)

		case TokBracketL:
			d := delim{role: roleOpenBracket, tok: tok, start: start, end: end, flags: dActive}
			if prevTok == TokBang && prevEnd == start {
				// "![" is one marker, so the image's open node covers both.
				d.flags |= dImage
				d.start = prevStart
			}
			p.pushDelim(d)

		case TokBracketR:
			p.pushDelim(delim{role: roleCloseBracket, tok: tok, start: start, end: end, flags: dActive})
			p.scanLinkTail()
			// The tail is content too, so the block reaches past the "]".
			last = p.l.Pos()
			prevTok, prevStart, prevEnd = tok, start, last
			continue

		case TokLT:
			if n, ok := p.scanAutolink(start); ok {
				p.pushDelim(delim{role: roleLeaf, tok: TokLT, start: n.Start, end: n.End})
				last = n.End
				prevTok, prevStart, prevEnd = tok, n.Start, n.End
				continue
			}
		}
		if tok != TokSpace {
			last = end
		}
		prevTok, prevStart, prevEnd = tok, start, end
	}
	p.inlineEnd = last
}

// scanBreak records the line ending and reports whether the block goes on. The
// break's span reaches the next line's first content byte, so the container
// prefix it steps over belongs to no node.
func (p *Parser) scanBreak(depth int, last Pos, prevTok Token, prevStart, prevEnd Pos) bool {
	start, flags := last, delimFlags(0)
	switch {
	case prevTok == TokSpace && prevEnd-prevStart >= 2:
		start, flags = prevStart, dHard
	case prevTok == TokSpace:
		start = prevStart // One trailing space is not a break, only not text.
	case prevTok == TokWord && p.l.PrevRune() == '\\':
		// An unescaped trailing backslash. An escaped one is a TokEscape, so it
		// never reaches here.
		start, flags = prevEnd-1, dHard
	}
	m := p.mark()
	if !p.continuesLeaf(depth) {
		p.reset(m)
		p.inlineEnd = last
		return false
	}
	p.pushDelim(delim{role: roleBreak, tok: TokNewline, start: start, end: p.l.Pos(), flags: flags})
	return true
}

// flanking applies CommonMark's left- and right-flanking rules to the runes
// abutting the run. A zero rune is a file edge, which both rules read as
// whitespace.
func (p *Parser) flanking(d *delim) {
	before, after := p.l.PrevRune(), p.l.PeekRune()
	bSpace, aSpace := before == 0 || isSpaceRune(before), after == 0 || isSpaceRune(after)
	bPunct, aPunct := isPunctRune(before), isPunctRune(after)
	left := !aSpace && (!aPunct || bSpace || bPunct)
	right := !bSpace && (!bPunct || aSpace || aPunct)
	open, close := left, right
	if d.tok == TokUnderscore {
		// "_" does not emphasize inside a word, so snake_case survives.
		open = left && (!right || bPunct)
		close = right && (!left || aPunct)
	}
	if open {
		d.flags |= dCanOpen
	}
	if close {
		d.flags |= dCanClose
	}
}

// scanLinkTail reads "(dest \"title\")" straight after a "]", recording it as
// records of its own. Reading it here rather than at resolution time is what
// keeps the whole layer single-read: the cursor is already standing on it.
func (p *Parser) scanLinkTail() {
	if p.l.PeekRune() != '(' {
		return
	}
	m := p.mark()
	tailStart := p.l.Pos()
	p.l.advance()
	p.skipInlineSpace()
	destStart := p.l.Pos()
	depth := 0
	for {
		r := p.l.PeekRune()
		if r == 0 || isLineEndRune(r) || isSpaceRune(r) {
			break
		}
		if r == '\\' {
			p.l.advance()
			if p.l.PeekRune() != 0 {
				p.l.advance()
			}
			continue
		}
		if r == '(' {
			depth++
		} else if r == ')' {
			if depth == 0 {
				break
			}
			depth--
		}
		p.l.advance()
	}
	destEnd := p.l.Pos()
	titleStart, titleEnd := Pos(0), Pos(0)
	p.skipInlineSpace()
	if q := p.l.PeekRune(); q == '"' || q == '\'' {
		p.l.advance()
		titleStart = p.l.Pos()
		for {
			r := p.l.PeekRune()
			if r == 0 || r == q {
				break
			}
			if r == '\\' {
				p.l.advance()
			}
			p.l.advance()
		}
		if p.l.PeekRune() != q {
			p.reset(m)
			return
		}
		titleEnd = p.l.Pos()
		p.l.advance()
		p.skipInlineSpace()
	}
	if p.l.PeekRune() != ')' {
		p.reset(m)
		return
	}
	p.l.advance()
	p.pushDelim(delim{role: roleTail, tok: TokParenL, start: tailStart, end: p.l.Pos()})
	if destEnd > destStart {
		p.pushDelim(delim{role: roleDest, start: destStart, end: destEnd})
	}
	if titleEnd > titleStart {
		p.pushDelim(delim{role: roleTitle, start: titleStart, end: titleEnd})
	}
}

// skipInlineSpace steps over spaces, tabs and one line ending, which a link
// tail may be broken across.
func (p *Parser) skipInlineSpace() {
	for isIndentRune(p.l.PeekRune()) {
		p.l.advance()
	}
}

// scanAutolink reads "<scheme:rest>" or "<user@host>" as one node.
func (p *Parser) scanAutolink(start Pos) (n Node, ok bool) {
	m := p.mark()
	scheme, at, chars := 0, false, 0
	for i := 0; ; i++ {
		r := p.l.PeekRune()
		switch {
		case r == '>':
			p.l.advance()
			if chars == 0 || (scheme == 0 && !at) {
				p.reset(m)
				return Node{}, false
			}
			return Node{Kind: KindAutolink, Start: start, End: p.l.Pos()}, true
		case r == 0 || isLineEndRune(r) || isSpaceRune(r) || r == '<':
			p.reset(m)
			return Node{}, false
		case r == ':' && scheme == 0 && chars > 0:
			scheme = i
		case r == '@':
			at = true
		}
		p.l.advance()
		chars++
	}
}

// pushDelim appends to the memo, reporting false once the cap is hit. Passing
// the cap is a degradation and not an error: the records already taken still
// resolve, and the rest of the block reads as flat text.
func (p *Parser) pushDelim(d delim) bool {
	if len(p.delims) >= p.maxDelims() {
		return false
	}
	d.openHead, d.openTail = -1, -1
	d.closeHead, d.closeTail = -1, -1
	p.delims = append(p.delims, d)
	return true
}

// addPair records a resolved construct. Emphasis prepends at its opener, since
// each later match takes bytes further left; everything else appends, which is
// the order those nodes are read in.
func (p *Parser) addPair(oi, ci int, pr pair, prepend bool) bool {
	if len(p.pairs) >= p.maxDelims() {
		return false
	}
	pr.nextOpen, pr.nextClose = -1, -1
	idx := int32(len(p.pairs))
	p.pairs = append(p.pairs, pr)
	if oi >= 0 {
		d := &p.delims[oi]
		switch {
		case d.openHead < 0:
			d.openHead, d.openTail = idx, idx
		case prepend:
			p.pairs[idx].nextOpen = d.openHead
			d.openHead = idx
		default:
			p.pairs[d.openTail].nextOpen = idx
			d.openTail = idx
		}
	}
	if ci >= 0 {
		d := &p.delims[ci]
		if d.closeHead < 0 {
			d.closeHead, d.closeTail = idx, idx
		} else {
			p.pairs[d.closeTail].nextClose = idx
			d.closeTail = idx
		}
	}
	return true
}

// inlineResolve pairs records in spec order: backtick runs first, since a code
// span swallows everything between its ticks; then brackets, since a link's text
// may not contain another link; then emphasis over what is left.
func (p *Parser) inlineResolve() {
	p.resolveCode()
	p.resolveLinks()
	p.resolveEmph(0, len(p.delims))
}

// resolveCode pairs backtick runs of equal length and silences everything
// between them. Breaks stay live, so a code span crossing lines still reports
// where its lines join instead of swallowing a container prefix into its text.
func (p *Parser) resolveCode() {
	for i := range p.delims {
		o := &p.delims[i]
		if o.role != roleRun || o.tok != TokTick || o.flags&dActive == 0 {
			continue
		}
		for j := i + 1; j < len(p.delims); j++ {
			c := &p.delims[j]
			if c.role != roleRun || c.tok != TokTick || c.length() != o.length() {
				continue
			}
			if !p.addPair(i, j, pair{
				kind: KindCodeSpan, attr: '`',
				openStart: o.start, openEnd: o.end, closeEnd: c.end,
			}, false) {
				return
			}
			o.openUsed, c.closeUsed = o.length(), c.length()
			o.flags &^= dActive
			c.flags &^= dActive
			for k := i + 1; k < j; k++ {
				if d := &p.delims[k]; d.role != roleBreak {
					d.flags &^= dActive | dCanOpen | dCanClose
					d.role = roleRun
					d.openUsed, d.closeUsed = 0, 0
				}
			}
			break
		}
	}
}

// resolveLinks pairs brackets that carry a tail, emitting the destination and
// title behind the open node so a renderer has them before the link's text.
func (p *Parser) resolveLinks() {
	for i := 0; i < len(p.delims); i++ {
		c := &p.delims[i]
		if c.role != roleCloseBracket || c.flags&dActive == 0 {
			continue
		}
		oi := -1
		for k := i - 1; k >= 0; k-- {
			if d := &p.delims[k]; d.role == roleOpenBracket && d.flags&dActive != 0 {
				oi = k
				break
			}
		}
		if oi < 0 {
			continue
		}
		if i+1 >= len(p.delims) || p.delims[i+1].role != roleTail {
			p.delims[oi].flags &^= dActive // No tail, so the bracket is literal.
			continue
		}
		o := &p.delims[oi]
		kind := KindLink
		if o.flags&dImage != 0 {
			kind = KindImage
		}
		if !p.addPair(oi, i, pair{
			kind:      kind,
			openStart: o.start, openEnd: o.end,
			closeEnd: p.delims[i+1].end,
		}, false) {
			return
		}
		for k := i + 2; k < len(p.delims); k++ {
			ak := KindDest
			switch p.delims[k].role {
			case roleDest:
			case roleTitle:
				ak = KindTitle
			default:
				k = len(p.delims)
				continue
			}
			p.addPair(oi, -1, pair{
				kind:      ak,
				openStart: p.delims[k].start, openEnd: p.delims[k].end,
			}, false)
		}
		o.openUsed, c.closeUsed = o.length(), c.length()
		o.flags &^= dActive
		c.flags &^= dActive
		if kind == KindLink {
			// A link's text may not hold another link.
			for k := 0; k < oi; k++ {
				if p.delims[k].role == roleOpenBracket {
					p.delims[k].flags &^= dActive
				}
			}
		}
		p.resolveEmph(oi+1, i)
	}
}

// resolveEmph runs CommonMark's delimiter-run rules over one index range.
func (p *Parser) resolveEmph(lo, hi int) {
	for ci := lo; ci < hi; ci++ {
		c := &p.delims[ci]
		if c.role != roleRun || c.tok == TokTick || c.flags&dCanClose == 0 {
			continue
		}
		for c.avail() > 0 {
			oi := p.findOpener(lo, ci)
			if oi < 0 {
				break
			}
			o := &p.delims[oi]
			n, kind := int32(1), KindEmph
			if o.avail() >= 2 && c.avail() >= 2 {
				n, kind = 2, KindStrong
			}
			oEnd := o.end - Pos(o.openUsed)
			cStart := c.start + Pos(c.closeUsed)
			if !p.addPair(oi, ci, pair{
				kind: kind, attr: int32(byteOf(c.tok)),
				openStart: oEnd - Pos(n), openEnd: oEnd,
				closeEnd: cStart + Pos(n),
			}, true) {
				return
			}
			o.openUsed += n
			c.closeUsed += n
			// Runs bridged by the match can no longer reach past it.
			for k := oi + 1; k < ci; k++ {
				p.delims[k].flags &^= dCanOpen | dCanClose
			}
		}
	}
}

// findOpener walks back for the nearest run that can open ci, honouring the
// rule that a run which both opens and closes may only pair when the two
// lengths do not sum to a multiple of three — unless both already are.
func (p *Parser) findOpener(lo, ci int) int {
	c := &p.delims[ci]
	for k := ci - 1; k >= lo; k-- {
		o := &p.delims[k]
		if o.role != roleRun || o.tok != c.tok || o.flags&dCanOpen == 0 || o.avail() <= 0 {
			continue
		}
		if o.flags&dCanClose != 0 || c.flags&dCanOpen != 0 {
			sum := o.length() + c.length()
			if sum%3 == 0 && (o.length()%3 != 0 || c.length()%3 != 0) {
				continue
			}
		}
		return k
	}
	return -1
}

func byteOf(tok Token) byte {
	switch tok {
	case TokStar:
		return '*'
	case TokUnderscore:
		return '_'
	}
	return '`'
}

// inlineNext is pass two: the next node, or false once the block's content is
// spent. Within one record the nodes come out in byte order — the closes that
// end on its first bytes, then whatever text no match claimed, then the opens
// that begin on its last bytes.
func (p *Parser) inlineNext() (Node, bool) {
	for {
		if int(p.emitIdx) >= len(p.delims) {
			if p.emitPos < p.inlineEnd {
				n := Node{Kind: KindText, Start: p.emitPos, End: p.inlineEnd}
				p.emitPos = p.inlineEnd
				return n, true
			}
			return Node{}, false
		}
		d := &p.delims[p.emitIdx]
		switch d.role {
		case roleTail, roleDest, roleTitle:
			p.emitIdx++
			continue
		case roleLeaf, roleBreak:
			if d.start > p.emitPos {
				n := Node{Kind: KindText, Start: p.emitPos, End: d.start}
				p.emitPos = d.start
				return n, true
			}
			p.emitIdx++
			p.emitPos = d.end
			return Node{Kind: leafKind(d), Start: d.start, End: d.end}, true
		}
		if d.idle() {
			p.emitIdx++ // Literal bytes, left to the surrounding text node.
			continue
		}
		if n, ok := p.emitRun(d); ok {
			return n, true
		}
	}
}

// emitRun steps one record through its four phases, reporting false when it is
// spent so the caller moves on.
func (p *Parser) emitRun(d *delim) (Node, bool) {
	switch p.emitPhase {
	case phaseGap:
		// The pair cursor is seeded on each phase change, never lazily: -1 has
		// to mean "list spent" and nothing else, or a spent list restarts.
		p.emitPhase, p.emitPair = phaseClose, d.closeHead
		// With no close anchored here the run's unclaimed head simply continues
		// the preceding text, so both come out as one node.
		stop := d.start
		if d.closeUsed == 0 {
			stop = d.end - Pos(d.openUsed)
		}
		if stop > p.emitPos {
			n := Node{Kind: KindText, Start: p.emitPos, End: stop}
			p.emitPos = stop
			return n, true
		}

	case phaseClose:
		if p.emitPair >= 0 {
			pr := p.pairs[p.emitPair]
			p.emitPair = pr.nextClose
			if pr.closeEnd > p.emitPos {
				p.emitPos = pr.closeEnd
			}
			return Node{
				Kind: pr.kind, Flags: FlagClose, Attr: pr.attr,
				Start: pr.openStart, End: pr.closeEnd,
			}, true
		}
		p.emitPhase = phaseLeft

	case phaseLeft:
		p.emitPhase, p.emitPair = phaseOpen, d.openHead
		if stop := d.end - Pos(d.openUsed); stop > p.emitPos {
			n := Node{Kind: KindText, Start: p.emitPos, End: stop}
			p.emitPos = stop
			return n, true
		}

	case phaseOpen:
		if p.emitPair >= 0 {
			pr := p.pairs[p.emitPair]
			p.emitPair = pr.nextOpen
			flags := Flags(FlagOpen)
			if pr.kind.IsLeaf() {
				// Dest and Title stand alone, and their spans sit in the tail
				// rather than here, so they must not move the content cursor
				// past the text they are announced ahead of.
				flags = 0
			} else if pr.openEnd > p.emitPos {
				p.emitPos = pr.openEnd
			}
			return Node{
				Kind: pr.kind, Flags: flags, Attr: pr.attr,
				Start: pr.openStart, End: pr.openEnd,
			}, true
		}
		if d.end > p.emitPos {
			p.emitPos = d.end
		}
		p.emitIdx++
		p.emitPhase, p.emitPair = phaseGap, -1
	}
	return Node{}, false
}

func leafKind(d *delim) Kind {
	switch {
	case d.role == roleBreak && d.flags&dHard != 0:
		return KindHardBreak
	case d.role == roleBreak:
		return KindSoftBreak
	case d.tok == TokEscape:
		return KindEscape
	case d.tok == TokEntity:
		return KindEntity
	}
	return KindAutolink
}
