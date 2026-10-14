package semantic

import (
	"reflect"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		old, new string
		added    bool // old does not exist
		deleted  bool // new does not exist
		want     Class
	}{
		// Identical / existence.
		{name: "identical", path: "main.go", old: "package main\n", new: "package main\n", want: Identical},
		{name: "identical empty", path: "x.go", old: "", new: "", want: Identical},
		{name: "added code", path: "main.go", new: "package main\n", added: true, want: Logic},
		{name: "added empty file", path: "empty.go", new: "", added: true, want: Logic},
		{name: "deleted code", path: "main.go", old: "package main\n", deleted: true, want: Logic},
		{name: "added doc", path: "docs/intro.md", new: "# Hi\n", added: true, want: DocsOnly},
		{name: "deleted doc", path: "README", old: "hello\n", deleted: true, want: DocsOnly},
		{name: "added unknown", path: "data.bin", new: "x", added: true, want: Logic},

		// Go.
		{name: "go reindent", path: "a.go",
			old: "func f() {\n\treturn 1\n}\n", new: "func f() {\n        return 1\n}\n\n\n", want: WhitespaceOnly},
		{name: "go spaces around operators", path: "a.go",
			old: "x := a+b\ny = -1\n", new: "x  :=  a + b\ny=-1\n", want: WhitespaceOnly},
		{name: "go crlf vs lf", path: "a.go",
			old: "package a\n\n// c\nfunc f() {}\n", new: "package a\r\n\r\n// c\r\nfunc f() {}\r\n", want: WhitespaceOnly},
		{name: "go crlf in raw string", path: "a.go",
			old: "var s = `a\nb`\n", new: "var s = `a\r\nb`\r\n", want: WhitespaceOnly},
		{name: "go whitespace inside comment", path: "a.go",
			old: "// hello world\nvar x = 1\n", new: "//   hello   world\nvar x = 1\n", want: WhitespaceOnly},
		{name: "go comment-only change", path: "a.go",
			old: "package a\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n",
			new: "package a\n\n// Add returns the sum of a and b.\nfunc Add(a, b int) int { return a + b }\n", want: CommentOnly},
		{name: "go comment added", path: "a.go",
			old: "x := 1\n", new: "x := 1 // one\n", want: CommentOnly},
		{name: "go block comment spanning lines", path: "a.go",
			old: "/*\n * old text\n */\nvar x = 1\n", new: "/*\n * new text\n * more\n */\nvar x = 1\n", want: CommentOnly},
		{name: "go comment moved relative to code", path: "a.go",
			old: "// c\nx := 1\ny := 2\n", new: "x := 1\n// c\ny := 2\n", want: CommentOnly},
		{name: "go logic change", path: "a.go",
			old: "return a + b\n", new: "return a - b\n", want: Logic},
		{name: "go string containing // changed", path: "a.go",
			old: "u := \"http://example.com\"\n", new: "u := \"http://example.org\"\n", want: Logic},
		{name: "go comment after string containing //", path: "a.go",
			old: "u := \"http://x\" // first\n", new: "u := \"http://x\" // second\n", want: CommentOnly},
		{name: "go string containing /* changed", path: "a.go",
			old: "s := \"/* a */\"\n", new: "s := \"/* b */\"\n", want: Logic},
		{name: "go escaped quote in string", path: "a.go",
			old: "s := \"\\\"// a\"\n", new: "s := \"\\\"// b\"\n", want: Logic},
		{name: "go raw string containing //", path: "a.go",
			old: "s := `// a\n/* x`\n", new: "s := `// b\n/* x`\n", want: Logic},
		{name: "go rune quote then string", path: "a.go",
			old: "c := '\"'; s := \"// a\"\n", new: "c := '\"'; s := \"// b\"\n", want: Logic},
		{name: "go whitespace inside string", path: "a.go",
			old: "s := strings.Split(x, \" \")\n", new: "s := strings.Split(x, \"\")\n", want: Logic},
		{name: "go joined identifiers", path: "a.go",
			old: "var x int\n", new: "var xint\n", want: Logic},
		{name: "go receive vs less-than-negative", path: "a.go",
			old: "ok := a < -b\n", new: "ok := a <- b\n", want: Logic},
		{name: "go build constraint", path: "a.go",
			old: "//go:build linux\n\npackage a\n", new: "//go:build windows\n\npackage a\n", want: Logic},
		{name: "go embed directive", path: "a.go",
			old: "//go:embed a.txt\nvar s string\n", new: "//go:embed b.txt\nvar s string\n", want: Logic},
		{name: "cgo preamble", path: "a.go",
			old: "package a\n\n// #include <stdio.h>\n// int x = 1;\nimport \"C\"\n",
			new: "package a\n\n// #include <stdio.h>\n// int x = 2;\nimport \"C\"\n", want: Logic},
		{name: "go comment not before import C", path: "a.go",
			old: "package a\n\n// doc one\nimport \"fmt\"\n", new: "package a\n\n// doc two\nimport \"fmt\"\n", want: CommentOnly},

		// C family.
		{name: "c char literal slash", path: "a.c",
			old: "char c = '/'; char *s = \"//a\";\n", new: "char c = '/'; char *s = \"//b\";\n", want: Logic},
		{name: "c escaped char literal", path: "a.c",
			old: "char c = '\\''; /* x */ int y = 1;\n", new: "char c = '\\''; /* z */ int y = 1;\n", want: CommentOnly},
		{name: "c++ raw string", path: "a.cpp",
			old: "auto s = R\"(// a \")\";\n", new: "auto s = R\"(// b \")\";\n", want: Logic},
		{name: "c++ digit separators", path: "a.cpp",
			old: "int x = 1'000; // a\n", new: "int x = 1'000; // b\n", want: CommentOnly},
		{name: "rust lifetimes then string", path: "a.rs",
			old: "fn f<'a>(x: &'a str) -> &'a str { \"// a\" }\n", new: "fn f<'a>(x: &'a str) -> &'a str { \"// b\" }\n", want: Logic},
		{name: "rust doc comment", path: "a.rs",
			old: "/// Old doc.\nfn f() {}\n", new: "/// New doc.\nfn f() {}\n", want: CommentOnly},
		{name: "rust raw string", path: "a.rs",
			old: "let s = r#\"a \" // x\"#;\n", new: "let s = r#\"a \" // y\"#;\n", want: Logic},
		{name: "rust inner attribute is code", path: "a.rs",
			old: "#![allow(dead_code)]\nfn f() {}\n", new: "#![deny(dead_code)]\nfn f() {}\n", want: Logic},
		{name: "java text block", path: "A.java",
			old: "String s = \"\"\"\n  // a\n  \"\"\";\n", new: "String s = \"\"\"\n  // b\n  \"\"\";\n", want: Logic},

		// JavaScript / TypeScript.
		{name: "js regex containing //", path: "a.js",
			old: "const re = /https?:\\/\\//.test(a);\n", new: "const re = /https?:\\/\\//.test(b);\n", want: Logic},
		{name: "js regex char class", path: "a.js",
			old: "x = s.replace(/[/*]/g, '') // a\n", new: "x = s.replace(/[/*]/g, '') // b\n", want: CommentOnly},
		{name: "js division is not regex", path: "a.js",
			old: "x = a / b / c // one\n", new: "x = a / b / c // two\n", want: CommentOnly},
		{name: "js template literal", path: "a.js",
			old: "s = `line1\n// a ${x}`\n", new: "s = `line1\n// b ${x}`\n", want: Logic},
		{name: "ts reference directive", path: "a.ts",
			old: "/// <reference path=\"a.d.ts\" />\n", new: "/// <reference path=\"b.d.ts\" />\n", want: Logic},

		// Python.
		{name: "python indentation change", path: "a.py",
			old: "if x:\n    y()\nz()\n", new: "if x:\n    y()\n    z()\n", want: Logic},
		{name: "python tabs vs spaces", path: "a.py",
			old: "if x:\n    y()\n", new: "if x:\n\ty()\n", want: Logic},
		{name: "python trailing whitespace", path: "a.py",
			old: "if x:\n    y()\n", new: "if x:   \n    y()\t\n", want: WhitespaceOnly},
		{name: "python blank lines and crlf", path: "a.py",
			old: "import os\ndef f():\n    pass\n", new: "import os\r\n\r\n\r\ndef f():\r\n    pass\r\n", want: WhitespaceOnly},
		{name: "python inner spaces", path: "a.py",
			old: "x = f(a,b)\n", new: "x = f( a, b )\n", want: WhitespaceOnly},
		{name: "python comment change", path: "a.py",
			old: "x = 1  # one\n", new: "x = 1  # uno\n", want: CommentOnly},
		{name: "python comment reindented", path: "a.py",
			old: "def f():\n    # c\n    pass\n", new: "def f():\n# c\n    pass\n", want: WhitespaceOnly},
		{name: "python hash in string", path: "a.py",
			old: "x = '#'\n", new: "x = '# '\n", want: Logic},
		{name: "python hash in string then comment", path: "a.py",
			old: "x = '#'  # a\n", new: "x = '#'  # b\n", want: CommentOnly},
		{name: "python docstring is a string", path: "a.py",
			old: "def f():\n    \"\"\"Old doc.\"\"\"\n", new: "def f():\n    \"\"\"New doc.\"\"\"\n", want: Logic},
		{name: "python multiline string continuation indent", path: "a.py",
			old: "x = '''a\n  # b\n'''\n", new: "x = '''a\n  # c\n'''\n", want: Logic},
		{name: "python shebang", path: "a.py",
			old: "#!/usr/bin/env python2\nprint(1)\n", new: "#!/usr/bin/env python3\nprint(1)\n", want: Logic},
		{name: "python coding declaration", path: "a.py",
			old: "# -*- coding: latin-1 -*-\nx = 1\n", new: "# -*- coding: utf-8 -*-\nx = 1\n", want: Logic},

		// Shell.
		{name: "shell array length not a comment", path: "a.sh",
			old: "n=${#arr[@]}\n", new: "n=${#other[@]}\n", want: Logic},
		{name: "shell $# not a comment", path: "a.sh",
			old: "if [ $# -gt 1 ]; then :; fi\n", new: "if [ $# -gt 2 ]; then :; fi\n", want: Logic},
		{name: "shell hash inside word", path: "a.sh",
			old: "echo foo#bar\n", new: "echo foo#baz\n", want: Logic},
		{name: "shell comment change", path: "a.sh",
			old: "echo hi # greet\n", new: "echo hi # say hello\n", want: CommentOnly},
		{name: "shell escaped quote", path: "a.sh",
			old: "echo it\\'s # a\necho '# x'\n", new: "echo it\\'s # b\necho '# x'\n", want: CommentOnly},
		{name: "shell single-quoted hash", path: "a.sh",
			old: "echo '# a'\n", new: "echo '# b'\n", want: Logic},
		{name: "shell heredoc body", path: "a.sh",
			old: "cat <<EOF > conf\n# setting a\nEOF\necho done # x\n", new: "cat <<EOF > conf\n# setting b\nEOF\necho done # x\n", want: Logic},
		{name: "shell comment after heredoc", path: "a.sh",
			old: "cat << 'EOF'\nbody\nEOF\n# a\n", new: "cat << 'EOF'\nbody\nEOF\n# b\n", want: CommentOnly},
		{name: "bashrc by basename", path: "home/.bashrc",
			old: "alias ll='ls -l' # a\n", new: "alias ll='ls -l' # b\n", want: CommentOnly},
		{name: "extensionless script via shebang", path: "bin/tool",
			old: "#!/bin/bash\n# old\nexit 0\n", new: "#!/bin/bash\n# new\nexit 0\n", want: CommentOnly},
		{name: "extensionless script logic", path: "bin/tool",
			old: "#!/bin/bash\nexit 0\n", new: "#!/bin/bash\nexit 1\n", want: Logic},

		// YAML / TOML / Makefile / Dockerfile.
		{name: "yaml indentation change", path: "ci.yml",
			old: "a:\n  b: 1\n  c: 2\n", new: "a:\n  b: 1\nc: 2\n", want: Logic},
		{name: "yaml comment", path: "ci.yaml",
			old: "a: 1 # one\n", new: "a: 1 # first\n", want: CommentOnly},
		{name: "yaml hash in plain scalar", path: "ci.yaml",
			old: "url: http://x/#a\n", new: "url: http://x/#b\n", want: Logic},
		{name: "yaml apostrophe in plain scalar", path: "ci.yaml",
			old: "msg: don't\nq: 'x # y'\n", new: "msg: don't\nq: 'x # z'\n", want: Logic},
		{name: "toml comment", path: "Cargo.toml",
			old: "[package]\nname = \"a#b\" # c1\n", new: "[package]\nname = \"a#b\" # c2\n", want: CommentOnly},
		{name: "makefile tab to spaces", path: "Makefile",
			old: "all:\n\tgo build\n", new: "all:\n    go build\n", want: Logic},
		{name: "makefile comment", path: "Makefile",
			old: "# build it\nall:\n\tgo build\n", new: "# build everything\nall:\n\tgo build\n", want: CommentOnly},
		{name: "makefile escaped hash", path: "build.mk",
			old: "X = a\\#b\n", new: "X = a\\#c\n", want: Logic},
		{name: "dockerfile comment", path: "Dockerfile",
			old: "FROM alpine\n# install\nRUN apk add git\n", new: "FROM alpine\n# install git\nRUN apk add git\n", want: CommentOnly},
		{name: "dockerfile syntax directive", path: "Dockerfile.dev",
			old: "# syntax=docker/dockerfile:1.4\nFROM a\n", new: "# syntax=docker/dockerfile:1.7\nFROM a\n", want: Logic},

		// Other languages.
		{name: "sql comment", path: "q.sql",
			old: "SELECT 1; -- one\n", new: "SELECT 1; -- uno\n", want: CommentOnly},
		{name: "sql dashes in string", path: "q.sql",
			old: "SELECT '-- a';\n", new: "SELECT '-- b';\n", want: Logic},
		{name: "lua block comment", path: "a.lua",
			old: "--[[ a\nb ]]\nx = 1\n", new: "--[[ c\nd ]]\nx = 1\n", want: CommentOnly},
		{name: "haskell block comment", path: "A.hs",
			old: "{- a -}\nmain = pure ()\n", new: "{- b -}\nmain = pure ()\n", want: CommentOnly},
		{name: "haskell prime identifiers", path: "A.hs",
			old: "f x' = x' -- a\n", new: "f x' = x' -- b\n", want: CommentOnly},
		{name: "html comment", path: "index.html",
			old: "<p>hi</p><!-- a -->\n", new: "<p>hi</p><!-- b -->\n", want: CommentOnly},
		{name: "html text change", path: "index.html",
			old: "<p>hi</p>\n", new: "<p>bye</p>\n", want: Logic},
		{name: "css comment", path: "a.css",
			old: "a { color: red; } /* x */\n", new: "a { color: red; } /* y */\n", want: CommentOnly},
		{name: "scss url is not a comment", path: "a.scss",
			old: "a { background: url(http://a/x.png); }\n", new: "a { background: url(http://a/y.png); }\n", want: Logic},
		{name: "clojure char literal", path: "a.clj",
			old: "(def c \\;) ; a\n", new: "(def c \\;) ; b\n", want: CommentOnly},
		{name: "json whitespace", path: "a.json",
			old: "{\"a\":1,\"b\":[1,2]}", new: "{\n  \"a\": 1,\n  \"b\": [1, 2]\n}\n", want: WhitespaceOnly},
		{name: "json value", path: "a.json",
			old: "{\"a\": 1}", new: "{\"a\": 2}", want: Logic},
		{name: "json // is not a comment", path: "a.json",
			old: "{\"a\": 1}\n", new: "{\"a\": 1} // x\n", want: Logic},

		// Docs.
		{name: "readme change", path: "README.md", old: "# Title\nHello\n", new: "# Title\nGoodbye\n", want: DocsOnly},
		{name: "readme whitespace", path: "README.md", old: "# Title\nHello  world\n", new: "# Title\r\n\r\nHello world   \r\n", want: WhitespaceOnly},
		{name: "docs dir", path: "docs/guide/setup.html", old: "<p>a</p>", new: "<p>b</p>", want: DocsOnly},
		{name: "license", path: "LICENSE", old: "MIT\n", new: "Apache\n", want: DocsOnly},
		{name: "binary doc", path: "docs/arch.png", old: "\x89PNG\x00a", new: "\x89PNG\x00b", want: DocsOnly},
		{name: "code in doc dir", path: "internal/doc/doc.go", old: "x := 1\n", new: "x := 2\n", want: Logic},

		// Binary and unknown.
		{name: "binary", path: "a.go", old: "a\x00b", new: "a\x00c", want: Logic},
		{name: "binary unknown", path: "blob", old: "\x00\x01", new: "\x00\x02", want: Logic},
		{name: "unknown trailing whitespace and crlf", path: "conf.ini",
			old: "a = 1\nb = 2\n", new: "a = 1  \r\n\r\nb  =  2\r\n", want: WhitespaceOnly},
		{name: "unknown change", path: "conf.ini", old: "a = 1\n", new: "a = 2\n", want: Logic},
		{name: "unknown reindent", path: "conf.ini", old: "a\n  b\n", new: "a\nb\n", want: Logic},
		{name: "unknown line join", path: "CODEOWNERS",
			old: "* @alice\n@bob\n", new: "* @alice @bob\n", want: Logic},
		{name: "unknown comment change", path: "conf.ini", old: "; a\nx=1\n", new: "; b\nx=1\n", want: Logic},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var old, new []byte
			if !tc.added {
				old = []byte(tc.old)
			}
			if !tc.deleted {
				new = []byte(tc.new)
			}
			got := Classify(tc.path, old, new, !tc.added, !tc.deleted)
			if got != tc.want {
				lang, _ := LanguageFor(tc.path)
				t.Errorf("Classify(%q) = %v, want %v\nold tokens: %q\nnew tokens: %q",
					tc.path, got, tc.want, Tokens(lang, old), Tokens(lang, new))
			}
		})
	}
}

