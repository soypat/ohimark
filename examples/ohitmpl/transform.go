package main

import (
	"io"

	"github.com/soypat/ohimark"
)

func transform(parser *ohimark.Parser, w io.Writer) error {
	for {
		parser.Next()
	}
}
