// Package semantic classifies the difference between two versions of a file
// so that review tooling can tell consequential updates (logic changes) from
// trivial ones (whitespace, comments, documentation).
//
// The classification is deliberately conservative: whenever the scanner cannot
// be sure that a difference is confined to whitespace or comments, the change
// is reported as Logic.
package semantic

import (
	"bytes"
	"path"
	"strconv"
	"strings"
)

// Class is the classification of a change to a single file, ordered from
// least to most consequential.
type Class int

const (
	// Identical means the two versions are byte-identical.
	Identical Class = iota
	// WhitespaceOnly means the versions differ only in whitespace (blank
	// lines, indentation, trailing spaces, line endings).
	WhitespaceOnly
	// CommentOnly means the versions differ only in comments (and possibly
	// whitespace); the code tokens are identical.
	CommentOnly
	// DocsOnly means the file is documentation and it changed.
	DocsOnly
	// Logic is anything else: code tokens changed, binary content changed, an
	// unknown file type changed, or the file was added or deleted.
	Logic
)

// String returns "identical", "whitespace", "comments", "docs" or "logic".
func (c Class) String() string {
	switch c {
	case Identical:
		return "identical"
	case WhitespaceOnly:
		return "whitespace"
	case CommentOnly:
		return "comments"
	case DocsOnly:
		return "docs"
	case Logic:
		return "logic"
	}
	return "Class(" + strconv.Itoa(int(c)) + ")"
}

// Combine returns the most consequential of the classes, or Identical when
// called with none.
func Combine(classes ...Class) Class {
	out := Identical
	for _, c := range classes {
		if c > out {
			out = c
		}
	}
	return out
}

// binarySniffLen is how many leading bytes are inspected for a NUL byte.
const binarySniffLen = 8000

func isBinary(b []byte) bool {
	if len(b) > binarySniffLen {
		b = b[:binarySniffLen]
	}
	return bytes.IndexByte(b, 0) >= 0
}

// Classify classifies the change from old to new content for a file at path.
//
// oldExists and newExists say whether the file exists on each side (so an
// empty file is distinguishable from an absent one). An added or deleted file
// is Logic, or DocsOnly when the path is a documentation path.
//
// Otherwise the rules are, in order:
//  1. byte-identical content is Identical;
//  2. a documentation path (see IsDocPath) is WhitespaceOnly when only
//     whitespace differs, else DocsOnly;
//  3. binary content (a NUL byte in the first 8000 bytes of either side) is
//     Logic;
//  4. for a known language (see LanguageFor; extensionless scripts are also
//     recognised by their #! line) the content is tokenized: identical token
//     and comment streams mean WhitespaceOnly, identical code tokens mean
//     CommentOnly, anything else is Logic. Whitespace inside string literals,
//     whitespace that separates tokens, and indentation in whitespace-
//     significant languages are all significant;
//  5. for an unknown text file the change is WhitespaceOnly when the files
//     agree line by line after normalising line endings, dropping blank lines,
//     stripping trailing whitespace and collapsing runs of inner whitespace
//     (leading indentation and line structure are kept); otherwise Logic.
func Classify(path string, old, new []byte, oldExists, newExists bool) Class {
	if !oldExists && !newExists {
		return Identical
	}
	if !oldExists || !newExists {
		if IsDocPath(path) {
			return DocsOnly
		}
		return Logic
	}
	if bytes.Equal(old, new) {
		return Identical
	}
	binary := isBinary(old) || isBinary(new)
	if IsDocPath(path) {
		if !binary && equalFields(old, new) {
			return WhitespaceOnly
		}
		return DocsOnly
	}
	if binary {
		return Logic
	}
	lang, ok := LanguageFor(path)
	if !ok {
		lang, ok = shebangPair(old, new)
	}
	if !ok {
		if equalLines(old, new) {
			return WhitespaceOnly
		}
		return Logic
	}
	a, b := scan(lang, old), scan(lang, new)
	switch {
	case sameStream(a, b, true):
		return WhitespaceOnly
	case sameStream(a, b, false):
		return CommentOnly
	}
	return Logic
}

// shebangPair returns the language named by the #! line of both versions,
// provided they agree.
func shebangPair(old, new []byte) (Language, bool) {
	la, oka := shebangLanguage(old)
	lb, okb := shebangLanguage(new)
	if oka && okb && la.Name == lb.Name {
		return la, true
	}
	return Language{}, false
}

