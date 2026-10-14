package semantic

import (
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"
)

// tokKind classifies a scanned token.
type tokKind uint8

const (
	tkIdent   tokKind = iota // identifier, keyword or number
	tkPunct                  // operator or punctuation
	tkString                 // string, char or regex literal, heredoc, shebang, directive comment
	tkComment                // comment (ignored when comparing code)
	tkIndent                 // synthetic indentation token
)

type token struct {
	text string
	kind tokKind
}

// indentPrefix starts every synthetic indentation token. No real token can
// start with a NUL byte followed by a letter.
const indentPrefix = "\x00indent:"

// operators are the multi-character operators recognised by maximal munch, a
// union over the supported languages. Recognising them means that, say,
// "a <- b" and "a < -b" tokenize differently, while "x=-1" and "x = -1" do
// not. Brackets are never part of an operator.
var operators = func() []string {
	ops := []string{
		">>>=", "<<=", ">>=", ">>>", "...", "**=", "//=", "&&=", "||=", "??=",
		"&^=", "<=>", "===", "!==", "->*", "..=", "..<", "<<-", "%>%",
		"<-", "->", "=>", ":=", "::", "==", "!=", "<=", ">=", "&&", "||", "<<",
		">>", "++", "--", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "&^",
		"**", "//", "??", "?.", "!!", "..", "|>", "<|", "<>", "~=", "=~", "!~",
		".*", "?:", "@=", ":-", "##",
	}
	sort.SliceStable(ops, func(i, j int) bool { return len(ops[i]) > len(ops[j]) })
	return ops
}()

// regexKeywords are JavaScript keywords after which a '/' starts a regular
// expression literal rather than a division.
var regexKeywords = map[string]bool{
	"return": true, "typeof": true, "instanceof": true, "in": true, "of": true,
	"new": true, "delete": true, "void": true, "throw": true, "case": true,
	"do": true, "else": true, "yield": true, "await": true,
}

// Tokens returns the code tokens of content for lang, with comments removed
// and whitespace normalised away. It is exposed for testing and debugging.
//
// Tokens are identifiers and numbers, string/char/regex literals and heredoc
// bodies (verbatim, including delimiters), and punctuation (multi-character
// operators by maximal munch). A leading "#!" line and comments that are
// compiler directives (such as Go's //go:build) are kept as tokens. CRLF line
// endings are normalised to LF first.
//
// For WhitespaceSignificant languages, the first token of each line that
// starts outside a multi-line literal is preceded by a synthetic token
// "\x00indent:" followed by the line's leading whitespace, verbatim.
func Tokens(lang Language, content []byte) []string {
	toks := scan(lang, content)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		if t.kind != tkComment {
			out = append(out, t.text)
		}
	}
	return out
}

type openKind uint8

// Opener kinds, in tie-break order for openers of equal length.
const (
	okString openKind = iota
	okBlock
	okLine
	okHeredoc
)

type opener struct {
	text string
	kind openKind
	idx  int // index into the Language slice the opener came from
}

type match struct {
	o    opener
	end  int    // heredoc: end of the introducer
	word string // heredoc: the delimiter word
}

type pendingHeredoc struct {
	word     string
	indented bool
	loose    bool
}

type scanner struct {
	lang    Language
	src     []byte
	openers []opener // longest first
	first   [256]bool

	out          []token
	prev         token // last code token
	lineStart    int
	lineHasToken bool
	pending      []pendingHeredoc
}

func scan(lang Language, content []byte) []token {
	src := content
	if bytes.IndexByte(src, '\r') >= 0 {
		src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	}
	s := &scanner{lang: lang, src: src}
	s.prepare()
	s.run()
	s.applyPreamble()
	return s.out
}

func (s *scanner) prepare() {
	add := func(text string, kind openKind, idx int) {
		if text == "" {
			return
		}
		s.openers = append(s.openers, opener{text: text, kind: kind, idx: idx})
		s.first[text[0]] = true
	}
	for i, d := range s.lang.Strings {
		add(d.Open, okString, i)
	}
	for i, b := range s.lang.BlockComments {
		add(b[0], okBlock, i)
	}
	for i, l := range s.lang.LineComments {
		add(l, okLine, i)
	}
	for i, h := range s.lang.Heredocs {
		add(h.Open, okHeredoc, i)
	}
	sort.SliceStable(s.openers, func(i, j int) bool {
		a, b := s.openers[i], s.openers[j]
		if len(a.text) != len(b.text) {
			return len(a.text) > len(b.text)
		}
		return a.kind < b.kind
	})
}

