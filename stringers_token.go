// Hand-written placeholder matching what `go generate` produces for Token.
// Running `go generate ./...` overwrites this file.

package ohimark

import "strconv"

var _tokenNames = [...]string{
	TokUndefined:  "undefined",
	TokIllegal:    "illegal",
	TokEOF:        "EOF",
	TokWord:       "<word>",
	TokDigits:     "<digits>",
	TokSpace:      "<space>",
	TokNewline:    "<newline>",
	runBeg:        "runBeg",
	TokHash:       "#",
	TokStar:       "*",
	TokUnderscore: "_",
	TokTick:       "`",
	TokTilde:      "~",
	TokDash:       "-",
	TokGT:         ">",
	runEnd:        "runEnd",
	TokPlus:       "+",
	TokBang:       "!",
	TokLT:         "<",
	TokBracketL:   "[",
	TokBracketR:   "]",
	TokParenL:     "(",
	TokParenR:     ")",
	TokDot:        ".",
	TokEscape:     "<escape>",
	TokEntity:     "<entity>",
}

func (tok Token) String() string {
	if int(tok) >= len(_tokenNames) {
		return "Token(" + strconv.Itoa(int(tok)) + ")"
	}
	return _tokenNames[tok]
}
