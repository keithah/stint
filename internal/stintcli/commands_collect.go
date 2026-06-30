package stintcli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

var executablePath = os.Executable

func runCollect(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if path, err := collectHelperPath(); err == nil {
		cmd := exec.Command(path, args...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	}
	if fileExists("go.mod") && fileExists(filepath.Join("cmd", "collect", "main.go")) {
		goArgs := append([]string{"run", "./cmd/collect"}, args...)
		cmd := exec.Command("go", goArgs...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	}
	return fmt.Errorf("stint-collect is not installed; run `make collect-install` or use `go run ./cmd/collect ...` from the repository")
}

func collectHelperPath() (string, error) {
	if path, err := exec.LookPath("stint-collect"); err == nil {
		return path, nil
	}
	exe, err := executablePath()
	if err != nil || exe == "" {
		return "", fmt.Errorf("stint-collect not found")
	}
	candidate := filepath.Join(filepath.Dir(exe), "stint-collect")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
		return candidate, nil
	}
	return "", fmt.Errorf("stint-collect not found")
}