func hasPrefixAt(src []byte, i int, p string) bool {
	return len(src)-i >= len(p) && string(src[i:i+len(p)]) == p
}

// lineEnd returns the index of the first '\n' at or after i, or len(src).
func lineEnd(src []byte, i int) int {
	if k := bytes.IndexByte(src[i:], '\n'); k >= 0 {
		return i + k
	}
	return len(src)
}

func isDigit(c byte) bool  { return '0' <= c && c <= '9' }
func isLetter(c byte) bool { return 'a' <= c|0x20 && c|0x20 <= 'z' }

// isIdentByte reports whether c can appear in an identifier or number. '$' is
// included so that shell/PHP/JS variables such as $x stay one token, and all
// non-ASCII bytes are included so that Unicode identifiers do too.
func isIdentByte(c byte) bool {
	return isLetter(c) || isDigit(c) || c == '_' || c == '$' || c >= 0x80
}

func (s *scanner) run() {
	src, n := s.src, len(s.src)
	i := 0
	// A #! line is an interpreter directive, not a comment ("#![" is a Rust
	// inner attribute and is scanned normally).
	if n >= 2 && src[0] == '#' && src[1] == '!' && (n == 2 || src[2] != '[') {
		end := lineEnd(src, 0)
		s.emit(string(src[:end]), tkString)
		i = end
	}
	for i < n {
		c := src[i]
		if c == '\n' {
			i = s.newline(i)
			continue
		}
		if isSpace(c) {
			i++
			continue
		}
		if m, ok := s.matchOpener(i); ok {
			i = s.consume(i, m)
			continue
		}
		if c == '\\' && s.lang.CodeEscapes && i+1 < n {
			_, size := utf8.DecodeRune(src[i+1:])
			end := i + 1 + size
			s.emit(string(src[i:end]), tkPunct)
			if src[i+1] == '\n' { // line continuation
				s.lineStart = end
				s.lineHasToken = true
			}
			i = end
			continue
		}
		if c == '/' && s.lang.RegexLiterals && s.regexAllowed() {
			if end, ok := s.scanRegex(i); ok {
				s.emit(string(src[i:end]), tkString)
				i = end
				continue
			}
		}
		if c == '\'' && s.lang.CharLiterals {
			if end, ok := s.scanChar(i); ok {
				s.emit(string(src[i:end]), tkString)
				i = end
				continue
			}
		}
		var end int
		kind := tkPunct
		if isIdentByte(c) {
			end, kind = s.scanIdent(i), tkIdent
		} else {
			end = s.scanPunct(i)
		}
		s.emit(string(src[i:end]), kind)
		i = end
	}
}

// emit appends a code token, preceded by an indentation token when it is the
// first token of a line in a whitespace-significant language.
func (s *scanner) emit(text string, kind tokKind) {
	if s.lang.WhitespaceSignificant && !s.lineHasToken {
		j := s.lineStart
		for j < len(s.src) && (s.src[j] == ' ' || s.src[j] == '\t' || s.src[j] == '\f' || s.src[j] == '\v') {
			j++
		}
		s.out = append(s.out, token{indentPrefix + string(s.src[s.lineStart:j]), tkIndent})
	}
	s.lineHasToken = true
	s.prev = token{text, kind}
	s.out = append(s.out, token{text, kind})
}

// crossLines updates line tracking after a token spanning src[start:end]; a
// line that starts inside a multi-line token is not a logical line start.
func (s *scanner) crossLines(start, end int) {
	if k := bytes.LastIndexByte(s.src[start:end], '\n'); k >= 0 {
		s.lineStart = start + k + 1
		s.lineHasToken = true
	}
}

// newline handles the '\n' at i, consuming any pending heredoc bodies, and
// returns the next position to scan.
func (s *scanner) newline(i int) int {
	i++
	s.lineStart = i
	s.lineHasToken = false
	pend := s.pending
	s.pending = nil
	for k, h := range pend {
		if i >= len(s.src) {
			break
		}
		end := s.heredocEnd(i, h)
		s.lineHasToken = true // a heredoc body is not a logical line
		s.emit(string(s.src[i:end]), tkString)
		s.crossLines(i, end)
		i = end
		if k < len(pend)-1 && i < len(s.src) {
			i++ // the next body starts after this terminator line
		}
	}
	return i
}

