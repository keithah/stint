package stintcli

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

const maxDependencyNameLength = 200
const maxDependenciesCount = 1000

var (
	goImportBlockStart = regexp.MustCompile(`^\s*import\s*\(`)
	goImportSingle     = regexp.MustCompile(`^\s*import\s+(?:[._A-Za-z0-9]+\s+)?["']([^"']+)["']`)
	jsImportFrom       = regexp.MustCompile(`^\s*import(?:\s+type)?(?:\s+["']([^"']+)["']|(?:.+?\s+from\s+)?["']([^"']+)["'])`)
	jsImportBlockStart = regexp.MustCompile(`^\s*import(?:\s+type)?\s*[\{\*\w]`)
	jsImportBlockFrom  = regexp.MustCompile(`\bfrom\s+["']([^"']+)["']`)
	jsRequire          = regexp.MustCompile(`\brequire\(\s*["']([^"']+)["']\s*\)`)
	jsExtension        = regexp.MustCompile(`\.\w{1,4}$`)
	pythonImport       = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_.,\s]+)`)
	pythonFrom         = regexp.MustCompile(`^\s*from\s+([A-Za-z0-9_.]+)\s+import\s+`)
	rustExternCrate    = regexp.MustCompile(`^\s*extern\s+crate\s+([A-Za-z0-9_]+)`)
	cInclude           = regexp.MustCompile(`^\s*#\s*include\s*[<"]([^>"]+)[>"]`)
	javaImport         = regexp.MustCompile(`^\s*import\s+(?:(?:static|package|namespace)\s+)?([A-Za-z0-9_.*]+)\s*;`)
	csharpUsing        = regexp.MustCompile(`^\s*using\s+(?:static\s+)?(?:[A-Za-z0-9_]+\s*=\s*)?([A-Za-z0-9_.]+)\s*;`)
	kotlinImport       = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_.*]+)`)
	scalaImport        = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_.*{}]+)`)
	haskellImport      = regexp.MustCompile(`^\s*import\s+(?:qualified\s+)?([A-Za-z0-9_.']+)`)
	elmImport          = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_.]+)`)
	haxeImport         = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_.]+)`)
	htmlScriptSrc      = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc\s*=\s*(".*?"|'.*?')`)
	htmlPlaceholder    = regexp.MustCompile(`(?i)\{\{[^\}]+\}\}[/\\]?`)
	objectiveCImport   = regexp.MustCompile(`^\s*#\s*import\s+(?:["'<])([^"'>]+)(?:["'>])`)
	phpInclude         = regexp.MustCompile(`^\s*(?:include|include_once|require|require_once)\s+["']([^"']+)["']`)
	swiftImport        = regexp.MustCompile(`^\s*import\s+([A-Za-z0-9_]+)`)
	vbnetImport        = regexp.MustCompile(`(?i)^\s*Imports\s+(.+)$`)
)

func detectDependencies(path string) []string {
	path = expandHome(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return detectJSONDependencies(path)
	default:
		if deps := detectUnknownFileDependencies(path); len(deps) > 0 {
			return deps
		}
		return detectSourceDependencies(path)
	}
}

func detectDependenciesForLanguage(path, language string) []string {
	path = expandHome(path)
	ext, ok := dependencyExtForLanguage(language)
	if !ok {
		if strings.TrimSpace(language) != "" {
			return detectUnknownFileDependencies(path)
		}
		return detectDependencies(path)
	}
	if ext == ".json" {
		return detectJSONDependencies(path)
	}
	return detectSourceDependenciesWithExt(path, ext)
}

func dependencyExtForLanguage(language string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "c":
		return ".c", true
	case "c++", "cpp":
		return ".cpp", true
	case "c#", "csharp":
		return ".cs", true
	case "elm":
		return ".elm", true
	case "go", "golang":
		return ".go", true
	case "haskell":
		return ".hs", true
	case "haxe":
		return ".hx", true
	case "html":
		return ".html", true
	case "java":
		return ".java", true
	case "javascript":
		return ".js", true
	case "json":
		return ".json", true
	case "jsx", "react":
		return ".jsx", true
	case "kotlin":
		return ".kt", true
	case "objective-c", "objective c", "objectivec", "objc", "obj-c":
		return ".m", true
	case "php":
		return ".php", true
	case "python", "python 2":
		return ".py", true
	case "rust":
		return ".rs", true
	case "scala":
		return ".scala", true
	case "swift":
		return ".swift", true
	case "tsx":
		return ".tsx", true
	case "typescript":
		return ".ts", true
	case "vb.net", "vbnet", "visual basic .net":
		return ".vb", true
	case "unknown":
		return "", false
	default:
		return "", false
	}
}

