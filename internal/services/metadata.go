package services

type EditorMetadata struct {
	Name    string `json:"name"`
	Key     string `json:"key"`
	Version string `json:"version,omitempty"`
}

type ProgramLanguage struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func KnownEditors() []EditorMetadata {
	return []EditorMetadata{
		{Name: "VS Code", Key: "vscode", Version: "1.89+"},
		{Name: "Cursor", Key: "cursor", Version: "0.40+"},
		{Name: "Zed", Key: "zed", Version: "0.140+"},
		{Name: "Neovim", Key: "neovim", Version: "0.9+"},
		{Name: "Vim", Key: "vim", Version: "8+"},
		{Name: "IntelliJ IDEA", Key: "intellij", Version: "2024+"},
		{Name: "GoLand", Key: "goland", Version: "2024+"},
		{Name: "PyCharm", Key: "pycharm", Version: "2024+"},
		{Name: "WebStorm", Key: "webstorm", Version: "2024+"},
		{Name: "Sublime Text", Key: "sublime", Version: "4+"},
		{Name: "Emacs", Key: "emacs", Version: "29+"},
		{Name: "Xcode", Key: "xcode", Version: "15+"},
		{Name: "Android Studio", Key: "androidstudio", Version: "2024+"},
	}
}

func KnownProgramLanguages() []ProgramLanguage {
	return []ProgramLanguage{
		{Name: "Go", Color: "#00ADD8"},
		{Name: "TypeScript", Color: "#3178C6"},
		{Name: "JavaScript", Color: "#F1E05A"},
		{Name: "Python", Color: "#3572A5"},
		{Name: "Rust", Color: "#DEA584"},
		{Name: "Ruby", Color: "#701516"},
		{Name: "Java", Color: "#B07219"},
		{Name: "C", Color: "#555555"},
		{Name: "C++", Color: "#F34B7D"},
		{Name: "C#", Color: "#178600"},
		{Name: "PHP", Color: "#4F5D95"},
		{Name: "Swift", Color: "#F05138"},
		{Name: "Kotlin", Color: "#A97BFF"},
		{Name: "Dart", Color: "#00B4AB"},
		{Name: "Scala", Color: "#C22D40"},
		{Name: "Elixir", Color: "#6E4A7E"},
		{Name: "Erlang", Color: "#B83998"},
		{Name: "Clojure", Color: "#DB5855"},
		{Name: "F#", Color: "#B845FC"},
		{Name: "Haskell", Color: "#5E5086"},
		{Name: "Lua", Color: "#000080"},
		{Name: "R", Color: "#198CE7"},
		{Name: "Shell", Color: "#89E051"},
		{Name: "PowerShell", Color: "#012456"},
		{Name: "HTML", Color: "#E34C26"},
		{Name: "CSS", Color: "#563D7C"},
		{Name: "SCSS", Color: "#C6538C"},
		{Name: "SQL", Color: "#E38C00"},
		{Name: "GraphQL", Color: "#E10098"},
		{Name: "JSON", Color: "#292929"},
		{Name: "YAML", Color: "#CB171E"},
		{Name: "TOML", Color: "#9C4221"},
		{Name: "Markdown", Color: "#083FA1"},
		{Name: "Dockerfile", Color: "#384D54"},
		{Name: "Terraform", Color: "#7B42BC"},
		{Name: "Nix", Color: "#7E7EFF"},
		{Name: "Objective-C", Color: "#438EFF"},
		{Name: "Perl", Color: "#0298C3"},
		{Name: "Racket", Color: "#3C5CAA"},
		{Name: "Vim Script", Color: "#199F4B"},
		{Name: "Vue", Color: "#41B883"},
		{Name: "Svelte", Color: "#FF3E00"},
	}
}
