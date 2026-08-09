package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/soypat/ohimark"
)

// transformer rewrites one markdown file, filling the code blocks its .code
// directives name. Everything it does not own is copied through byte for byte:
// the parser hands out spans, so untouched markdown is never re-rendered and
// never reformatted.
type transformer struct {
	src  io.ReaderAt // The markdown being transformed.
	name string      // Its path, for diagnostics.
	dir  string      // Its directory, which asset paths resolve against.

	// cursor is the offset everything before which has already been written.
	// It jumps over a stale generated block rather than copying it.
	cursor ohimark.Pos

	aux  []byte // Holds one node's bytes when the parser could not.
	copy []byte // Copy buffer for the verbatim spans.
}

// run walks the document and writes the transformed markdown to w.
func (t *transformer) run(p *ohimark.Parser, w io.Writer) error {
	var (
		block  []byte // The fence a directive produced, not yet written.
		filled bool   // A directive is waiting to see what follows it.
		stale  bool   // What follows is the block it generated last time.
	)
	for {
		n, lit, ok, err := t.next(p)
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		// A directive's block goes in once it is known whether the next block
		// is the one it wrote last time, which is the only block it may
		// replace.
		if filled && !(n.Kind == ohimark.KindCodeBlock && n.IsOpen() && n.Flags&ohimark.FlagFenced != 0) {
			if _, err = fmt.Fprintf(w, "\n\n%s", block); err != nil {
				return err
			}
			filled, stale = false, false
		} else if filled {
			filled, stale = false, true
			continue
		}
		if stale {
			if n.Kind == ohimark.KindCodeBlock && n.IsClose() {
				// Dropping it needs nothing written: skipping past it leaves
				// the source's own line ending to close the new fence, so
				// replacing reads the same as inserting.
				if _, err = fmt.Fprintf(w, "\n\n%s", block); err != nil {
					return err
				}
				t.cursor, stale = n.End, false
			}
			continue
		}

		switch {
		case n.Kind == ohimark.KindDocument && n.IsClose():
			return t.emit(w, n.End)

		case n.Kind != ohimark.KindParagraph || !n.IsClose():
			continue
		}

		// A close node spans its whole paragraph, so its literal is the
		// directive text without a byte read of its own.
		if !ok {
			if lit, err = t.readLiteral(n); err != nil {
				return err
			}
		}
		d, err := parseDirective(lit)
		if err != nil {
			if isNotDirective(err) {
				continue
			}
			return t.errAt(n.Start, err)
		}
		if block, err = t.render(d); err != nil {
			return t.errAt(n.Start, err)
		}
		if err = t.emit(w, n.End); err != nil {
			return err
		}
		filled = true
	}
}

// next is [ohimark.Parser.TryNextLiteral] with its refusal made into a flag:
// ok reports whether lit holds the node's bytes. Most nodes never need them,
// so the read the parser declined is left to whoever does.
func (t *transformer) next(p *ohimark.Parser) (n ohimark.Node, lit []byte, ok bool, err error) {
	n, lit, err = p.TryNextLiteral()
	if err == nil {
		return n, lit, true, nil
	} else if !errors.Is(err, ohimark.ErrLiteralUnavailable) {
		return n, nil, false, err
	}
	n, err = p.Next()
	return n, nil, false, err
}

// readLiteral fetches a node's bytes the slow way, for a span the parser's
// window had already passed.
func (t *transformer) readLiteral(n ohimark.Node) ([]byte, error) {
	if int64(cap(t.aux)) < n.Len() {
		t.aux = make([]byte, n.Len())
	}
	t.aux = t.aux[:n.Len()]
	if _, err := t.src.ReadAt(t.aux, int64(n.Start)); err != nil && err != io.EOF {
		return nil, err
	}
	return t.aux, nil
}

// render reads the asset the directive names and returns the fenced block that
// stands for it.
func (t *transformer) render(d directive) ([]byte, error) {
	fp, err := os.Open(filepath.Join(t.dir, filepath.FromSlash(d.file)))
	if err != nil {
		return nil, err
	}
	defer fp.Close()
	content, err := extract(fp, d)
	if err != nil {
		return nil, err
	}
	fence := fenceFor(content)
	// No trailing newline: the source's own is what ends the block, which is
	// what makes inserting and replacing produce the same bytes.
	return fmt.Appendf(nil, "%s%s\n%s%s", fence, infoFor(d.file), content, fence), nil
}

// emit copies the source through upto, which is everything the transform left
// alone.
func (t *transformer) emit(w io.Writer, upto ohimark.Pos) error {
	if upto <= t.cursor {
		return nil
	}
	if t.copy == nil {
		t.copy = make([]byte, 4096)
	}
	r := io.NewSectionReader(t.src, int64(t.cursor), int64(upto-t.cursor))
	if _, err := io.CopyBuffer(w, r, t.copy); err != nil {
		return err
	}
	t.cursor = upto
	return nil
}

// errAt names the line the failed directive sits on.
func (t *transformer) errAt(pos ohimark.Pos, err error) error {
	var aux [1024]byte
	line, col, _, lcerr := pos.ToLineCol(t.src, aux[:])
	if lcerr != nil {
		return fmt.Errorf("%s %v: %w", t.name, pos, err)
	}
	return fmt.Errorf("%s:%d:%d: %w", t.name, line, col, err)
}
