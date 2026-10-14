package semantic

import (
	"bytes"
	"path"
	"slices"
	"strings"
)

// Language describes how to tokenize a file type for comparison. A single
// generic scanner (see Tokens) is driven by these fields.
type Language struct {
	Name          string
	LineComments  []string    // e.g. "//", "#", "--", ";"
	BlockComments [][2]string // e.g. {"/*", "*/"}, {"<!--", "-->"}; nesting is not supported
	Strings       []StringDelim
	// WhitespaceSignificant languages (Python, YAML, Makefile, Haskell) keep
	// each logical line's leading indentation as a token, so indentation
	// changes are Logic; trailing whitespace and blank lines still are not.
	WhitespaceSignificant bool

	// LineCommentNeedsSpace means a line comment marker only starts a comment
	// at the start of a line or after whitespace (shell "${#a[@]}" and "$#",
	// YAML "a#b", Makefile "\#").
	LineCommentNeedsSpace bool
	// CharLiterals enables single-quoted character literals ('a', '\n',
	// '"'); a quote that does not form one (Rust lifetimes, C++ digit
	// separators) is punctuation.
	CharLiterals bool
	// CodeEscapes means a backslash outside strings escapes the next
	// character (shell "\#", "\"", Lisp "#\;", line continuations).
	CodeEscapes bool
	// RegexLiterals enables JavaScript-style /regex/ literals.
	RegexLiterals bool
	// Heredocs lists here-document introducers.
	Heredocs []Heredoc
	// Directives are comment prefixes (matched case-insensitively against
	// the whole comment, marker included) that make a comment significant:
	// compiler directives and magic comments such as "//go:build" or
	// "# frozen_string_literal:". Such comments are compared as code.
	Directives []string
	// PreambleBefore is a sequence of code tokens; comments immediately
	// preceding it are compared as code (the cgo preamble before import "C").
	PreambleBefore []string
	// Markup marks markup, style and data formats (HTML, CSS, SVG, JSON,
	// YAML, TOML), which count as documentation inside a docs directory.
	Markup bool
}

// StringDelim describes a string literal so that comment markers inside it
// are not treated as comments.
type StringDelim struct {
	Open, Close string
	Escape      byte // 0 for none (e.g. Go raw strings `...`), usually '\\'
	// Multiline strings may span lines (and run to end of file when
	// unterminated); an unterminated single-line string is treated as a
	// stray quote character.
	Multiline bool
}

// Heredoc describes a here-document introducer such as "<<" or "<<-". The
// body runs from the line after the introducer to a line equal to the
// delimiter word, and is compared verbatim.
type Heredoc struct {
	Open        string // e.g. "<<", "<<-", "<<~", "<<<"
	AllowSpace  bool   // whitespace may separate Open and the delimiter word
	IndentedEnd bool   // the terminating line may be indented
	LooseEnd    bool   // the terminating line may continue after the word (PHP "EOT;")
}

// clone returns a deep copy of l, so callers cannot modify the table.
func (l Language) clone() Language {
	c := l
	c.LineComments = slices.Clone(l.LineComments)
	c.BlockComments = slices.Clone(l.BlockComments)
	c.Strings = slices.Clone(l.Strings)
	c.Heredocs = slices.Clone(l.Heredocs)
	c.Directives = slices.Clone(l.Directives)
	c.PreambleBefore = slices.Clone(l.PreambleBefore)
	return c
}

