package stintcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBuildHeartbeatDetectsProjectLanguageAndLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".wakatime-project"), []byte("custom/{project}\nmain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "custom/"+filepath.Base(dir) {
		t.Fatalf("project = %q", hb.Project)
	}
	if hb.Branch != "main" {
		t.Fatalf("branch = %q", hb.Branch)
	}
	if hb.Language != "Go" {
		t.Fatalf("language = %q", hb.Language)
	}
	if hb.Lines == nil || *hb.Lines != 2 {
		t.Fatalf("lines = %#v", hb.Lines)
	}
	if hb.ProjectRootCount == nil || *hb.ProjectRootCount == 0 {
		t.Fatalf("project root count = %#v", hb.ProjectRootCount)
	}
	if hb.UserAgent == "" {
		t.Fatalf("expected user agent")
	}
}

func TestBuildHeartbeatGuessLanguageFromContents(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "script")
	if err := os.WriteFile(file, []byte("#!/usr/bin/env bash\necho hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "" {
		t.Fatalf("language without guessing = %q", hb.Language)
	}
	hb, err = BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", GuessLanguage: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "Bash" {
		t.Fatalf("language with guessing = %q", hb.Language)
	}
}

func TestBuildHeartbeatGuessLanguageFromVimModelineLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "script")
	if err := os.WriteFile(file, []byte("/* vim: tw=60 ft=python ts=2: */\nprint('hi')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", GuessLanguage: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "Python" {
		t.Fatalf("language = %q, want Python", hb.Language)
	}
}

func TestBuildHeartbeatDetectsCPlusPlusHeadersLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	header := filepath.Join(dir, "widget.h")
	if err := os.WriteFile(header, []byte("#pragma once\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widget.cpp"), []byte("#include \"widget.h\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: header, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "C++" {
		t.Fatalf("language = %q, want C++", hb.Language)
	}
}

func TestBuildHeartbeatDetectsMatlabMFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "analysis.m")
	if err := os.WriteFile(file, []byte("disp('hi')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.mat"), []byte("matlab data"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "Matlab" {
		t.Fatalf("language = %q, want Matlab", hb.Language)
	}
}

func TestBuildHeartbeatDetectsForthFSFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "stack.fs")
	if err := os.WriteFile(file, []byte(": square dup * ;\n\n\\ Forth line comment\n\n( stack effect comment )\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "Forth" {
		t.Fatalf("language = %q, want Forth", hb.Language)
	}
}

func TestBuildHeartbeatDetectsFSharpFSFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "program.fs")
	if err := os.WriteFile(file, []byte("let describe value =\n    match value with\n    | Some text -> text\n    | None -> \"missing\"\n\n// F# line comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "F#" {
		t.Fatalf("language = %q, want F#", hb.Language)
	}
}

func TestBuildHeartbeatDetectsCMakeSpecialFilenameLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "CMmakeLists.txt")
	if err := os.WriteFile(file, []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "CMake" {
		t.Fatalf("language = %q, want CMake", hb.Language)
	}
}

func TestBuildHeartbeatDetectsJSXFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "component.jsx")
	if err := os.WriteFile(file, []byte("export function Component() { return <div />; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "JSX" {
		t.Fatalf("language = %q, want JSX", hb.Language)
	}
}

func TestBuildHeartbeatNormalizesTopLanguageAliasesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{
		".ruby-version":         "Ruby",
		".Rprofile":             "S",
		"crontab":               "Crontab",
		"file.cfm":              "ColdFusion",
		"file.fhtml":            "Velocity",
		"file.fsi":              "F#",
		"file.gs":               "Gosu",
		"file.i":                "SWIG",
		"file.inc":              "Pawn",
		"file.j":                "Objective-J",
		"file.kif":              "newLisp",
		"file.lasso9":           "Lasso",
		"file.markdown":         "Markdown",
		"file.marko":            "Marko",
		"file.mustache":         "Mustache",
		"file.mo":               "Modelica",
		"file.pug":              "Pug",
		"file.re":               "Reason",
		"file.sketch":           "Sketch Drawing",
		"file.slim":             "Slim",
		"file.sublime-settings": "Sublime Text Config",
		"file.swg":              "SWIG",
		"file.svh":              "SystemVerilog",
		"file.txt":              "Text",
		"file.vue":              "Vue.js",
		"file.vm":               "Velocity",
		"file.xaml":             "XAML",
		"file.xpl":              "XSLT",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(dir, name)
			if err := os.WriteFile(file, []byte("sample\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
			if err != nil {
				t.Fatal(err)
			}
			if hb.Language != want {
				t.Fatalf("language = %q, want %s", hb.Language, want)
			}
		})
	}
}