// sameStream compares two token streams. Code tokens must match exactly.
// When withComments is set, comments must also match (ignoring whitespace
// inside them) and appear in the same order relative to the code; otherwise
// comments are ignored.
func sameStream(a, b []token, withComments bool) bool {
	i, j := 0, 0
	for {
		if !withComments {
			for i < len(a) && a[i].kind == tkComment {
				i++
			}
			for j < len(b) && b[j].kind == tkComment {
				j++
			}
		}
		if i == len(a) || j == len(b) {
			return i == len(a) && j == len(b)
		}
		x, y := a[i], b[j]
		if (x.kind == tkComment) != (y.kind == tkComment) {
			return false
		}
		if x.kind == tkComment {
			if squashSpace(x.text) != squashSpace(y.text) {
				return false
			}
		} else if x.text != y.text {
			return false
		}
		i++
		j++
	}
}

func isSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

func isSpaceRune(r rune) bool { return r < 0x80 && isSpace(byte(r)) }

// squashSpace removes all ASCII whitespace from s.
func squashSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if isSpaceRune(r) {
			return -1
		}
		return r
	}, s)
}

// equalFields reports whether a and b contain the same sequence of
// whitespace-separated words.
func equalFields(a, b []byte) bool {
	fa, fb := bytes.FieldsFunc(a, isSpaceRune), bytes.FieldsFunc(b, isSpaceRune)
	if len(fa) != len(fb) {
		return false
	}
	for i := range fa {
		if !bytes.Equal(fa[i], fb[i]) {
			return false
		}
	}
	return true
}

// normLines returns the non-blank lines of b with line endings normalised,
// trailing whitespace stripped and inner whitespace runs collapsed; leading
// indentation is kept verbatim.
func normLines(b []byte) []string {
	var out []string
	for _, line := range bytes.Split(b, []byte("\n")) {
		f := bytes.FieldsFunc(line, isSpaceRune)
		if len(f) == 0 {
			continue
		}
		indent := line[:len(line)-len(bytes.TrimLeft(line, " \t\f\v\r"))]
		out = append(out, string(indent)+string(bytes.Join(f, []byte(" "))))
	}
	return out
}

func equalLines(a, b []byte) bool {
	la, lb := normLines(a), normLines(b)
	if len(la) != len(lb) {
		return false
	}
	for i := range la {
		if la[i] != lb[i] {
			return false
		}
	}
	return true
}

// docExts are the extensions of documentation files.
var docExts = map[string]bool{
	".md":       true,
	".markdown": true,
	".rst":      true,
	".txt":      true,
	".adoc":     true,
	".asciidoc": true,
}

// docBaseNames are basename stems (upper-case) of documentation files, such
// as README, README.md, README_zh.md or LICENSE-MIT.
var docBaseNames = []string{
	"README", "LICENSE", "LICENCE", "CHANGELOG", "CHANGES", "CONTRIBUTING",
	"CONTRIBUTORS", "NOTICE", "AUTHORS", "COPYING",
}

// neverDocs are basenames that control review or ownership and are therefore
// never documentation, even inside a docs directory.
var neverDocs = map[string]bool{
	"CODEOWNERS":     true,
	"OWNERS":         true,
	"OWNERS_ALIASES": true,
}

// IsDocPath reports whether a path is documentation: an extension of .md,
// .markdown, .rst, .txt, .adoc or .asciidoc; a basename of README*, LICENSE*,
// CHANGELOG*, CONTRIBUTING*, NOTICE* (and a few similar); or any path
// component equal to "docs" or "doc" (case-insensitive).
//
// Source files of a recognised programming language (for example
// docs/conf.py, internal/doc/doc.go, CMakeLists.txt or requirements.txt) are
// never documentation, and neither are ownership files such as CODEOWNERS.
// Markup, style and data files (HTML, CSS, SVG, JSON, YAML, TOML) inside a
// docs directory are documentation.
func IsDocPath(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	base := path.Base(p)
	if neverDocs[base] {
		return false
	}
	if lang, ok := LanguageFor(p); ok && !lang.Markup {
		return false
	}
	if docExts[strings.ToLower(path.Ext(base))] {
		return true
	}
	upper := strings.ToUpper(base)
	for _, name := range docBaseNames {
		if upper == name {
			return true
		}
		if strings.HasPrefix(upper, name) {
			switch upper[len(name)] {
			case '.', '-', '_':
				return true
			}
		}
	}
	for _, comp := range strings.Split(p, "/") {
		if strings.EqualFold(comp, "docs") || strings.EqualFold(comp, "doc") {
			return true
		}
	}
	return false
}