var (
	dq      = StringDelim{Open: `"`, Close: `"`, Escape: '\\'}
	dqML    = StringDelim{Open: `"`, Close: `"`, Escape: '\\', Multiline: true}
	dqRawML = StringDelim{Open: `"`, Close: `"`, Multiline: true}
	sq      = StringDelim{Open: `'`, Close: `'`, Escape: '\\'}
	sqML    = StringDelim{Open: `'`, Close: `'`, Escape: '\\', Multiline: true}
	sqRaw   = StringDelim{Open: `'`, Close: `'`}
	sqRawML = StringDelim{Open: `'`, Close: `'`, Multiline: true}
	tdq     = StringDelim{Open: `"""`, Close: `"""`, Escape: '\\', Multiline: true}
	tsq     = StringDelim{Open: `'''`, Close: `'''`, Escape: '\\', Multiline: true}
	tdqRaw  = StringDelim{Open: `"""`, Close: `"""`, Multiline: true}
	tsqRaw  = StringDelim{Open: `'''`, Close: `'''`, Multiline: true}
	btML    = StringDelim{Open: "`", Close: "`", Escape: '\\', Multiline: true}
	btRawML = StringDelim{Open: "`", Close: "`", Multiline: true}
	btRaw   = StringDelim{Open: "`", Close: "`"}
	cdata   = StringDelim{Open: "<![CDATA[", Close: "]]>", Multiline: true}

	slashes  = []string{"//"}
	hash     = []string{"#"}
	dashes   = []string{"--"}
	cBlock   = [][2]string{{"/*", "*/"}}
	xmlBlock = [][2]string{{"<!--", "-->"}}

	shellHeredocs = []Heredoc{{Open: "<<-", AllowSpace: true, IndentedEnd: true}, {Open: "<<", AllowSpace: true}}
)

func strs(d ...StringDelim) []StringDelim { return d }

type langEntry struct {
	lang  Language
	exts  []string // lower-case, including the dot
	names []string // exact basenames
}

