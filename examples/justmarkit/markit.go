package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/soypat/lexorg"
	"github.com/soypat/ohimark"
)

type Flags struct {
	Input string // If empty use stdin.
	// Output string // If empty use stdout
}

func main() {
	var flags Flags
	flag.StringVar(&flags.Input, "i", "", "Input markdown file. If not set is stdin")
	flag.Parse()
	start := time.Now()
	err := run(flags)
	elapsed := time.Since(start)

	if err != nil {
		log.Fatalf("fail in %s: %s", elapsed, err)
	}
	log.Println("finished", elapsed)
}

func run(flags Flags) (err error) {
	var fp io.ReaderAt
	if flags.Input != "" {
		fp, err = os.Open(flags.Input)
		if err != nil {
			return err
		}
	} else {
		var rd lexorg.StreamReaderAt
		rd.Reset(os.Stdin, make([]byte, 2048))
		fp = &rd
	}

	var parser ohimark.Parser
	err = parser.Reset(flags.Input, fp, make([]byte, 2048))
	if err != nil {
		return err
	}
	oc := []string{"open", "close"}
	var totlen int64
	for {
		node, err := parser.Next()
		if err != nil {
			return err
		}
		n := node.Len()
		totlen += n
		fmt.Println(node.Kind.String(), oc[b2i(node.IsClose())], "len:", n)
		if node.Kind == ohimark.KindDocument && node.IsClose() {
			break
		}
	}
	// fmt.Println("total length:", totlen)
	return nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
