package main

import (
	"strings"
	"testing"
)

// The asset the example file draws from, inline so the test says what it means
// rather than tracking a file on disk.
const helloworld = `//go:build omit

package main

import "fmt"

func main() {
	fmt.Println("Hello world!")
}

// omit end
`

func TestExtract(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		dir  directive
		want string
		bad  bool
	}{{
		// The whole file, minus the omitted lines, is what a bare directive
		// yields. Selection runs before the omit filter, so this is also the
		// baseline the ranged cases trim.
		name: "whole file",
		src:  helloworld,
		want: "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello world!\")\n}\n",
	}, {
		// The range ends on "// omit end", which the omit filter then drops:
		// selecting first is what lets a marker line bound a range without
		// showing up in it.
		name: "range to an omitted marker",
		src:  helloworld,
		dir:  directive{from: "package", to: "end", ranged: true},
		want: "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello world!\")\n}\n",
	}, {
		// A literal substring, so the first line holding "fmt" wins — the
		// import, not the call.
		name: "one line",
		src:  helloworld,
		dir:  directive{from: "fmt"},
		want: "import \"fmt\"\n",
	}, {
		name: "range within a body",
		src:  helloworld,
		dir:  directive{from: "func main", to: "}", ranged: true},
		want: "func main() {\n\tfmt.Println(\"Hello world!\")\n}\n",
	}, {
		// Common indentation goes, relative indentation stays.
		name: "dedent",
		src:  "\tfunc a() {\n\t\tb()\n\t}\n",
		want: "func a() {\n\tb()\n}\n",
	}, {
		name: "blank edges trimmed",
		src:  "\n\n  x\n\n\n",
		want: "x\n",
	}, {
		name: "everything omitted",
		src:  "a // omit\nb // omit\n",
		bad:  true,
	}, {
		name: "start not found",
		src:  helloworld,
		dir:  directive{from: "nowhere"},
		bad:  true,
	}, {
		name: "end not found",
		src:  helloworld,
		dir:  directive{from: "package", to: "nowhere", ranged: true},
		bad:  true,
	}, {
		// The end term is looked for at or after the start, so an earlier match
		// does not close the range.
		name: "end before start",
		src:  helloworld,
		dir:  directive{from: "func main", to: "//go:build", ranged: true},
		bad:  true,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extract(strings.NewReader(tt.src), tt.dir)
			if tt.bad {
				if err == nil {
					t.Fatalf("extract = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("extract =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestOmitLine pins the delimiting rule: "omit" is a word, not a substring.
func TestOmitLine(t *testing.T) {
	for src, want := range map[string]bool{
		"a // omit": true,
		"omit":      true,
		"\tomit\t":  true,
		"x omit y":  true,
		"omitted":   false,
		"vomit":     false,
		"a//omit":   false,
		"\"omit\"":  false,
		"":          false,
	} {
		if got := omitLine([]byte(src)); got != want {
			t.Errorf("omitLine(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestFenceFor(t *testing.T) {
	for _, tt := range []struct {
		content string
		want    string
	}{
		{content: "plain\n", want: "```"},
		{content: "a ``` b\n", want: "````"},
		{content: "a ````` b\n", want: "``````"},
		{content: "a ` b\n", want: "```"},
	} {
		if got := fenceFor([]byte(tt.content)); got != tt.want {
			t.Errorf("fenceFor(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

func TestInfoFor(t *testing.T) {
	for path, want := range map[string]string{
		"a/b/helloworld.go": "go",
		"x.py":              "py",
		"Makefile":          "",
		"a.b/c":             "",
	} {
		if got := infoFor(path); got != want {
			t.Errorf("infoFor(%q) = %q, want %q", path, got, want)
		}
	}
}
