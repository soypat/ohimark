package main

import (
	"errors"
	"fmt"
	"strings"
)

// A directive is one "<!-- .code file addr -->" comment. The address selects
// lines by literal substring, not by regexp: a term means itself, so a file
// holding "a.c" is found by writing "a.c" rather than escaping it.
//
//	<!-- .code f.go -->              the whole file
//	<!-- .code f.go /needle/ -->     the first line holding needle
//	<!-- .code f.go /a/,/b/ -->      that line through the first at or after
//	                                 it holding b, both ends included
type directive struct {
	file   string
	from   string
	to     string
	ranged bool // A second term was given, so to bounds the selection.
}

const (
	commentOpen  = "<!--"
	commentClose = "-->"
	verb         = ".code"
)

// errNotDirective marks a paragraph this tool has no claim on, which is not a
// failure: markdown is full of comments and text that are not ours. A directive
// that is ours but malformed reports what is wrong with it instead.
var errNotDirective = errors.New("not a .code directive")

func isNotDirective(err error) bool { return errors.Is(err, errNotDirective) }

// parseDirective reads one paragraph's literal text. It matches only when the
// comment is the whole paragraph, so a directive quoted mid-sentence is text.
func parseDirective(lit []byte) (directive, error) {
	s := strings.TrimSpace(string(lit))
	body, ok := strings.CutPrefix(s, commentOpen)
	if !ok {
		return directive{}, errNotDirective
	}
	body, ok = strings.CutSuffix(body, commentClose)
	if !ok || strings.Contains(body, commentClose) {
		return directive{}, errNotDirective
	}
	rest := strings.TrimSpace(body)
	fields := strings.Fields(rest)
	if len(fields) == 0 || fields[0] != verb {
		return directive{}, errNotDirective
	}
	// Past here the comment is ours, so anything wrong with it is an error the
	// author wants to hear about rather than a silent pass-through.
	if len(fields) < 2 {
		return directive{}, errors.New(verb + " names no file")
	}
	// Taking each part off the front rather than searching for it keeps a file
	// named after a word already in the comment from matching the wrong one.
	rest = strings.TrimSpace(rest[len(verb):])
	d := directive{file: fields[1]}
	// The address may hold spaces, so it is everything left rather than the
	// fields after the file.
	addr := strings.TrimSpace(rest[len(d.file):])
	if addr == "" {
		return d, nil
	}
	var err error
	d.from, d.to, d.ranged, err = parseAddr(addr)
	if err != nil {
		return directive{}, err
	}
	return d, nil
}

// parseAddr splits "/a/" or "/a/,/b/" into its terms. Every other address form
// present understands — a line number, a range of them, "$" — is refused
// rather than guessed at, as is a term that only means something as a regexp.
func parseAddr(addr string) (from, to string, ranged bool, err error) {
	from, rest, err := cutTerm(addr)
	if err != nil {
		return "", "", false, err
	}
	if rest == "" {
		return from, "", false, nil
	}
	rest, ok := strings.CutPrefix(rest, ",")
	if !ok {
		return "", "", false, fmt.Errorf("unexpected %q after the address term", rest)
	}
	to, rest, err = cutTerm(strings.TrimSpace(rest))
	if err != nil {
		return "", "", false, err
	}
	if rest != "" {
		return "", "", false, fmt.Errorf("unexpected %q after the address", rest)
	}
	return from, to, true, nil
}

// cutTerm takes one "/term/" off the front of s. A term runs to the last slash
// before the next comma, so it may hold slashes of its own.
func cutTerm(s string) (term, rest string, err error) {
	body, ok := strings.CutPrefix(s, "/")
	if !ok {
		return "", "", fmt.Errorf("address %q is not a /term/: line numbers and $ are unsupported", s)
	}
	end := strings.LastIndex(body, "/")
	if i := strings.Index(body, ","); i >= 0 {
		end = strings.LastIndex(body[:i], "/")
	}
	if end < 0 {
		return "", "", fmt.Errorf("address term %q is not closed", s)
	}
	term, rest = body[:end], strings.TrimSpace(body[end+1:])
	if term == "" {
		return "", "", errors.New("address term is empty")
	}
	if i := strings.IndexAny(term, metachars); i >= 0 {
		return "", "", fmt.Errorf("address term %q holds %q: terms are literal text, not regexps",
			term, term[i])
	}
	return term, rest, nil
}

// metachars are the characters that would mean something other than themselves
// to a regexp. Refusing them keeps a term that looks like a pattern from
// silently matching as literal text instead.
const metachars = `.*+?()[]{}|^$\`