func TestBuildHeartbeatDetectsDependenciesFromResolvedLanguageLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	goTextFile := filepath.Join(dir, "main.txt")
	if err := os.WriteFile(goTextFile, []byte("package main\n\nimport \"net/http\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: goTextFile, EntityType: "file", Category: "coding", Language: "Go"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(hb.Dependencies, ",") != "net/http" {
		t.Fatalf("explicit Go dependency parse = %#v, want net/http", hb.Dependencies)
	}

	goFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n\nimport \"net/http\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err = BuildHeartbeat(Options{Entity: goFile, EntityType: "file", Category: "coding", Language: "Python"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Dependencies != nil {
		t.Fatalf("explicit Python dependency parse on Go source = %#v, want nil", hb.Dependencies)
	}
}

func TestBuildHeartbeatDependencyLanguageAliasesMatchWakaTime(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		language string
		body     string
		want     string
	}{
		{name: "CSharp", language: "CSharp", body: "using Newtonsoft.Json;\n", want: "Newtonsoft"},
		{name: "CPP", language: "CPP", body: "#include <vector>\n", want: "vector"},
		{name: "ObjectiveC", language: "ObjectiveC", body: "#import <Foundation/Foundation.h>\n", want: "Foundation"},
		{name: "Visual Basic .NET", language: "Visual Basic .NET", body: "Imports Newtonsoft.Json\n", want: "Newtonsoft"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "_")+".txt")
			if err := os.WriteFile(file, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Language: tt.language})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(hb.Dependencies, ",") != tt.want {
				t.Fatalf("%s dependencies = %#v, want %q", tt.language, hb.Dependencies, tt.want)
			}
		})
	}
}