var languageTable = []langEntry{
	{Language{
		Name: "Go", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(dq, btRawML), CharLiterals: true,
		Directives: []string{
			"//go:", "//+build", "// +build", "//line ", "/*line ",
			"//export ", "//extern ",
		},
		PreambleBefore: []string{"import", `"C"`},
	}, []string{".go"}, nil},
	{Language{
		Name: "Go module", LineComments: slashes, Strings: strs(dq, btRawML),
	}, nil, []string{"go.mod", "go.work"}},
	{Language{
		Name: "C", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(dq), CharLiterals: true,
	}, []string{".c", ".h"}, nil},
	{Language{
		Name: "C++", LineComments: slashes, BlockComments: cBlock,
		Strings:      strs(dq, StringDelim{Open: `R"(`, Close: `)"`, Multiline: true}),
		CharLiterals: true,
	}, []string{".cc", ".cpp", ".cxx", ".c++", ".hpp", ".hh", ".hxx", ".h++", ".ipp", ".inl", ".tpp", ".cu", ".cuh", ".ino"}, nil},
	{Language{
		Name: "Objective-C", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(dq), CharLiterals: true,
	}, []string{".m", ".mm"}, nil},
	{Language{
		Name: "Java", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(tdq, dq), CharLiterals: true,
	}, []string{".java"}, nil},
	{Language{
		Name: "Kotlin", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(tdqRaw, dq), CharLiterals: true,
	}, []string{".kt", ".kts"}, nil},
	{Language{
		Name: "Scala", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(tdqRaw, dq), CharLiterals: true,
	}, []string{".scala", ".sc", ".sbt"}, nil},
	{Language{
		Name: "Swift", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(
			StringDelim{Open: `#"""`, Close: `"""#`, Multiline: true},
			StringDelim{Open: `#"`, Close: `"#`, Multiline: true},
			tdq, dq,
		),
	}, []string{".swift"}, nil},
	{Language{
		Name: "C#", LineComments: slashes, BlockComments: cBlock,
		Strings:      strs(tdqRaw, StringDelim{Open: `@"`, Close: `"`, Multiline: true}, dq),
		CharLiterals: true,
	}, []string{".cs", ".csx"}, nil},
	{Language{
		Name: "Rust", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(
			dqML,
			StringDelim{Open: `r"`, Close: `"`, Multiline: true},
			StringDelim{Open: `r#"`, Close: `"#`, Multiline: true},
			StringDelim{Open: `r##"`, Close: `"##`, Multiline: true},
			StringDelim{Open: `br"`, Close: `"`, Multiline: true},
			StringDelim{Open: `br#"`, Close: `"#`, Multiline: true},
			StringDelim{Open: `br##"`, Close: `"##`, Multiline: true},
		),
		CharLiterals: true,
	}, []string{".rs"}, nil},
	{Language{
		Name: "JavaScript", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(dq, sq, btML), RegexLiterals: true,
	}, []string{".js", ".mjs", ".cjs", ".jsx"}, nil},
	{Language{
		Name: "TypeScript", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(dq, sq, btML), RegexLiterals: true,
		Directives: []string{"/// <reference", "///<reference", "/// <amd-", "///<amd-"},
	}, []string{".ts", ".tsx", ".mts", ".cts"}, nil},
	{Language{
		Name: "Dart", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(
			StringDelim{Open: `r"""`, Close: `"""`, Multiline: true},
			StringDelim{Open: `r'''`, Close: `'''`, Multiline: true},
			StringDelim{Open: `r"`, Close: `"`},
			StringDelim{Open: `r'`, Close: `'`},
			tdq, tsq, dq, sq,
		),
	}, []string{".dart"}, nil},
	{Language{
		// "#" comments are deliberately not recognised: "#[...]" is a PHP 8
		// attribute.
		Name: "PHP", LineComments: slashes, BlockComments: cBlock,
		Strings:  strs(dqML, sqML, btML),
		Heredocs: []Heredoc{{Open: "<<<", AllowSpace: true, IndentedEnd: true, LooseEnd: true}},
	}, []string{".php", ".phtml"}, nil},
	{Language{
		Name: "Groovy", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(tdq, tsq, dq, sq),
	}, []string{".groovy", ".gradle", ".gvy"}, []string{"Jenkinsfile"}},
	{Language{
		Name: "Protocol Buffers", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(dq, sq),
	}, []string{".proto"}, nil},
	{Language{
		Name: "Solidity", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(dq, sq),
	}, []string{".sol"}, nil},
	{Language{
		Name: "CSS", BlockComments: cBlock, Strings: strs(dq, sq), Markup: true,
	}, []string{".css"}, nil},
	{Language{
		Name: "SCSS", LineComments: slashes, LineCommentNeedsSpace: true,
		BlockComments: cBlock, Strings: strs(dq, sq), Markup: true,
	}, []string{".scss"}, nil},
	{Language{
		Name: "Less", LineComments: slashes, LineCommentNeedsSpace: true,
		BlockComments: cBlock, Strings: strs(dq, sq), Markup: true,
	}, []string{".less"}, nil},
	{Language{
		Name: "JSON", Strings: strs(dq), Markup: true,
	}, []string{".json", ".geojson", ".ipynb", ".webmanifest"}, nil},
	{Language{
		Name: "JSON with comments", LineComments: slashes, BlockComments: cBlock,
		Strings: strs(dq), Markup: true,
	}, []string{".jsonc", ".json5"}, nil},
	{Language{
		Name: "HTML", BlockComments: xmlBlock, Strings: strs(cdata), Markup: true,
	}, []string{".html", ".htm", ".xhtml"}, nil},
	{Language{
		Name: "XML", BlockComments: xmlBlock, Strings: strs(cdata), Markup: true,
	}, []string{
		".xml", ".xsd", ".xsl", ".xslt", ".svg", ".plist", ".csproj", ".fsproj",
		".vbproj", ".props", ".targets", ".xaml", ".resx", ".wsdl", ".rss",
	}, nil},
	{Language{
		Name: "Vue", BlockComments: xmlBlock, Strings: strs(cdata),
	}, []string{".vue"}, nil},
	{Language{
		Name: "Svelte", BlockComments: xmlBlock, Strings: strs(cdata),
	}, []string{".svelte"}, nil},
	{Language{
		Name: "Python", LineComments: hash, Strings: strs(tdq, tsq, dq, sq),
		WhitespaceSignificant: true,
		Directives:            []string{"# -*-", "#-*-", "# coding:", "# coding=", "#coding:", "#coding="},
	}, []string{".py", ".pyi", ".pyw"}, []string{"SConstruct", "SConscript"}},
	{Language{
		Name: "Starlark", LineComments: hash, Strings: strs(tdq, tsq, dq, sq),
		WhitespaceSignificant: true,
	}, []string{".bzl", ".star", ".bazel"}, []string{"BUILD", "WORKSPACE"}},
	{Language{
		Name: "Ruby", LineComments: hash, LineCommentNeedsSpace: true,
		Strings: strs(dqML, sqML),
		Heredocs: []Heredoc{
			{Open: "<<~", IndentedEnd: true}, {Open: "<<-", IndentedEnd: true}, {Open: "<<"},
		},
		Directives: []string{
			"# frozen_string_literal:", "#frozen_string_literal:", "# encoding:",
			"#encoding:", "# coding:", "#coding:", "# -*-", "#-*-", "# warn_indent:",
			"# shareable_constant_value:",
		},
	}, []string{".rb", ".rake", ".gemspec", ".ru"}, []string{
		"Gemfile", "Rakefile", "Vagrantfile", "Podfile", "Brewfile", "Guardfile",
		"Capfile", "Fastfile", "Dangerfile", "Berksfile", "Thorfile", ".irbrc", ".pryrc",
	}},
	{Language{
		Name: "Shell", LineComments: hash, LineCommentNeedsSpace: true, CodeEscapes: true,
		Strings:  strs(StringDelim{Open: `$'`, Close: `'`, Escape: '\\', Multiline: true}, dqML, sqRawML),
		Heredocs: shellHeredocs,
	}, []string{".sh", ".bash", ".zsh", ".ksh", ".fish", ".bats"}, []string{
		".bashrc", ".bash_profile", ".bash_login", ".bash_logout", ".bash_aliases",
		".profile", ".zshrc", ".zshenv", ".zprofile", ".zlogin", ".zlogout", ".kshrc",
		".envrc", "PKGBUILD",
	}},
	{Language{
		Name: "Perl", LineComments: hash, LineCommentNeedsSpace: true,
		Strings: strs(dqML, sqML),
		Heredocs: []Heredoc{
			{Open: "<<~", AllowSpace: true, IndentedEnd: true}, {Open: "<<", AllowSpace: true},
		},
	}, []string{".pl", ".pm"}, nil},
	{Language{
		Name: "PowerShell", LineComments: hash, BlockComments: [][2]string{{"<#", "#>"}},
		Strings: strs(
			StringDelim{Open: `@"`, Close: `"@`, Multiline: true},
			StringDelim{Open: `@'`, Close: `'@`, Multiline: true},
			StringDelim{Open: `"`, Close: `"`, Escape: '`', Multiline: true},
			sqRawML,
		),
	}, []string{".ps1", ".psm1", ".psd1"}, nil},
	{Language{
		Name: "R", LineComments: hash, Strings: strs(dqML, sqML, btRaw),
	}, []string{".r"}, []string{".Rprofile"}},
	{Language{
		Name: "Julia", LineComments: hash, BlockComments: [][2]string{{"#=", "=#"}},
		Strings: strs(tdq, dqML), CharLiterals: true,
	}, []string{".jl"}, nil},
	{Language{
		Name: "Elixir", LineComments: hash, LineCommentNeedsSpace: true,
		Strings: strs(tdq, tsq, dqML, sqML),
	}, []string{".ex", ".exs"}, nil},
	{Language{
		Name: "YAML", LineComments: hash, LineCommentNeedsSpace: true,
		Strings: strs(dq, sqRaw), WhitespaceSignificant: true, Markup: true,
	}, []string{".yml", ".yaml"}, []string{".clang-format", ".clang-tidy"}},
	{Language{
		Name: "TOML", LineComments: hash, Strings: strs(tdq, tsqRaw, dq, sqRaw), Markup: true,
	}, []string{".toml"}, []string{"Pipfile"}},
	{Language{
		Name: "Makefile", LineComments: hash, LineCommentNeedsSpace: true, CodeEscapes: true,
		Strings: strs(dq, sqRaw), WhitespaceSignificant: true,
	}, []string{".mk", ".mak"}, nil},
	{Language{
		Name: "Dockerfile", LineComments: hash, LineCommentNeedsSpace: true, CodeEscapes: true,
		Strings: strs(dq, sqRaw), Heredocs: shellHeredocs,
		Directives: []string{"# syntax=", "#syntax=", "# escape=", "#escape=", "# check=", "#check="},
	}, []string{".dockerfile"}, []string{"Dockerfile", "Containerfile"}},
	{Language{
		Name: "CMake", LineComments: hash, BlockComments: [][2]string{{"#[[", "]]"}},
		Strings: strs(dqML, StringDelim{Open: "[[", Close: "]]", Multiline: true}),
	}, []string{".cmake"}, []string{"CMakeLists.txt"}},
	{Language{
		Name: "pip requirements", LineComments: hash, LineCommentNeedsSpace: true,
	}, nil, nil},
	{Language{
		Name: "HCL", LineComments: []string{"#", "//"}, BlockComments: cBlock,
		Strings:  strs(dq),
		Heredocs: []Heredoc{{Open: "<<-", IndentedEnd: true}, {Open: "<<"}},
	}, []string{".tf", ".tfvars", ".hcl", ".nomad"}, nil},
	{Language{
		Name: "GraphQL", LineComments: hash, Strings: strs(tdq, dq),
	}, []string{".graphql", ".gql", ".graphqls"}, nil},
	{Language{
		Name: "SQL", LineComments: dashes, BlockComments: cBlock,
		Strings: strs(sqRawML, dqRawML, btRaw),
	}, []string{".sql"}, nil},
	{Language{
		Name: "Lua", LineComments: dashes, BlockComments: [][2]string{{"--[[", "]]"}},
		Strings: strs(dq, sq, StringDelim{Open: "[[", Close: "]]", Multiline: true}),
	}, []string{".lua"}, nil},
	{Language{
		Name: "Haskell", LineComments: dashes, BlockComments: [][2]string{{"{-", "-}"}},
		Strings: strs(dq), CharLiterals: true, WhitespaceSignificant: true,
	}, []string{".hs"}, nil},
	{Language{
		Name: "Elm", LineComments: dashes, BlockComments: [][2]string{{"{-", "-}"}},
		Strings: strs(tdq, dq), CharLiterals: true, WhitespaceSignificant: true,
	}, []string{".elm"}, nil},
	{Language{
		Name: "OCaml", BlockComments: [][2]string{{"(*", "*)"}},
		Strings:      strs(dqML, StringDelim{Open: "{|", Close: "|}", Multiline: true}),
		CharLiterals: true,
	}, []string{".ml", ".mli"}, nil},
	{Language{
		Name: "Clojure", LineComments: []string{";"}, Strings: strs(dqML), CodeEscapes: true,
	}, []string{".clj", ".cljs", ".cljc", ".edn"}, nil},
	{Language{
		Name: "Lisp", LineComments: []string{";"}, BlockComments: [][2]string{{"#|", "|#"}},
		Strings: strs(dqML), CodeEscapes: true,
	}, []string{".lisp", ".lsp", ".el", ".scm", ".ss", ".rkt"}, nil},
	{Language{
		Name: "TeX", LineComments: []string{"%"}, CodeEscapes: true,
	}, []string{".tex", ".sty", ".ltx"}, nil},
}