func (s *scanner) matchOpener(i int) (match, bool) {
	src := s.src
	if !s.first[src[i]] {
		return match{}, false
	}
	for _, o := range s.openers {
		if !hasPrefixAt(src, i, o.text) {
			continue
		}
		switch o.kind {
		case okLine:
			if s.lang.LineCommentNeedsSpace && i > 0 && !isSpace(src[i-1]) {
				continue
			}
		case okHeredoc:
			end, word, ok := s.parseHeredoc(i, s.lang.Heredocs[o.idx])
			if !ok {
				continue
			}
			return match{o: o, end: end, word: word}, true
		}
		return match{o: o}, true
	}
	return match{}, false
}

// consume scans the construct introduced by m at i and returns the position
// after it.
func (s *scanner) consume(i int, m match) int {
	src, n := s.src, len(s.src)
	switch m.o.kind {
	case okLine:
		end := lineEnd(src, i)
		s.comment(i, end)
		return end
	case okBlock:
		closer := s.lang.BlockComments[m.o.idx][1]
		start := i + len(m.o.text)
		end := n
		if closer != "" {
			if k := bytes.Index(src[start:], []byte(closer)); k >= 0 {
				end = start + k + len(closer)
			}
		}
		s.comment(i, end)
		return end
	case okString:
		end, ok := s.scanString(i, s.lang.Strings[m.o.idx])
		if !ok {
			// Unterminated single-line string: treat the quote as
			// punctuation and resynchronise.
			s.emit(string(src[i:i+1]), tkPunct)
			return i + 1
		}
		s.emit(string(src[i:end]), tkString)
		s.crossLines(i, end)
		return end
	case okHeredoc:
		h := s.lang.Heredocs[m.o.idx]
		s.emit(string(src[i:m.end]), tkString)
		s.pending = append(s.pending, pendingHeredoc{word: m.word, indented: h.IndentedEnd, loose: h.LooseEnd})
		return m.end
	}
	return i + 1
}

// comment records the comment src[start:end], or a code token if it is a
// directive.
func (s *scanner) comment(start, end int) {
	text := string(s.src[start:end])
	if s.isDirective(text) {
		s.emit(text, tkString)
	} else {
		s.out = append(s.out, token{text, tkComment})
	}
	s.crossLines(start, end)
}

func (s *scanner) isDirective(text string) bool {
	for _, d := range s.lang.Directives {
		if d != "" && len(text) >= len(d) && strings.EqualFold(text[:len(d)], d) {
			return true
		}
	}
	return false
}

func (s *scanner) scanString(i int, d StringDelim) (int, bool) {
	src, n := s.src, len(s.src)
	j := i + len(d.Open)
	if d.Close == "" {
		return j, true
	}
	for j < n {
		c := src[j]
		if d.Escape != 0 && c == d.Escape {
			j += 2
			continue
		}
		if hasPrefixAt(src, j, d.Close) {
			return j + len(d.Close), true
		}
		if c == '\n' && !d.Multiline {
			return 0, false
		}
		j++
	}
	if d.Multiline {
		return n, true // unterminated: runs to end of file
	}
	return 0, false
}

// scanChar recognises a character literal such as 'a', '"', '\n' or
// '\u{1F600}' at i. A lone quote (a Rust lifetime, a C++ digit separator,
// a Haskell prime) is not one.
func (s *scanner) scanChar(i int) (int, bool) {
	src, n := s.src, len(s.src)
	j := i + 1
	if j >= n {
		return 0, false
	}
	if src[j] == '\\' {
		for k := j + 2; k < n && k <= i+16; k++ {
			switch src[k] {
			case '\n':
				return 0, false
			case '\'':
				return k + 1, true
			}
		}
		return 0, false
	}
	r, size := utf8.DecodeRune(src[j:])
	if r == '\n' || r == '\'' {
		return 0, false
	}
	if j+size < n && src[j+size] == '\'' {
		return j + size + 1, true
	}
	return 0, false
}

// regexAllowed reports whether a '/' at the current position would start a
// JavaScript regular expression literal, judging by the previous token.
func (s *scanner) regexAllowed() bool {
	p := s.prev
	if p.text == "" {
		return true
	}
	switch p.kind {
	case tkPunct:
		switch p.text {
		case ")", "]", "}":
			return false
		}
		return true
	case tkIdent:
		return regexKeywords[p.text]
	}
	return false
}