func detectUnknownFileDependencies(path string) []string {
	deps := newDependencyCollector()
	filename := strings.ToLower(filepath.Base(path))
	if strings.Contains(filename, "grunt") {
		deps.add("grunt")
	}
	return deps.list()
}

func detectJSONDependencies(path string) []string {
	data := readHead(path, maxFileStatsBytes)
	if data == "" {
		return nil
	}
	deps := newDependencyCollector()
	switch strings.ToLower(filepath.Base(path)) {
	case "bower.json", "component.json":
		deps.add("bower")
	case "package.json":
		deps.add("npm")
	}
	decoder := json.NewDecoder(strings.NewReader(data))
	if err := collectJSONDependencies(decoder, deps); err != nil {
		return deps.list()
	}
	return deps.list()
}

func detectSourceDependencies(path string) []string {
	return detectSourceDependenciesWithExt(path, strings.ToLower(filepath.Ext(path)))
}

func detectSourceDependenciesWithExt(path, ext string) []string {
	head := readHead(path, maxFileStatsBytes)
	if head == "" {
		return nil
	}
	ext = strings.ToLower(ext)
	if isHTMLExt(ext) {
		return detectHTMLDependencies(head)
	}
	deps := newDependencyCollector()
	inGoImportBlock := false
	inJSImportBlock := false
	scanner := bufio.NewScanner(strings.NewReader(head))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if isGoExt(ext) && goImportBlockStart.MatchString(line) {
			inGoImportBlock = true
			continue
		}
		if inGoImportBlock {
			if strings.HasPrefix(line, ")") {
				inGoImportBlock = false
				continue
			}
			if dep := quotedImport(line); dep != "" {
				deps.add(goDependency(dep))
			}
			continue
		}
		if isGoExt(ext) {
			for _, match := range goImportSingle.FindAllStringSubmatch(line, -1) {
				deps.add(goDependency(match[1]))
			}
		}
		if isJSExt(ext) {
			jsMatched := false
			for _, match := range jsImportFrom.FindAllStringSubmatch(line, -1) {
				if dep := first(match[1:]...); dep != "" {
					deps.add(jsDependency(dep))
					jsMatched = true
				}
			}
			if !jsMatched && inJSImportBlock {
				if match := jsImportBlockFrom.FindStringSubmatch(line); len(match) == 2 {
					deps.add(jsDependency(match[1]))
					inJSImportBlock = false
				}
			}
			if !jsMatched && !inJSImportBlock && jsImportBlockStart.MatchString(line) {
				inJSImportBlock = true
			}
			for _, match := range jsRequire.FindAllStringSubmatch(line, -1) {
				deps.add(jsDependency(match[1]))
			}
		}
		if ext == ".py" {
			for _, match := range pythonImport.FindAllStringSubmatch(line, -1) {
				for _, part := range strings.Split(match[1], ",") {
					fields := strings.Fields(strings.TrimSpace(part))
					if len(fields) > 0 {
						deps.add(pythonDependency(fields[0]))
					}
				}
			}
			for _, match := range pythonFrom.FindAllStringSubmatch(line, -1) {
				deps.add(pythonDependency(match[1]))
			}
		}
		if ext == ".rs" {
			for _, match := range rustExternCrate.FindAllStringSubmatch(line, -1) {
				deps.add(match[1])
			}
		}
		if isCExt(ext) {
			for _, match := range cInclude.FindAllStringSubmatch(line, -1) {
				deps.add(cDependency(match[1], isCPPExt(ext)))
			}
		}
		if ext == ".java" {
			for _, match := range javaImport.FindAllStringSubmatch(line, -1) {
				deps.add(javaDependency(match[1]))
			}
		}
		if ext == ".cs" {
			for _, match := range csharpUsing.FindAllStringSubmatch(line, -1) {
				deps.add(csharpDependency(match[1]))
			}
		}
		if ext == ".kt" || ext == ".kts" {
			for _, match := range kotlinImport.FindAllStringSubmatch(line, -1) {
				deps.add(javaDependency(match[1]))
			}
		}
		if ext == ".scala" {
			for _, match := range scalaImport.FindAllStringSubmatch(line, -1) {
				deps.add(scalaDependency(match[1]))
			}
		}
		if ext == ".hs" {
			for _, match := range haskellImport.FindAllStringSubmatch(line, -1) {
				deps.add(strings.Split(match[1], ".")[0])
			}
		}
		if ext == ".elm" {
			for _, match := range elmImport.FindAllStringSubmatch(line, -1) {
				deps.add(strings.Split(match[1], ".")[0])
			}
		}
		if ext == ".hx" {
			for _, match := range haxeImport.FindAllStringSubmatch(line, -1) {
				deps.add(haxeDependency(match[1]))
			}
		}
		if isObjectiveCExt(ext) {
			for _, match := range objectiveCImport.FindAllStringSubmatch(line, -1) {
				deps.add(objectiveCDependency(match[1]))
			}
		}
		if ext == ".php" {
			processPHPDependencies(line, deps)
		}
		if ext == ".swift" {
			for _, match := range swiftImport.FindAllStringSubmatch(line, -1) {
				deps.add(swiftDependency(match[1]))
			}
		}
		if ext == ".vb" {
			for _, match := range vbnetImport.FindAllStringSubmatch(line, -1) {
				deps.add(vbnetDependency(match[1]))
			}
		}
	}
	return deps.list()
}

