package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/soypat/ohimark"
)

type Flags struct {
	Target string
	Dir    bool
}

func main() {
	var flags Flags
	flag.StringVar(&flags.Target, "tgt", "", "Target file to transform. If -d is set will transform all markdown files in directory.")
	flag.BoolVar(&flags.Dir, "d", false, "Runs on the tgt directory/folder. Will fail if tgt is a file.")
	flag.Parse()

	start := time.Now()
	err := run(flags)
	elapsed := time.Since(start)
	if err != nil {
		log.Fatal("fail", elapsed, err)
	}
	log.Println("success", elapsed)
}

func run(flags Flags) error {
	if flags.Target == "" {
		flag.CommandLine.Usage()
		return errors.New("empty target provided")
	}
	info, err := os.Stat(flags.Target)
	if err != nil {
		return err
	} else if info.IsDir() != flags.Dir {
		return fmt.Errorf("expected dir=%v, got %v", flags.Dir, info.IsDir())
	}
	buf := make([]byte, 2048)
	var parser ohimark.Parser
	const Directory, File = true, false
	switch flags.Dir {
	case Directory:
		err = errors.New("directory mode unsupported")
	case File:
		var fp *os.File
		fp, err = os.Open(flags.Target)
		if err != nil {
			break
		}
		defer fp.Close()
		parser.Reset(flags.Target, fp, buf)
		var output bytes.Buffer
		err = transform(&parser, &output)
		if err != nil {
			break
		}
		_, err = fp.Seek(0, 0)
		if err != nil {
			break
		}
		err = os.WriteFile(flags.Target, output.Bytes(), 0777)
	}
	return fmt.Errorf("failure during transform: %w", err)
}