func TestDetectDependenciesCapsWakaTimeDependencyCount(t *testing.T) {
	var body strings.Builder
	body.WriteString("package main\n\nimport (\n")
	for i := 0; i < maxDependenciesCount+5; i++ {
		body.WriteString(fmt.Sprintf("\t\"example.com/dep%04d\"\n", i))
	}
	body.WriteString(")\n")
	file := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(file, []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	got := detectDependencies(file)
	if len(got) != maxDependenciesCount {
		t.Fatalf("dependency count = %d, want %d", len(got), maxDependenciesCount)
	}
	if got[0] != "example.com/dep0000" || got[len(got)-1] != "example.com/dep0999" {
		t.Fatalf("dependency cap did not preserve first-seen dependencies: first=%q last=%q", got[0], got[len(got)-1])
	}
}

func TestDetectAdditionalLanguageDependencies(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
		want []string
	}{
		{
			name: "csharp",
			file: "Program.cs",
			body: "using System;\nusing Microsoft.Extensions.Logging;\nusing WakaTime.Forms;\nusing static Math.Foo;\nusing Task = Fart.Threading.Tasks.Task;\nusing static Proper.Bar;\n",
			want: []string{"WakaTime", "Math", "Fart", "Proper"},
		},
		{
			name: "c",
			file: "main.c",
			body: "#include <stdio.h>\n#include <math.h>\n#include <openssl/ssl.h>\n",
			want: []string{"math", "openssl"},
		},
		{
			name: "cpp",
			file: "main.cpp",
			body: "#include <iostream>\n#include <openssl/ssl.h>\n#include \"wakatime/client.h\"\n",
			want: []string{"openssl", "wakatime"},
		},
		{
			name: "typescript",
			file: "App.tsx",
			body: "import React from 'react'\nimport Footer from '../components/Footer.tsx'\nimport constants from \"./lib/constants.js\"\nconst fp = require('lodash/fp')\nimport pkg from '@scope/package'\n",
			want: []string{"react", "Footer", "constants", "fp", "package"},
		},
		{
			name: "typescript multiline import",
			file: "App.ts",
			body: "import {\n  alpha,\n  bravo,\n} from './november';\nimport charlie from 'delta';\n",
			want: []string{"november", "delta"},
		},
		{
			name: "javascript waka fixture imports",
			file: "es6.js",
			body: "import Alpha from './bravo';\nimport { charlie, delta } from '../../echo/foxtrot';\nimport golf from './hotel/india.js';\nimport juliett from 'kilo';\nimport {\n  lima,\n  mike,\n} from './november';\nimport * from '/modules/oscar';\nimport * as papa from 'quebec';\nimport {romeo as sierra} from from 'tango.jsx';\nimport 'uniform.js';\nimport victorDefault, * as victorModule from '/modules/victor.js';\nimport whiskeyDefault, {whiskeyOne, whiskeyTwo} from 'whiskey';\n",
			want: []string{"bravo", "foxtrot", "india", "kilo", "november", "oscar", "quebec", "tango", "uniform", "victor", "whiskey"},
		},
		{
			name: "gruntfile",
			file: "Gruntfile",
			body: "module.exports = function(grunt) {\n  require('grunt');\n}\n",
			want: []string{"grunt"},
		},
		{
			name: "python",
			file: "main.py",
			body: "import os, flask, simplejson as json\nfrom django.conf import settings\nfrom sys import path\n",
			want: []string{"flask", "simplejson", "django"},
		},
		{
			name: "go",
			file: "main.go",
			body: "package main\n\nimport (\n\t\"fmt\"\n\t\"log\"\n\t\"os\"\n\t\"github.com/acme/pkg\"\n)\n",
			want: []string{"log", "os", "github.com/acme/pkg"},
		},
		{
			name: "java",
			file: "Hello.java",
			body: "import java.io.*;\nimport static com.googlecode.javacv.jna.highgui.cvReleaseCapture;\nimport javax.servlet.*;\nimport com.colorfulwolf.webcamapplet.gui.ImagePanel;\nimport com.foobar.*;\nimport package com.apackage.something;\nimport namespace com.anamespace.other;\n",
			want: []string{"googlecode.javacv", "colorfulwolf.webcamapplet", "foobar", "apackage.something", "anamespace.other"},
		},
		{
			name: "kotlin",
			file: "Main.kt",
			body: "import java.util.List\nimport com.squareup.moshi.Moshi\nimport org.example.tools.*\n",
			want: []string{"squareup.moshi", "example.tools"},
		},
		{
			name: "scala",
			file: "Main.scala",
			body: "import __root__.com.alpha.SomeClass\nimport _root_.com.bravo.something\nimport com.charlie._\nimport golf\nimport juliett.kilo.Lima\n",
			want: []string{"com.alpha.SomeClass", "com.bravo.something", "com.charlie", "golf", "juliett.kilo.Lima"},
		},
		{
			name: "scala grouped imports",
			file: "Grouped.scala",
			body: "import com.alpha.SomeClass\nimport com.bravo.something.{User, UserPreferences}\nimport com.charlie.{Delta => Foxtrot}\nimport __root__.golf._\nimport com.hotel.india._\nimport juliett.kilo\n",
			want: []string{"com.alpha.SomeClass", "com.bravo.something", "com.charlie", "golf", "com.hotel.india", "juliett.kilo"},
		},
		{
			name: "haskell",
			file: "Main.hs",
			body: "import qualified Data.Map as Map\nimport Control.Monad\n",
			want: []string{"Data", "Control"},
		},
		{
			name: "elm",
			file: "Main.elm",
			body: "import Html exposing (text)\nimport Json.Decode as Decode\n",
			want: []string{"Html", "Json"},
		},
		{
			name: "rust",
			file: "lib.rs",
			body: "extern crate proc_macro;\nextern crate phrases;\nuse serde::Serialize;\n",
			want: []string{"proc_macro", "phrases"},
		},
		{
			name: "haxe",
			file: "Main.hx",
			body: "import alpha.ds.StringMap;\nimport bravo.macro.*;\nimport Math.random;\nimport charlie.fromCharCode in f;\nimport delta.something;\nimport haxe.ds.StringMap;\n",
			want: []string{"alpha", "bravo", "Math", "charlie", "delta"},
		},
		{
			name: "html",
			file: "index.html",
			body: `<script src="/static/app.js"></script>` + "\n" + `<script type="text/javascript" src="{{ STATIC_URL }}/libs/json2.js"></script>` + "\n" + `<script src="this is a` + "\n" + ` multiline value"></script>` + "\n",
			want: []string{`"/static/app.js"`, `"libs/json2.js"`, "\"this is a\n multiline value\""},
		},
		{
			name: "objective-c",
			file: "ViewController.m",
			body: "#import \"SomeViewController.h\"\n#import 'OtherViewController.h'\n#import <UIKit/UIKit.h>\n#import <PromiseKit/PromiseKit.h>\n",
			want: []string{"SomeViewController", "OtherViewController", "UIKit", "PromiseKit"},
		},
		{
			name: "php",
			file: "service.php",
			body: "<?php\nuse Interop\\Container\\ContainerInterface;\nrequire 'ServiceLocator.php';\nrequire \"ServiceLocatorTwo.php\";\nuse FooBarOne\\Classname as Another;\nuse function FooBarThree\\Full\\functionNameThree;\nuse FooBarSeven\\Full\\ClassnameSeven as AnotherSeven, FooBarEight\\Full\\NSnameEight;\n",
			want: []string{"Interop", "'ServiceLocator.php'", "'ServiceLocatorTwo.php'", "FooBarOne", "FooBarThree", "FooBarSeven", "FooBarEight"},
		},
		{
			name: "swift",
			file: "ViewController.swift",
			body: "import Foundation\nimport UIKit\nimport PromiseKit\n",
			want: []string{"UIKit", "PromiseKit"},
		},
		{
			name: "vbnet",
			file: "Main.vb",
			body: "Imports System\nImports Microsoft.VisualBasic\nImports WakaTime.Core\nImports mat = Math.Foo\nImports pr = Proper.Bar\n",
			want: []string{"WakaTime", "Math", "Proper"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.file)
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got := detectDependencies(path)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("dependencies = %#v, want %#v", got, tt.want)
			}
		})
	}
}
