package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soypat/ohimark"
)

// run transforms src as if it were a file in dir, which is what the asset
// paths inside it resolve against.
func transformString(t *testing.T, dir, src string) string {
	t.Helper()
	var p ohimark.Parser
	r := strings.NewReader(src)
	if err := p.Reset(t.Name(), r, make([]byte, ohimark.MinWindow)); err != nil {
		t.Fatal(err)
	}
	tr := transformer{src: r, dir: dir}
	var out bytes.Buffer
	if err := tr.run(&p, &out); err != nil {
		t.Fatalf("transform: %v", err)
	}
	return out.String()
}

// assetDir writes helloworld.go into a temporary assets/ directory and returns
// the directory the markdown would sit in.
func assetDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "helloworld.go"), []byte(helloworld), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const filled = "```go\n" +
	"package main\n" +
	"\n" +
	"import \"fmt\"\n" +
	"\n" +
	"func main() {\n" +
	"\tfmt.Println(\"Hello world!\")\n" +
	"}\n" +
	"```"

func TestTransform(t *testing.T) {
	dir := assetDir(t)
	const directive = "<!-- .code assets/helloworld.go /package/,/end/ -->"

	for _, tt := range []struct {
		name string
		src  string
		want string
	}{{
		name: "insert",
		src:  "# h\n\n" + directive + "\n\ntail\n",
		want: "# h\n\n" + directive + "\n\n" + filled + "\n\ntail\n",
	}, {
		// The second run sees its own output and must reproduce it exactly.
		name: "replace",
		src:  "# h\n\n" + directive + "\n\n" + filled + "\n\ntail\n",
		want: "# h\n\n" + directive + "\n\n" + filled + "\n\ntail\n",
	}, {
		// A stale block gets thrown away whatever it held.
		name: "replace stale content",
		src:  "# h\n\n" + directive + "\n\n```go\nold and wrong\n```\n\ntail\n",
		want: "# h\n\n" + directive + "\n\n" + filled + "\n\ntail\n",
	}, {
		name: "directive at end of file",
		src:  "# h\n\n" + directive + "\n",
		want: "# h\n\n" + directive + "\n\n" + filled + "\n",
	}, {
		// A directive quoted inside a fence is content, not an instruction.
		name: "fenced copy untouched",
		src:  "# h\n\n```html\n" + directive + "\n```\n\ntail\n",
		want: "# h\n\n```html\n" + directive + "\n```\n\ntail\n",
	}, {
		// So is the bare form, which is not a comment at all.
		name: "bare form untouched",
		src:  "# h\n\n.code assets/helloworld.go /fmt/\n\ntail\n",
		want: "# h\n\n.code assets/helloworld.go /fmt/\n\ntail\n",
	}, {
		name: "no directive at all",
		src:  "# h\n\nsome *text* and a [link](x).\n\n    indented\n\n> quoted\n",
		want: "# h\n\nsome *text* and a [link](x).\n\n    indented\n\n> quoted\n",
	}, {
		// Only the block right after the directive is the tool's to replace.
		name: "unrelated fence kept",
		src:  "# h\n\ntext\n\n```sh\nls\n```\n\ntail\n",
		want: "# h\n\ntext\n\n```sh\nls\n```\n\ntail\n",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			got := transformString(t, dir, tt.src)
			if got != tt.want {
				t.Errorf("transform =\n%q\nwant\n%q", got, tt.want)
			}
			// Whatever it produced, producing it again must change nothing.
			if again := transformString(t, dir, got); again != got {
				t.Errorf("second run changed the output:\n%q\nwant\n%q", again, got)
			}
		})
	}
}

// TestTransformExample runs the tool over its own example file, which is the
// end-to-end case the README documents.
func TestTransformExample(t *testing.T) {
	const path = "examples/code-example.md"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := transformString(t, "examples", string(src))
	if !strings.Contains(got, filled) {
		t.Errorf("the range directive filled nothing:\n%s", got)
	}
	if !strings.Contains(got, "```go\nimport \"fmt\"\n```") {
		t.Errorf("the single-line directive filled nothing:\n%s", got)
	}
	// The quoted copy inside the html fence is content and stays as it was.
	if !strings.Contains(got, "```html\n<!-- .code assets/helloworld.go /fmt/ -->\n```") {
		t.Errorf("the fenced copy was rewritten:\n%s", got)
	}
	if again := transformString(t, "examples", got); again != got {
		t.Error("transforming the example twice is not the same as once")
	}
}

func TestTransformBadDirective(t *testing.T) {
	dir := assetDir(t)
	for _, src := range []string{
		"<!-- .code -->\n",                            // No file.
		"<!-- .code assets/helloworld.go 1,10 -->\n",  // A line range.
		"<!-- .code assets/helloworld.go /a.c/ -->\n", // A regexp.
		"<!-- .code assets/nosuch.go /package/ -->\n", // No such file.
		"<!-- .code assets/helloworld.go /zzz/ -->\n", // No such line.
	} {
		var p ohimark.Parser
		r := strings.NewReader(src)
		if err := p.Reset(t.Name(), r, make([]byte, ohimark.MinWindow)); err != nil {
			t.Fatal(err)
		}
		tr := transformer{src: r, dir: dir}
		if err := tr.run(&p, new(bytes.Buffer)); err == nil {
			t.Errorf("transform(%q) succeeded, want an error", src)
		}
	}
}
