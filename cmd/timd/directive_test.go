package main

import "testing"

func TestParseDirective(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want directive
		bad  bool
	}{
		{name: "not a comment", src: "just text", bad: true},
		{name: "comment without a verb", src: "<!-- a note -->", bad: true},
		{name: "unknown verb", src: "<!-- .image f.png -->", bad: true},
		{name: "no file", src: "<!-- .code -->", bad: true},

		{name: "whole file", src: "<!-- .code f.go -->",
			want: directive{file: "f.go"}},
		{name: "one term", src: "<!-- .code f.go /fmt/ -->",
			want: directive{file: "f.go", from: "fmt"}},
		{name: "two terms", src: "<!-- .code f.go /package/,/end/ -->",
			want: directive{file: "f.go", from: "package", to: "end", ranged: true}},
		{name: "loose spacing", src: "<!--   .code   f.go   /a/,/b/   -->",
			want: directive{file: "f.go", from: "a", to: "b", ranged: true}},
		{name: "term holding spaces", src: "<!-- .code f.go /func main/ -->",
			want: directive{file: "f.go", from: "func main"}},
		{name: "term holding a slash", src: "<!-- .code f.go /a/b/,/c/ -->",
			want: directive{file: "f.go", from: "a/b", to: "c", ranged: true}},
		// The file is taken off the front, so one named after a word already in
		// the comment is not confused for it.
		{name: "file named like the verb", src: "<!-- .code code /a/ -->",
			want: directive{file: "code", from: "a"}},

		// Addresses are literal substrings, so anything that only means
		// something as a regexp or a line number is refused rather than matched
		// as itself.
		{name: "line number", src: "<!-- .code f.go 5 -->", bad: true},
		{name: "line range", src: "<!-- .code f.go 1,10 -->", bad: true},
		{name: "dollar", src: "<!-- .code f.go /a/,$ -->", bad: true},
		{name: "unterminated term", src: "<!-- .code f.go /a -->", bad: true},
		{name: "empty term", src: "<!-- .code f.go // -->", bad: true},
		{name: "trailing comma", src: "<!-- .code f.go /a/, -->", bad: true},
		{name: "three terms", src: "<!-- .code f.go /a/,/b/,/c/ -->", bad: true},
		{name: "metachar dot", src: "<!-- .code f.go /a.c/ -->", bad: true},
		{name: "metachar star", src: "<!-- .code f.go /a*/ -->", bad: true},
		{name: "metachar group", src: "<!-- .code f.go /(a|b)/ -->", bad: true},
		{name: "metachar class", src: "<!-- .code f.go /[ab]/ -->", bad: true},
		{name: "metachar anchor", src: "<!-- .code f.go /^a/ -->", bad: true},
		{name: "metachar escape", src: `<!-- .code f.go /a\.c/ -->`, bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDirective([]byte(tt.src))
			if tt.bad {
				if err == nil {
					t.Fatalf("parseDirective(%q) = %+v, want an error", tt.src, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDirective(%q): %v", tt.src, err)
			}
			if got != tt.want {
				t.Errorf("parseDirective(%q) = %+v, want %+v", tt.src, got, tt.want)
			}
		})
	}
}

// TestParseDirectiveNotDirective pins the difference between "not for us" and
// "malformed": a paragraph that is not a directive at all must be left alone,
// not reported as a broken one.
func TestParseDirectiveNotDirective(t *testing.T) {
	for _, src := range []string{
		"just text",
		".code f.go /a/",           // The bare form the example file shows.
		"<!-- a note -->",          // A comment, but not ours.
		"<!-- .image f.png -->",    // Another tool's verb.
		"text <!-- .code f.go -->", // Not the whole paragraph.
		"<!-- .code f.go --> text",
	} {
		if _, err := parseDirective([]byte(src)); err == nil {
			t.Errorf("parseDirective(%q) matched, want no match", src)
		} else if !isNotDirective(err) {
			t.Errorf("parseDirective(%q) = %v, want a no-match", src, err)
		}
	}
}