// scanRegex scans a regular expression literal (with flags) at i.
func (s *scanner) scanRegex(i int) (int, bool) {
	src, n := s.src, len(s.src)
	j := i + 1
	if j < n && (src[j] == '/' || src[j] == '*') {
		return 0, false
	}
	inClass := false
	for j < n {
		switch c := src[j]; {
		case c == '\n':
			return 0, false
		case c == '\\':
			if j+1 < n && src[j+1] == '\n' {
				return 0, false
			}
			j += 2
			continue
		case c == '[':
			inClass = true
		case c == ']':
			inClass = false
		case c == '/' && !inClass:
			j++
			for j < n && isLetter(src[j]) {
				j++
			}
			return j, true
		}
		j++
	}
	return 0, false
}

// scanIdent scans an identifier or number at i. It stops where a string
// opener such as r" or $' begins.
func (s *scanner) scanIdent(i int) int {
	src, n := s.src, len(s.src)
	num := isDigit(src[i])
	j := i + 1
	for j < n {
		c := src[j]
		if isIdentByte(c) {
			if s.first[c] {
				if _, ok := s.matchOpener(j); ok {
					break
				}
			}
			j++
			continue
		}
		if num {
			if c == '.' && j+1 < n && isDigit(src[j+1]) {
				j++
				continue
			}
			if p := src[j-1] | 0x20; (c == '+' || c == '-') && (p == 'e' || p == 'p') && j+1 < n && isDigit(src[j+1]) {
				j++
				continue
			}
		}
		break
	}
	return j
}

// scanPunct scans an operator (by maximal munch) or a single punctuation
// byte at i, never swallowing the start of a comment or string.
func (s *scanner) scanPunct(i int) int {
	src := s.src
	l := 1
	for _, op := range operators {
		if hasPrefixAt(src, i, op) {
			l = len(op)
			break
		}
	}
	for k := 1; k < l; k++ {
		if s.first[src[i+k]] {
			if _, ok := s.matchOpener(i + k); ok {
				l = k
				break
			}
		}
	}
	return i + l
}

// parseHeredoc parses a here-document introducer h at i, returning the end of
// the introducer and the delimiter word.
func (s *scanner) parseHeredoc(i int, h Heredoc) (int, string, bool) {
	src, n := s.src, len(s.src)
	j := i + len(h.Open)
	if h.AllowSpace {
		for j < n && (src[j] == ' ' || src[j] == '\t') {
			j++
		}
	}
	if j >= n {
		return 0, "", false
	}
	switch q := src[j]; q {
	case '\'', '"':
		k := j + 1
		for k < n && src[k] != q && src[k] != '\n' {
			k++
		}
		if k >= n || src[k] != q || k == j+1 {
			return 0, "", false
		}
		return k + 1, string(src[j+1 : k]), true
	case '\\':
		j++
	}
	if j >= n || !(isLetter(src[j]) || src[j] == '_') {
		return 0, "", false
	}
	k := j
	for k < n && (isLetter(src[k]) || isDigit(src[k]) || src[k] == '_') {
		k++
	}
	return k, string(src[j:k]), true
}

// heredocEnd returns the end (exclusive, before the newline) of the terminator
// line of the heredoc body starting at i, or len(src) if there is none.
func (s *scanner) heredocEnd(i int, h pendingHeredoc) int {
	src, n := s.src, len(s.src)
	for j := i; j < n; {
		e := lineEnd(src, j)
		line := src[j:e]
		if h.indented {
			line = bytes.TrimLeft(line, " \t")
		}
		if string(line) == h.word {
			return e
		}
		if h.loose && bytes.HasPrefix(line, []byte(h.word)) && !isIdentByte(line[len(h.word)]) {
			return e
		}
		j = e + 1
	}
	return n
}

// applyPreamble turns the comments immediately preceding the token sequence
// lang.PreambleBefore into code (e.g. the cgo preamble before import "C").
func (s *scanner) applyPreamble() {
	seq := s.lang.PreambleBefore
	if len(seq) == 0 {
		return
	}
	var code []int
	for k, t := range s.out {
		if t.kind != tkComment && t.kind != tkIndent {
			code = append(code, k)
		}
	}
	for p := 0; p+len(seq) <= len(code); p++ {
		matched := true
		for q := range seq {
			if s.out[code[p+q]].text != seq[q] {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		for k := code[p] - 1; k >= 0 && (s.out[k].kind == tkComment || s.out[k].kind == tkIndent); k-- {
			if s.out[k].kind == tkComment {
				s.out[k].kind = tkString
			}
		}
	}
}