var (
	byExt      = map[string]*Language{}
	byBasename = map[string]*Language{}
	byName     = map[string]*Language{}
)

func init() {
	for i := range languageTable {
		e := &languageTable[i]
		if byName[e.lang.Name] != nil {
			panic("semantic: duplicate language " + e.lang.Name)
		}
		byName[e.lang.Name] = &e.lang
		for _, x := range e.exts {
			if byExt[x] != nil {
				panic("semantic: duplicate extension " + x)
			}
			byExt[x] = &e.lang
		}
		for _, b := range e.names {
			if byBasename[b] != nil {
				panic("semantic: duplicate basename " + b)
			}
			byBasename[b] = &e.lang
		}
	}
}

// LanguageFor picks a language by file path: by well-known basename (Makefile,
// Dockerfile, BUILD, .bashrc, CMakeLists.txt, requirements.txt, ...) and
// otherwise by extension. It returns ok=false for unknown types, including
// documentation formats such as Markdown.
func LanguageFor(p string) (Language, bool) {
	p = strings.ReplaceAll(p, `\`, "/")
	base := path.Base(p)
	if base == "." || base == "/" || base == "" {
		return Language{}, false
	}
	if l := byBasename[base]; l != nil {
		return l.clone(), true
	}
	lower := strings.ToLower(base)
	ext := path.Ext(lower)
	name := ""
	switch {
	case docExts[ext] && ext != ".txt":
		return Language{}, false
	case lower == "makefile" || lower == "gnumakefile" || strings.HasPrefix(lower, "makefile."):
		if !docExts[ext] {
			name = "Makefile"
		}
	case strings.HasPrefix(lower, "dockerfile.") || strings.HasPrefix(lower, "containerfile."):
		if !docExts[ext] {
			name = "Dockerfile"
		}
	case ext == ".txt" || ext == ".in":
		if strings.HasPrefix(lower, "requirements") || strings.HasPrefix(lower, "constraints") ||
			(ext == ".txt" && path.Base(path.Dir(p)) == "requirements") {
			name = "pip requirements"
		}
	}
	if name != "" {
		return byName[name].clone(), true
	}
	if l := byExt[ext]; l != nil {
		return l.clone(), true
	}
	return Language{}, false
}

// interpreters maps #! interpreter names (with any version suffix removed) to
// language names.
var interpreters = map[string]string{
	"sh": "Shell", "bash": "Shell", "zsh": "Shell", "ksh": "Shell", "mksh": "Shell",
	"dash": "Shell", "ash": "Shell", "fish": "Shell",
	"python": "Python", "pypy": "Python",
	"ruby": "Ruby", "perl": "Perl",
	"node": "JavaScript", "nodejs": "JavaScript", "deno": "TypeScript", "bun": "TypeScript",
	"ts-node": "TypeScript", "tsx": "TypeScript",
	"lua": "Lua", "Rscript": "R", "php": "PHP", "make": "Makefile",
	"pwsh": "PowerShell", "julia": "Julia", "elixir": "Elixir",
}

// shebangLanguage recognises the language of an extensionless script by its
// #! line, e.g. "#!/usr/bin/env python3".
func shebangLanguage(content []byte) (Language, bool) {
	if !bytes.HasPrefix(content, []byte("#!")) {
		return Language{}, false
	}
	line := content[2:lineEnd(content, 0)]
	fields := strings.Fields(string(line))
	if len(fields) == 0 {
		return Language{}, false
	}
	interp := path.Base(fields[0])
	if interp == "env" {
		interp = ""
		for _, a := range fields[1:] {
			if strings.HasPrefix(a, "-") || strings.Contains(a, "=") {
				continue
			}
			interp = path.Base(a)
			break
		}
	}
	interp = strings.TrimRight(interp, "0123456789.")
	if name, ok := interpreters[interp]; ok {
		return byName[name].clone(), true
	}
	return Language{}, false
}