func detectHTMLDependencies(head string) []string {
	deps := newDependencyCollector()
	for _, match := range htmlScriptSrc.FindAllStringSubmatch(head, -1) {
		deps.add(htmlPlaceholder.ReplaceAllString(match[1], ""))
	}
	return deps.list()
}

func isGoExt(ext string) bool { return ext == ".go" }

func isJSExt(ext string) bool {
	switch ext {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func isCExt(ext string) bool {
	switch ext {
	case ".c", ".cc", ".cpp", ".cxx", ".h", ".hpp", ".hh":
		return true
	default:
		return false
	}
}

func isCPPExt(ext string) bool {
	switch ext {
	case ".cc", ".cpp", ".cxx", ".hpp", ".hh":
		return true
	default:
		return false
	}
}

func isHTMLExt(ext string) bool {
	switch ext {
	case ".html", ".htm":
		return true
	default:
		return false
	}
}

func isObjectiveCExt(ext string) bool {
	switch ext {
	case ".m", ".mm":
		return true
	default:
		return false
	}
}

func quotedImport(line string) string {
	start := strings.IndexAny(line, `"'`)
	if start < 0 {
		return ""
	}
	quote := line[start]
	end := strings.IndexByte(line[start+1:], quote)
	if end < 0 {
		return ""
	}
	return line[start+1 : start+1+end]
}

func goDependency(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"`))
	if value == "fmt" {
		return ""
	}
	return value
}

type dependencyCollector struct {
	values []string
	seen   map[string]struct{}
}

func newDependencyCollector() *dependencyCollector {
	return &dependencyCollector{seen: make(map[string]struct{})}
}

func (c *dependencyCollector) add(value string) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxDependencyNameLength || strings.HasPrefix(value, ".") || len(c.values) >= maxDependenciesCount {
		return
	}
	if _, ok := c.seen[value]; ok {
		return
	}
	c.seen[value] = struct{}{}
	c.values = append(c.values, value)
}

func (c *dependencyCollector) list() []string {
	return c.values
}

func collectJSONDependencies(decoder *json.Decoder, deps *dependencyCollector) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return errors.New("json root is not an object")
	}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("json object key is not a string")
		}
		switch key {
		case "dependencies", "devDependencies":
			if err := collectJSONDependencyObject(decoder, deps); err != nil {
				return err
			}
		default:
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	}
	_, err = decoder.Token()
	return err
}

