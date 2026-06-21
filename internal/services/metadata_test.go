package services

import "testing"

func TestKnownProgramLanguagesIncludesBroadColorCatalog(t *testing.T) {
	languages := KnownProgramLanguages()
	if len(languages) < 25 {
		t.Fatalf("expected broad language catalog, got %d entries", len(languages))
	}

	colors := map[string]string{}
	for _, language := range languages {
		colors[language.Name] = language.Color
	}
	for name, want := range map[string]string{
		"Go":         "#00ADD8",
		"TypeScript": "#3178C6",
		"Python":     "#3572A5",
		"Ruby":       "#701516",
		"Rust":       "#DEA584",
	} {
		if colors[name] != want {
			t.Fatalf("expected %s color %s, got %s", name, want, colors[name])
		}
	}
}

func TestKnownEditorsIncludesCommonWakaTimePlugins(t *testing.T) {
	editors := KnownEditors()
	keys := map[string]bool{}
	for _, editor := range editors {
		keys[editor.Key] = true
	}
	for _, key := range []string{"vscode", "cursor", "zed", "neovim", "intellij", "sublime", "emacs"} {
		if !keys[key] {
			t.Fatalf("expected editor catalog to include %s", key)
		}
	}
}
