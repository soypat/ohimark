package ohimark

import (
	"io"
	"strconv"

	"github.com/soypat/lexorg"
)

// Node is one AST event: a construct's kind and the byte span it covers.
// Dereference Start and End with the io.ReaderAt passed to [Parser.Reset].
//
// A node's meaning is always a pure function of its span. Text splits at every
// escape and entity, so no node needs a value assembled in a buffer: "\*" is a
// KindEscape span rendered by skipping its first byte.
//
// An open node's span covers its marker ("## ", "> ", "```go\n"); the matching
// close node's covers the whole construct. A leaf's covers the leaf.
type Node struct {
	Kind  Kind
	Flags Flags
	_     [2]byte
	// Attr is kind-specific:
	//
	//	KindHeading    level, 1..6
	//	KindList       start number if FlagOrdered, else bullet byte
	//	KindItem       ordinal within the list, 0-based
	//	KindCodeBlock  fence run length if FlagFenced
	//	KindEmph, KindStrong, KindCodeSpan   delimiter byte
	//	otherwise      0
	Attr  int32
	Start Pos
	End   Pos
}

func (n Node) IsOpen() bool   { return n.Flags&FlagOpen != 0 }
func (n Node) IsClosed() bool { return n.Flags&FlagClose != 0 }

// Len returns the byte length of the node's span.
func (n Node) Len() int64 { return int64(n.End - n.Start) }

// Pos is an absolute byte offset into the markdown source.
type Pos int64

// String returns the offset formatted as "@0x<hex>".
func (pos Pos) String() string {
	var buf [7 + 3]byte
	return string(pos.AppendString(buf[:0]))
}

func (pos Pos) AppendString(dst []byte) []byte {
	dst = append(dst, "@0x"...)
	dst = strconv.AppendInt(dst, int64(pos), 16)
	return dst
}

// ToLineCol converts the offset to 1-indexed line:column plus that line's
// length, for diagnostics. aux is a read scratch buffer; 1024B is a good size.
func (pos Pos) ToLineCol(r io.ReaderAt, aux []byte) (line, col, lineLength int, err error) {
	return lexorg.ToLineCol(r, int64(pos), aux)
}

// Kind is the type of markdown construct a [Node] describes.
type Kind uint8

const (
	KindUndefined Kind = iota // undefined
	KindDocument              // document

	// Container blocks. Always emitted as an open/close pair.
	containerBeg
	KindBlockQuote // blockquote
	KindList       // list
	KindItem       // item
	containerEnd

	// Leaf blocks. Open/close pairs; inline nodes appear between them.
	leafBeg
	KindParagraph     // paragraph
	KindHeading       // heading
	KindCodeBlock     // codeblock
	KindThematicBreak // thematicbreak
	leafEnd

	// Inline containers. Open/close pairs.
	inlineBeg
	KindEmph     // emph
	KindStrong   // strong
	KindLink     // link
	KindImage    // image
	KindCodeSpan // codespan

	// Inline leaves. Single nodes, no close.
	KindText      // text
	KindEscape    // escape
	KindEntity    // entity
	KindAutolink  // autolink
	KindSoftBreak // softbreak
	KindHardBreak // hardbreak
	inlineEnd

	// Attribute leaves. Emitted right after their parent's open node, before its
	// content, so their spans are not in source order relative to that content.
	KindInfo  // info
	KindDest  // dest
	KindTitle // title
	KindRaw   // raw
)

// IsContainer reports whether k holds blocks rather than inline content.
func (k Kind) IsContainer() bool { return k > containerBeg && k < containerEnd }

// IsBlock reports whether k is a container or leaf block.
func (k Kind) IsBlock() bool { return k.IsContainer() || (k > leafBeg && k < leafEnd) }

// IsInline reports whether k is an inline construct.
func (k Kind) IsInline() bool { return k > inlineBeg && k < inlineEnd }

// IsLeaf reports whether k is emitted alone, with no matching close.
func (k Kind) IsLeaf() bool {
	switch k {
	case KindText, KindEscape, KindEntity, KindAutolink, KindSoftBreak, KindHardBreak,
		KindThematicBreak, KindInfo, KindDest, KindTitle, KindRaw:
		return true
	}
	return false
}

// Flags carries a [Node]'s open/close bit plus kind-specific bits.
type Flags uint8

const (
	FlagOpen  Flags = 1 << iota // Node opens a container.
	FlagClose                   // Node closes a container.

	FlagOrdered // KindList: "1." rather than "-".
	FlagLoose   // KindList close: an item was blank-line separated.
	FlagFenced  // KindCodeBlock: ``` rather than a 4-space indent.
)