func TestClassifyEmptyVersusAbsent(t *testing.T) {
	// An empty file that exists on both sides is not an addition.
	if got := Classify("a.go", []byte{}, nil, true, true); got != Identical {
		t.Errorf("empty vs nil (both exist) = %v, want identical", got)
	}
	if got := Classify("a.go", nil, nil, false, false); got != Identical {
		t.Errorf("absent on both sides = %v, want identical", got)
	}
}

func TestTokens(t *testing.T) {
	goLang, _ := LanguageFor("x.go")
	py, _ := LanguageFor("x.py")
	sh, _ := LanguageFor("x.sh")
	tests := []struct {
		name string
		lang Language
		src  string
		want []string
	}{
		{"go", goLang, "x := a<-b // c\n/* d */ y++", []string{"x", ":=", "a", "<-", "b", "y", "++"}},
		{"go string", goLang, `s := "a // b" + 'c'`, []string{"s", ":=", `"a // b"`, "+", "'c'"}},
		{"go number", goLang, "f(1.5e-3, 0x1F, x.y)", []string{"f", "(", "1.5e-3", ",", "0x1F", ",", "x", ".", "y", ")"}},
		{"python", py, "if x:  # c\n    y = '#'\n", []string{
			indentPrefix, "if", "x", ":", indentPrefix + "    ", "y", "=", "'#'",
		}},
		{"shell", sh, "echo ${#a} $# # c\n", []string{"echo", "$", "{", "#", "a", "}", "$", "#"}},
		{"shell heredoc", sh, "cat <<-EOF\n\t# x\n\tEOF\n", []string{"cat", "<<-EOF", "\t# x\n\tEOF"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Tokens(tc.lang, []byte(tc.src))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Tokens(%q)\n got %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestLanguageFor(t *testing.T) {
	tests := []struct {
		path string
		want string // "" for unknown
	}{
		{"main.go", "Go"},
		{"pkg/a/b.GO", "Go"},
		{"go.mod", "Go module"},
		{"src/lib.rs", "Rust"},
		{"a.c", "C"},
		{"a.hpp", "C++"},
		{"A.java", "Java"},
		{"a.kt", "Kotlin"},
		{"a.swift", "Swift"},
		{"a.cs", "C#"},
		{"a.scala", "Scala"},
		{"a.dart", "Dart"},
		{"a.js", "JavaScript"},
		{"a.tsx", "TypeScript"},
		{"a.py", "Python"},
		{"a.rb", "Ruby"},
		{"Gemfile", "Ruby"},
		{"a.sh", "Shell"},
		{`C:\home\.bashrc`, "Shell"},
		{"a.pl", "Perl"},
		{"a.yml", "YAML"},
		{"Cargo.toml", "TOML"},
		{"Makefile", "Makefile"},
		{"GNUmakefile", "Makefile"},
		{"rules.mk", "Makefile"},
		{"Dockerfile", "Dockerfile"},
		{"Dockerfile.prod", "Dockerfile"},
		{"app.dockerfile", "Dockerfile"},
		{"BUILD", "Starlark"},
		{"BUILD.bazel", "Starlark"},
		{"defs.bzl", "Starlark"},
		{"a.R", "R"},
		{"CMakeLists.txt", "CMake"},
		{"requirements.txt", "pip requirements"},
		{"requirements-dev.txt", "pip requirements"},
		{"requirements/base.txt", "pip requirements"},
		{"a.sql", "SQL"},
		{"a.lua", "Lua"},
		{"A.hs", "Haskell"},
		{"index.html", "HTML"},
		{"a.svg", "XML"},
		{"a.css", "CSS"},
		{"a.clj", "Clojure"},
		{"a.el", "Lisp"},
		{"a.json", "JSON"},
		{"main.tf", "HCL"},
		{"a.proto", "Protocol Buffers"},
		{"README.md", ""},
		{"notes.txt", ""},
		{"image.png", ""},
		{"noext", ""},
		{"", ""},
	}
	for _, tc := range tests {
		lang, ok := LanguageFor(tc.path)
		if ok != (tc.want != "") || lang.Name != tc.want {
			t.Errorf("LanguageFor(%q) = %q, %v; want %q", tc.path, lang.Name, ok, tc.want)
		}
	}
	if len(languageTable) < 25 {
		t.Errorf("only %d languages", len(languageTable))
	}
}

func TestLanguageForReturnsCopy(t *testing.T) {
	a, _ := LanguageFor("a.go")
	a.LineComments[0] = "XX"
	a.Strings[0].Open = "XX"
	b, _ := LanguageFor("a.go")
	if b.LineComments[0] != "//" || b.Strings[0].Open != `"` {
		t.Fatal("LanguageFor returned a shared table entry")
	}
}

func TestLanguageTableSane(t *testing.T) {
	for _, e := range languageTable {
		l := e.lang
		for _, d := range l.Strings {
			if d.Open == "" || d.Close == "" {
				t.Errorf("%s: empty string delimiter %+v", l.Name, d)
			}
		}
		for _, b := range l.BlockComments {
			if b[0] == "" || b[1] == "" {
				t.Errorf("%s: empty block comment %q", l.Name, b)
			}
		}
		for _, c := range l.LineComments {
			if c == "" {
				t.Errorf("%s: empty line comment", l.Name)
			}
		}
	}
}

func TestIsDocPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"README.md", true},
		{"README", true},
		{"pkg/readme.rst", true},
		{"README_zh.md", true},
		{"LICENSE", true},
		{"LICENSE-MIT", true},
		{"CHANGELOG", true},
		{"CONTRIBUTING.adoc", true},
		{"NOTICE", true},
		{"notes.txt", true},
		{"guide.markdown", true},
		{"docs/index.html", true},
		{"docs/img/arch.png", true},
		{"project/doc/overview", true},
		{"Docs/style.css", true},
		{`docs\setup.md`, true},
		{"main.go", false},
		{"ReadmeParser.java", false},
		{"license.go", false},
		{"docs/conf.py", false},
		{"internal/doc/doc.go", false},
		{"doc.go", false},
		{"docs/Makefile", false},
		{"CMakeLists.txt", false},
		{"requirements.txt", false},
		{"docs/CODEOWNERS", false},
		{"documentation.yaml", false},
		{"src/docsite/x.ts", false},
	}
	for _, tc := range tests {
		if got := IsDocPath(tc.path); got != tc.want {
			t.Errorf("IsDocPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestCombine(t *testing.T) {
	tests := []struct {
		in   []Class
		want Class
	}{
		{nil, Identical},
		{[]Class{Identical}, Identical},
		{[]Class{WhitespaceOnly, Identical}, WhitespaceOnly},
		{[]Class{CommentOnly, WhitespaceOnly}, CommentOnly},
		{[]Class{DocsOnly, CommentOnly, Identical}, DocsOnly},
		{[]Class{WhitespaceOnly, Logic, DocsOnly}, Logic},
	}
	for _, tc := range tests {
		if got := Combine(tc.in...); got != tc.want {
			t.Errorf("Combine(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestClassString(t *testing.T) {
	want := map[Class]string{
		Identical:      "identical",
		WhitespaceOnly: "whitespace",
		CommentOnly:    "comments",
		DocsOnly:       "docs",
		Logic:          "logic",
		Class(42):      "Class(42)",
	}
	for c, s := range want {
		if got := c.String(); got != s {
			t.Errorf("Class(%d).String() = %q, want %q", int(c), got, s)
		}
	}
	if !(Identical < WhitespaceOnly && WhitespaceOnly < CommentOnly && CommentOnly < DocsOnly && DocsOnly < Logic) {
		t.Error("classes are not ordered by consequence")
	}
}

func TestTokensRobustness(t *testing.T) {
	// Unterminated constructs and odd input must not panic.
	inputs := []string{
		"\"", "'", "`", "/*", "<<", "<<EOF", "<<'", "#!", "\\", "'\\", "r#\"", "x = /", "1e", "'a",
		"cat <<A <<B\na\nA\nb\nB\n", strings.Repeat("\"a\\", 3),
	}
	for _, e := range languageTable {
		for _, in := range inputs {
			Tokens(e.lang, []byte(in))
			Classify("x", []byte(in), []byte(in+" "), true, true)
		}
	}
}
