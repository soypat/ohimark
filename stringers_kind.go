// Hand-written placeholder matching what `go generate` produces for Kind.
// Running `go generate ./...` overwrites this file.

package ohimark

import "strconv"

var _kindNames = [...]string{
	KindUndefined:     "undefined",
	KindDocument:      "document",
	containerBeg:      "containerBeg",
	KindBlockQuote:    "blockquote",
	KindList:          "list",
	KindItem:          "item",
	containerEnd:      "containerEnd",
	leafBeg:           "leafBeg",
	KindParagraph:     "paragraph",
	KindHeading:       "heading",
	KindCodeBlock:     "codeblock",
	KindThematicBreak: "thematicbreak",
	leafEnd:           "leafEnd",
	inlineBeg:         "inlineBeg",
	KindEmph:          "emph",
	KindStrong:        "strong",
	KindLink:          "link",
	KindImage:         "image",
	KindCodeSpan:      "codespan",
	KindText:          "text",
	KindEscape:        "escape",
	KindEntity:        "entity",
	KindAutolink:      "autolink",
	KindSoftBreak:     "softbreak",
	KindHardBreak:     "hardbreak",
	inlineEnd:         "inlineEnd",
	KindInfo:          "info",
	KindDest:          "dest",
	KindTitle:         "title",
	KindRaw:           "raw",
}

func (k Kind) String() string {
	if int(k) >= len(_kindNames) {
		return "Kind(" + strconv.Itoa(int(k)) + ")"
	}
	return _kindNames[k]
}
