package stintcli

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
)

func detectLanguage(path string) string {
	return detectLanguageWithGuess(path, false)
}

func detectLanguageWithGuess(path string, guess bool) string {
	base := filepath.Base(path)
	if language := wakaTimeExactLanguageFiles[strings.ToLower(base)]; language != "" {
		return language
	}
	if strings.EqualFold(base, "go.mod") {
		return "Go"
	}
	if base == "CMmakeLists.txt" {
		return "CMake"
	}
	ext := strings.ToLower(filepath.Ext(path))
	if language := wakaTimeExtensionLanguages[ext]; language != "" {
		return language
	}
	switch ext {
	case ".go":
		return "Go"
	case ".js":
		return "JavaScript"
	case ".jsx":
		return "JSX"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".py":
		return "Python"
	case ".rb":
		return "Ruby"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".c", ".h":
		return detectCFamilyLanguage(path)
	case ".cc", ".cpp", ".hpp":
		return "C++"
	case ".m":
		return detectMFileLanguage(path)
	case ".mm":
		if siblingExists(strings.TrimSuffix(path, filepath.Ext(path)), ".h") {
			return "Objective-C++"
		}
		return "Objective-C++"
	case ".pas":
		if folderContainsAnyExtension(filepath.Dir(path), ".fmx", ".dfm", ".dproj") {
			return "Delphi"
		}
		if lexer := lexers.Match(path); lexer != nil {
			return lexer.Config().Name
		}
		return ""
	case ".cs":
		return "C#"
	case ".php":
		return "PHP"
	case ".sh", ".bash", ".zsh":
		return "Bash"
	case ".fs":
		return detectFSFileLanguage(path)
	case ".swift":
		return "Swift"
	case ".pbxproj":
		return "Xcode Config"
	case ".md":
		return "Markdown"
	default:
		if lexer := lexers.Match(path); lexer != nil {
			return wakaTimeLanguageName(lexer.Config().Name)
		}
		if guess {
			head := readHead(path, 64*1024)
			if language := detectVimModelineLanguage(head); language != "" {
				return language
			}
			if lexer := lexers.Analyse(head); lexer != nil {
				return wakaTimeLanguageName(lexer.Config().Name)
			}
		}
		return ""
	}
}

func detectFSFileLanguage(path string) string {
	text := readHead(path, 64*1024)
	forthWeight := float32(0)
	if forthFuncRegex.MatchString(text) {
		forthWeight = 0.9
	}
	if strings.Contains(text, `\ `) {
		forthWeight += 0.5
	}
	if strings.Contains(text, "( ") {
		forthWeight += 0.2
	}
	if forthWeight > 1 {
		forthWeight = 1
	}

	fsharpWeight := float32(0)
	if strings.Contains(text, "let ") && strings.Contains(text, "match ") && strings.Contains(text, " ->") {
		fsharpWeight = 0.9
	}
	if strings.Contains(text, "// ") || (strings.Contains(text, "(* ") && strings.Contains(text, " *)")) {
		fsharpWeight += 0.7
	}
	if fsharpWeight > 1 {
		fsharpWeight = 1
	}

	if fsharpWeight > 0 && fsharpWeight >= forthWeight {
		return "F#"
	}
	if forthWeight > 0 {
		return "Forth"
	}
	if lexer := lexers.Match(path); lexer != nil {
		return wakaTimeLanguageName(lexer.Config().Name)
	}
	return ""
}

func detectCFamilyLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	if ext == ".h" || strings.HasPrefix(ext, ".c") {
		if siblingExists(stem, ".c") {
			return "C"
		}
		if siblingExists(stem, ".m") {
			return "Objective-C"
		}
		if siblingExists(stem, ".mm") {
			return "Objective-C++"
		}
		dir := filepath.Dir(path)
		if folderContainsAnyExtension(dir, ".cpp", ".hpp", ".c++", ".h++", ".cc", ".hh", ".cxx", ".hxx", ".C", ".H", ".cp", ".CPP") {
			return "C++"
		}
		if folderContainsAnyExtension(dir, ".c") {
			return "C"
		}
	}
	return "C"
}

func detectMFileLanguage(path string) string {
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	if siblingExists(stem, ".h") {
		return "Objective-C"
	}
	if folderContainsAnyExtension(filepath.Dir(path), ".mat") {
		return "Matlab"
	}
	if lexer := lexers.Match(path); lexer != nil {
		return wakaTimeLanguageName(lexer.Config().Name)
	}
	return ""
}

func siblingExists(stem, extension string) bool {
	if _, err := os.Stat(stem + extension); err == nil {
		return true
	}
	if _, err := os.Stat(stem + strings.ToUpper(extension)); err == nil {
		return true
	}
	return false
}

func folderContainsAnyExtension(dir string, extensions ...string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		for _, candidate := range extensions {
			if ext == candidate {
				return true
			}
		}
	}
	return false
}

func detectVimModelineLanguage(text string) string {
	matches := vimModelineRegex.FindStringSubmatch(text)
	if len(matches) != 2 {
		return ""
	}
	name := strings.ToLower(strings.TrimSpace(matches[1]))
	switch name {
	case "a65", "asm", "asm68k", "asmh8300":
		name = "asm"
	case "cs":
		name = "csharp"
	case "htmlcheetah", "htmldjango", "htmlm4", "xhtml":
		name = "html"
	case "lhaskell":
		name = "haskell"
	case "objc":
		return "Objective-C"
	case "objcpp":
		return "Objective-C++"
	case "perl6":
		name = "perl"
	case "phtml":
		name = "php"
	case "vb":
		return "VB.NET"
	case "vim":
		return "VimL"
	}
	if lexer := lexers.Get(name); lexer != nil {
		return wakaTimeLanguageName(lexer.Config().Name)
	}
	return ""
}

func wakaTimeLanguageName(name string) string {
	switch strings.ToLower(name) {
	case "emacslisp":
		return "Emacs Lisp"
	case "fsharp":
		return "F#"
	case "markdown":
		return "Markdown"
	case "plaintext":
		return "Text"
	case "r":
		return "S"
	case "reasonml":
		return "Reason"
	case "systemverilog":
		return "SystemVerilog"
	case "vue":
		return "Vue.js"
	default:
		return name
	}
}

func readHead(path string, maxBytes int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return ""
	}
	return string(data)
}

func countLines(path string) int {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxFileStatsBytes {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}
