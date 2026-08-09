package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"
)

// omit is the keyword that drops a line from the output. It is a word, so a
// line reading "omitted" keeps.
const omitWord = "omit"

// extract reads r and returns the lines d selects, as one block ready to go
// inside a fence.
//
// Order matters: the address is matched against the file as written, and only
// then are omitted lines dropped. A marker line can therefore bound a range
// without appearing in it, which is what "/package/,/end/" over a file ending
// in "// omit end" relies on.
func extract(r io.Reader, d directive) ([]byte, error) {
	lines, err := selectLines(r, d)
	if err != nil {
		return nil, err
	}
	lines = dropOmitted(lines)
	lines = trimBlankEdges(lines)
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s selects no lines", d.file)
	}
	return join(lines, commonIndent(lines)), nil
}

// selectLines returns the address's range of lines, or every line when the
// directive names no address.
func selectLines(r io.Reader, d directive) ([][]byte, error) {
	var lines [][]byte
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	from, to := -1, -1
	for i := 0; sc.Scan(); i++ {
		// Scanner reuses its buffer, so each line has to be kept by value.
		lines = append(lines, bytes.Clone(sc.Bytes()))
		switch {
		case d.from == "":
		case from < 0:
			if bytes.Contains(lines[i], []byte(d.from)) {
				from = i
				if !d.ranged {
					to = i
				}
			}
		case d.ranged && to < 0:
			if bytes.Contains(lines[i], []byte(d.to)) {
				to = i
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if d.from == "" {
		return lines, nil
	}
	if from < 0 {
		return nil, fmt.Errorf("%s holds no line with %q", d.file, d.from)
	}
	if to < 0 {
		return nil, fmt.Errorf("%s holds no line with %q at or after %q", d.file, d.to, d.from)
	}
	return lines[from : to+1], nil
}

func dropOmitted(lines [][]byte) [][]byte {
	kept := lines[:0]
	for _, l := range lines {
		if !omitLine(l) {
			kept = append(kept, l)
		}
	}
	return kept
}

// omitLine reports whether the line carries the omit keyword, which must stand
// alone between whitespace rather than sit inside a longer word.
func omitLine(line []byte) bool {
	for _, f := range bytes.Fields(line) {
		if string(f) == omitWord {
			return true
		}
	}
	return false
}

func trimBlankEdges(lines [][]byte) [][]byte {
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func blank(line []byte) bool { return len(bytes.TrimSpace(line)) == 0 }

// commonIndent returns the whitespace prefix every non-blank line shares, which
// is what a fence should not carry: the excerpt is shown on its own, not at the
// depth it sat in its file.
func commonIndent(lines [][]byte) []byte {
	var common []byte
	first := true
	for _, l := range lines {
		if blank(l) {
			continue
		}
		indent := l[:len(l)-len(bytes.TrimLeft(l, " \t"))]
		if first {
			common, first = indent, false
			continue
		}
		n := 0
		for n < len(common) && n < len(indent) && common[n] == indent[n] {
			n++
		}
		common = common[:n]
	}
	return common
}

func join(lines [][]byte, indent []byte) []byte {
	var b bytes.Buffer
	for _, l := range lines {
		b.Write(bytes.TrimPrefix(l, indent))
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// fenceFor returns a backtick run long enough to hold content, which needs to
// outrun any run inside it.
func fenceFor(content []byte) string {
	const min = 3
	longest, run := 0, 0
	for _, c := range content {
		if c != '`' {
			run = 0
			continue
		}
		if run++; run > longest {
			longest = run
		}
	}
	return strings.Repeat("`", max(min, longest+1))
}

// infoFor returns the fence's info string: the file's extension, which is what
// a highlighter wants, or nothing when it has none.
func infoFor(file string) string {
	ext := path.Ext(path.Base(file))
	if ext == "" {
		return ""
	}
	return ext[1:]
}