func collectJSONDependencyObject(decoder *json.Decoder, deps *dependencyCollector) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil
	}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("json dependency key is not a string")
		}
		deps.add(key)
		if err := skipJSONValue(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func skipJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return nil
	}
	for decoder.More() {
		if err := skipJSONValue(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func javaDependency(value string) string {
	if strings.HasPrefix(value, "java.") || strings.HasPrefix(value, "javax.") {
		return ""
	}
	return jvmDependency(value)
}

func jvmDependency(value string) string {
	parts := strings.Split(strings.TrimSuffix(value, ".*"), ".")
	if len(parts) > 0 && len(parts[0]) == 3 {
		parts = parts[1:]
	}
	return strings.Join(firstN(parts, 2), ".")
}

func scalaDependency(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "__root__")
	value = strings.TrimPrefix(value, "_root_")
	if before, _, ok := strings.Cut(value, "{"); ok {
		value = before
	}
	value = strings.Trim(value, "_. ")
	return value
}

func haxeDependency(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, "."))
	value = strings.Split(value, ".")[0]
	if strings.EqualFold(value, "haxe") {
		return ""
	}
	return value
}

func csharpDependency(value string) string {
	first := strings.Split(value, ".")[0]
	switch strings.ToLower(first) {
	case "system", "microsoft":
		return ""
	default:
		return first
	}
}

func cDependency(value string, cpp bool) string {
	value = strings.TrimSpace(strings.Trim(value, `"<> `))
	value = strings.Split(value, "/")[0]
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	switch lower {
	case "stdio.h", "stdlib.h", "string.h", "time.h":
		return ""
	case "iostream":
		if cpp {
			return ""
		}
	}
	return strings.TrimSuffix(value, ".h")
}

func jsDependency(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"' `))
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if len(parts) > 0 {
		value = parts[len(parts)-1]
	}
	return jsExtension.ReplaceAllString(value, "")
}

func pythonDependency(value string) string {
	value = strings.TrimSpace(strings.Split(value, ".")[0])
	switch strings.ToLower(value) {
	case "os", "sys":
		return ""
	}
	if strings.HasPrefix(value, "__") && strings.HasSuffix(value, "__") {
		return ""
	}
	return value
}

func processPHPDependencies(line string, deps *dependencyCollector) {
	if match := phpInclude.FindStringSubmatch(line); len(match) == 2 {
		deps.add(phpDependency("'" + match[1] + "'"))
		return
	}

	statement := strings.TrimSpace(line)
	if !strings.HasPrefix(statement, "use ") {
		return
	}
	statement = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(statement, "use "), ";"))
	switch {
	case strings.HasPrefix(statement, "const "):
		return
	case strings.HasPrefix(statement, "function "):
		statement = strings.TrimSpace(strings.TrimPrefix(statement, "function "))
	}

	for _, part := range strings.Split(statement, ",") {
		deps.add(phpDependency(part))
	}
}

func phpDependency(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if before, _, ok := cutCaseInsensitive(value, " as "); ok {
		value = before
	}
	first := strings.Split(strings.TrimSpace(value), `\`)[0]
	lower := strings.ToLower(first)
	if lower == "app" || lower == "app.php" {
		return ""
	}
	return first
}

func swiftDependency(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "Foundation") {
		return ""
	}
	return value
}

func cutCaseInsensitive(value, sep string) (before, after string, ok bool) {
	index := strings.Index(strings.ToLower(value), strings.ToLower(sep))
	if index < 0 {
		return "", "", false
	}
	return value[:index], value[index+len(sep):], true
}

func objectiveCDependency(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'<> `))
	value = strings.Split(value, "/")[0]
	value = strings.TrimSuffix(value, ".h")
	value = strings.TrimSuffix(value, ".m")
	return strings.TrimSpace(value)
}

func vbnetDependency(value string) string {
	value = strings.TrimSpace(value)
	if _, after, ok := strings.Cut(value, "="); ok {
		value = after
	}
	first := strings.Split(strings.TrimSpace(value), ".")[0]
	switch strings.ToLower(first) {
	case "system", "microsoft":
		return ""
	default:
		return first
	}
}

func firstN(values []string, n int) []string {
	if len(values) < n {
		return values
	}
	return values[:n]
}
