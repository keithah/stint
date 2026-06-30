package stintcli

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"
)

func TestBuildHeartbeatKeepsRemoteEntityAndUsesLocalFileForStats(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".wakatime-project"), []byte("remote-project\nremote-dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	localFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(localFile, []byte("package main\nimport \"github.com/acme/pkg\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	remoteEntity := "ssh://user@example.org/home/me/project/main.go"
	hb, err := BuildHeartbeat(Options{
		Category:   "coding",
		Entity:     remoteEntity,
		EntityType: "file",
		LocalFile:  localFile,
		Write:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "ssh://example.org/home/me/project/main.go" {
		t.Fatalf("entity = %q", hb.Entity)
	}
	if hb.Project != "remote-project" || hb.Branch != "remote-dev" {
		t.Fatalf("unexpected project/branch from local file: %#v", hb)
	}
	if hb.Language != "Go" {
		t.Fatalf("language = %q", hb.Language)
	}
	if hb.Lines == nil || *hb.Lines != 2 {
		t.Fatalf("lines = %#v", hb.Lines)
	}
	if strings.Join(hb.Dependencies, ",") != "github.com/acme/pkg" {
		t.Fatalf("dependencies = %#v", hb.Dependencies)
	}
}

func TestBuildHeartbeatDownloadsRemoteEntityWithoutLocalFile(t *testing.T) {
	original := remoteFileDownload
	var downloadedPath string
	remoteFileDownload = func(client remoteFileClient, localPath string) error {
		if client.Host != "example.org" || client.Path != "/home/me/project/main.go" {
			t.Fatalf("unexpected remote client: %#v", client)
		}
		downloadedPath = localPath
		return os.WriteFile(localPath, []byte("package main\nimport \"github.com/acme/pkg\"\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })

	remoteEntity := "sftp://user@example.org/home/me/project/main.go"
	hb, err := BuildHeartbeat(Options{
		Category:   "coding",
		Entity:     remoteEntity,
		EntityType: "file",
		Write:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "sftp://example.org/home/me/project/main.go" || !hb.IsWrite {
		t.Fatalf("unexpected remote heartbeat: %#v", hb)
	}
	if hb.Language != "Go" || hb.Lines == nil || *hb.Lines != 2 {
		t.Fatalf("remote stats not applied: %#v", hb)
	}
	if strings.Join(hb.Dependencies, ",") != "github.com/acme/pkg" {
		t.Fatalf("dependencies = %#v", hb.Dependencies)
	}
	if downloadedPath == "" {
		t.Fatalf("expected remote downloader to run")
	}
	if _, err := os.Stat(downloadedPath); !os.IsNotExist(err) {
		t.Fatalf("remote temp file was not cleaned up: %s err=%v", downloadedPath, err)
	}
}

func TestBuildHeartbeatRemoteEntitySkipsProjectFileFilterLikeWakaTime(t *testing.T) {
	original := remoteFileDownload
	remoteFileDownload = func(_ remoteFileClient, localPath string) error {
		return os.WriteFile(localPath, []byte("package main\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })
	hb, err := BuildHeartbeat(Options{
		Entity:                 "ssh://example.test/home/me/main.go",
		EntityType:             "file",
		IncludeOnlyProjectFile: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "ssh://example.test/home/me/main.go" {
		t.Fatalf("entity = %q", hb.Entity)
	}
}

func TestBuildHeartbeatUnsavedRemoteEntityDoesNotBypassProjectFileFilterLikeWakaTime(t *testing.T) {
	_, err := BuildHeartbeat(Options{
		Entity:                 "ssh://example.test/home/me/main.go",
		EntityType:             "file",
		IsUnsavedEntity:        true,
		IncludeOnlyProjectFile: true,
		Category:               "coding",
	})
	if err == nil || !strings.Contains(err.Error(), "project has no .wakatime-project") {
		t.Fatalf("expected unsaved remote-looking entity to require project file like WakaTime, got %v", err)
	}
}

func TestParseRemoteFileClient(t *testing.T) {
	client, err := parseRemoteFileClient("ssh://alice:secret@example.org:222/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if client.User != "alice" || client.Pass != "secret" || client.Host != "example.org" || client.Port != 222 || client.Path != "/home/me/main.go" {
		t.Fatalf("unexpected remote client: %#v", client)
	}
	client, err = parseRemoteFileClient("sftp://example.org/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if client.Port != 22 || client.User != "" || client.Path != "/home/me/main.go" {
		t.Fatalf("unexpected default remote client: %#v", client)
	}
	client, err = parseRemoteFileClient("sftp://example.org")
	if err != nil {
		t.Fatal(err)
	}
	if client.Host != "example.org" || client.Port != 22 || client.Path != "" {
		t.Fatalf("unexpected host-only remote client: %#v", client)
	}
}

func TestParseRemoteFileClientUsesSSHConfigHostAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host prod\n  HostName prod.example.org\n  User deploy\n  Port 2222\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("sftp://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if client.Host != "prod.example.org" || client.User != "deploy" || client.Port != 2222 || client.Path != "/home/me/main.go" {
		t.Fatalf("unexpected ssh config remote client: %#v", client)
	}
}

func TestParseRemoteFileClientUsesDerivedHostSSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\nHost prod.example.org\n  User deploy\n  Port 2222\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if client.Host != "prod.example.org" || client.User != "deploy" || client.Port != 2222 {
		t.Fatalf("unexpected derived-host ssh config remote client: %#v", client)
	}
}

func TestRemoteSSHIdentityFilesUseSSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identityFile := filepath.Join(sshDir, "prod_key")
	if err := os.WriteFile(identityFile, []byte("not a real key"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\nHost prod.example.org\n  IdentityFile " + identityFile + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	identityFiles := remoteSSHIdentityFiles(client)
	if len(identityFiles) == 0 || identityFiles[0] != identityFile {
		t.Fatalf("identity files = %#v, want first %q", identityFiles, identityFile)
	}
}

func TestRemoteKnownHostsFilesUseSSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	knownHostsFile := filepath.Join(sshDir, "prod_known_hosts")
	if err := os.WriteFile(knownHostsFile, []byte("prod.example.org ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForPathSelectionOnly\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\nHost prod.example.org\n  UserKnownHostsFile " + knownHostsFile + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	files := remoteKnownHostsFiles(client)
	if len(files) == 0 || files[0] != knownHostsFile {
		t.Fatalf("known hosts files = %#v, want first %q", files, knownHostsFile)
	}
}

func TestRemoteHostKeyCallbackHonorsStrictHostKeyCheckingNo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	knownPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	knownPublicKey, err := ssh.NewPublicKey(knownPublic)
	if err != nil {
		t.Fatal(err)
	}
	knownHostsLine := "other.example.org " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(knownPublicKey))) + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(knownHostsLine), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\n  StrictHostKeyChecking no\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	callback, err := remoteHostKeyCallback(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("prod.example.org:22", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, publicKey); err != nil {
		t.Fatalf("StrictHostKeyChecking no should accept unknown key, got %v", err)
	}
}

func TestRemoteHostKeyCallbackRejectsMissingKnownHostWhenStrictYes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\n  StrictHostKeyChecking yes\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remoteHostKeyCallback(client); err == nil || !strings.Contains(err.Error(), "known host key not found") {
		t.Fatalf("StrictHostKeyChecking yes without known host should fail like WakaTime, got %v", err)
	}
}

func TestRemoteHostKeyCallbackHonorsHostKeyAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	knownHostsLine := "prod-alias " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))) + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(knownHostsLine), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\n  HostKeyAlias prod-alias\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	callback, err := remoteHostKeyCallback(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("prod.example.org:22", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, publicKey); err != nil {
		t.Fatalf("HostKeyAlias should match known_hosts alias, got %v", err)
	}
}

func TestRemoteDownloadFallbackPolicy(t *testing.T) {
	if shouldFallbackRemoteDownload(fmt.Errorf("ssh: handshake failed")) != true {
		t.Fatalf("expected generic ssh failure to allow scp fallback")
	}
	if shouldFallbackRemoteDownload(fmt.Errorf("ssh: host key mismatch")) != false {
		t.Fatalf("host key mismatch must not fall back to scp")
	}
	if shouldFallbackRemoteDownload(fmt.Errorf("knownhosts: key mismatch")) != false {
		t.Fatalf("knownhosts key mismatch must not fall back to scp")
	}
}

func TestBuildHeartbeatHidesProjectFolderAndRemoteCredentials(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", HideProjectFolder: true, ProjectFolder: dir})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "main.go" || hb.ProjectRootCount != nil {
		t.Fatalf("project folder was not hidden: %#v", hb)
	}

	original := remoteFileDownload
	remoteFileDownload = func(_ remoteFileClient, localPath string) error {
		return os.WriteFile(localPath, []byte("package main\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })
	remoteHB, err := BuildHeartbeat(Options{Entity: "ssh://alice:secret@example.org/home/me/main.go", EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if remoteHB.Entity != "ssh://example.org/home/me/main.go" {
		t.Fatalf("remote credentials were not hidden: %q", remoteHB.Entity)
	}
}

func TestGitProjectFromGitRemote(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "config"), []byte("[remote \"origin\"]\n\turl = git@github.com:keithah/stint.git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("git", "project_from_git_remote", "true")
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "keithah/stint" {
		t.Fatalf("project = %q", hb.Project)
	}
}

func TestWakaTimeProjectPlaceholderUsesGitRemoteProjectLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".wakatime-project"), []byte("custom/{project}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "config"), []byte("[remote \"origin\"]\n\turl = git@github.com:keithah/stint.git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("git", "project_from_git_remote", "true")

	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "custom/keithah/stint" {
		t.Fatalf("project = %q", hb.Project)
	}
}

func TestProjectFromRemoteURLHandlesHTTPS(t *testing.T) {
	if got := projectFromRemoteURL("https://github.com/keithah/stint.git"); got != "keithah/stint" {
		t.Fatalf("project = %q", got)
	}
}

func TestProcessExtraHeartbeatRemoteEntitySkipsProjectFileFilterLikeWakaTime(t *testing.T) {
	original := remoteFileDownload
	remoteFileDownload = func(_ remoteFileClient, localPath string) error {
		return os.WriteFile(localPath, []byte("package main\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })
	hb, skip, err := processExtraHeartbeat(Heartbeat{
		Entity:     "ssh://example.test/home/me/main.go",
		EntityType: "file",
		Time:       123,
	}, Options{IncludeOnlyProjectFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatal("remote extra heartbeat was unexpectedly filtered")
	}
	if hb.Entity != "ssh://example.test/home/me/main.go" {
		t.Fatalf("entity = %q", hb.Entity)
	}
}

func TestProcessExtraHeartbeatDownloadsRemoteEntityWithoutLocalFile(t *testing.T) {
	original := remoteFileDownload
	var downloadedPath string
	remoteFileDownload = func(client remoteFileClient, localPath string) error {
		if client.Host != "example.test" || client.Path != "/tmp/remote.py" {
			t.Fatalf("unexpected remote client: %#v", client)
		}
		downloadedPath = localPath
		return os.WriteFile(localPath, []byte("import requests\nprint('ok')\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })

	hb, skip, err := processExtraHeartbeat(Heartbeat{
		Entity:     "sftp://user:secret@example.test/tmp/remote.py",
		EntityType: "file",
		Time:       123,
	}, Options{Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatal("remote extra heartbeat was unexpectedly skipped")
	}
	if hb.Entity != "sftp://example.test/tmp/remote.py" {
		t.Fatalf("entity = %q", hb.Entity)
	}
	if hb.Language != "Python" || hb.Lines == nil || *hb.Lines != 2 || len(hb.Dependencies) == 0 {
		t.Fatalf("remote stats were not applied: %#v", hb)
	}
	if downloadedPath == "" {
		t.Fatal("expected remote downloader to run")
	}
	if _, err := os.Stat(downloadedPath); !os.IsNotExist(err) {
		t.Fatalf("remote temp file was not cleaned up: %s err=%v", downloadedPath, err)
	}
}
