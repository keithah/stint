package stintcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kevinburke/ssh_config"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	remoteDownloadMaxBytes = 512000
	remoteDefaultPort      = 22
	remoteTimeout          = 20 * time.Second
)

type remoteFileClient struct {
	User         string
	Pass         string
	OriginalHost string
	Host         string
	Port         int
	Path         string
}

var remoteFileDownload = downloadRemoteFile

func prepareRemoteStatsFile(entity string) (string, func(), error) {
	client, err := parseRemoteFileClient(entity)
	if err != nil {
		return "", nil, err
	}
	tmp, err := os.CreateTemp("", "*_"+filepath.Base(client.Path))
	if err != nil {
		return "", nil, fmt.Errorf("create remote temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", nil, fmt.Errorf("close remote temp file: %w", err)
	}
	if err := remoteFileDownload(client, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", nil, err
	}
	return tmpPath, func() { _ = os.Remove(tmpPath) }, nil
}

func parseRemoteFileClient(entity string) (remoteFileClient, error) {
	parsed, err := url.Parse(entity)
	if err != nil {
		return remoteFileClient{}, fmt.Errorf("parse remote file url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "ssh" && scheme != "sftp" {
		return remoteFileClient{}, fmt.Errorf("unsupported remote file scheme %q", parsed.Scheme)
	}
	port := remoteDefaultPort
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil {
			return remoteFileClient{}, fmt.Errorf("parse remote file port: %w", err)
		}
	}
	host := parsed.Hostname()
	if host == "" {
		return remoteFileClient{}, fmt.Errorf("remote file url must include host")
	}
	pass, _ := parsed.User.Password()
	originalHost := host
	if configuredHost := strings.TrimSpace(sshConfigValue(originalHost, "HostName")); configuredHost != "" {
		host = configuredHost
	}
	user := parsed.User.Username()
	if user == "" {
		user = strings.TrimSpace(sshConfigValueForHosts("User", originalHost, host))
	}
	if parsed.Port() == "" {
		if configuredPort := strings.TrimSpace(sshConfigValueForHosts("Port", originalHost, host)); configuredPort != "" {
			port, err = strconv.Atoi(configuredPort)
			if err != nil {
				return remoteFileClient{}, fmt.Errorf("parse remote file port from ssh config: %w", err)
			}
		}
	}
	return remoteFileClient{
		User:         user,
		Pass:         pass,
		OriginalHost: originalHost,
		Host:         host,
		Port:         port,
		Path:         parsed.Path,
	}, nil
}

func sshConfigValueForHosts(key string, hosts ...string) string {
	seen := map[string]struct{}{}
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		if value := sshConfigValue(host, key); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sshConfigValue(host, key string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return ""
	}
	defer f.Close()
	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return ""
	}
	value, err := cfg.Get(host, key)
	if err != nil {
		return ""
	}
	return value
}

func sshConfigValues(host, key string) []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return nil
	}
	defer f.Close()
	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil
	}
	values, err := cfg.GetAll(host, key)
	if err != nil {
		return nil
	}
	return values
}

func downloadRemoteFile(client remoteFileClient, localPath string) error {
	if err := downloadRemoteFileSFTP(client, localPath); err != nil {
		if !shouldFallbackRemoteDownload(err) {
			return err
		}
		if fallbackErr := downloadRemoteFileSCP(client, localPath); fallbackErr != nil {
			return fmt.Errorf("%w; scp fallback failed: %v", err, fallbackErr)
		}
	}
	return nil
}

func shouldFallbackRemoteDownload(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return !strings.Contains(msg, "host key mismatch") && !strings.Contains(msg, "key mismatch")
}

func downloadRemoteFileSFTP(client remoteFileClient, localPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()
	sshClient, err := sshDial(ctx, client)
	if err != nil {
		return fmt.Errorf("connect remote file over ssh: %w", err)
	}
	defer sshClient.Close()
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("start sftp subsystem: %w", err)
	}
	defer sftpClient.Close()
	remote, err := sftpClient.OpenFile(client.Path, os.O_RDONLY)
	if err != nil {
		return fmt.Errorf("open remote file: %w", err)
	}
	defer remote.Close()
	local, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("open local remote temp file: %w", err)
	}
	defer local.Close()
	if _, err := io.CopyN(local, remote, remoteDownloadMaxBytes); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("download remote file: %w", err)
	}
	return nil
}

func sshDial(ctx context.Context, client remoteFileClient) (*ssh.Client, error) {
	config, err := remoteSSHConfig(client)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(client.Host, strconv.Itoa(client.Port))
	dialer := net.Dialer{Timeout: remoteTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}

func remoteSSHConfig(client remoteFileClient) (*ssh.ClientConfig, error) {
	auths := remoteSSHAuthMethods(client)
	callback, err := remoteHostKeyCallback(client)
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User:            first(client.User, os.Getenv("USER")),
		Auth:            auths,
		HostKeyCallback: callback,
		Timeout:         remoteTimeout,
	}, nil
}

func remoteSSHAuthMethods(client remoteFileClient) []ssh.AuthMethod {
	var auths []ssh.AuthMethod
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	for _, keyPath := range remoteSSHIdentityFiles(client) {
		if signer, err := signerFromFile(keyPath); err == nil {
			auths = append(auths, ssh.PublicKeys(signer))
		}
	}
	if client.Pass != "" {
		auths = append(auths, ssh.Password(client.Pass))
	}
	return auths
}

func remoteSSHIdentityFiles(client remoteFileClient) []string {
	var files []string
	seen := map[string]struct{}{}
	for _, host := range []string{client.OriginalHost, client.Host} {
		for _, keyPath := range sshConfigValues(host, "IdentityFile") {
			expanded, err := expandHomeStrict(keyPath)
			if err != nil {
				continue
			}
			if _, err := os.Stat(expanded); err != nil {
				continue
			}
			if _, ok := seen[expanded]; ok {
				continue
			}
			seen[expanded] = struct{}{}
			files = append(files, expanded)
		}
	}
	files = append(files, defaultSSHIdentityFiles()...)
	return files
}

func defaultSSHIdentityFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
		filepath.Join(home, ".ssh", "id_rsa"),
	}
}

func signerFromFile(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(key)
}

func remoteHostKeyCallback(client remoteFileClient) (ssh.HostKeyCallback, error) {
	strict := remoteStrictHostKeyChecking(client)
	if strict == "no" {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // WakaTime-compatible explicit SSH config opt-out.
	}
	files := remoteKnownHostsFiles(client)
	if len(files) == 0 && strict == "yes" {
		return nil, fmt.Errorf("known host key not found for %s, will not connect", client.OriginalHost)
	}
	if len(files) == 0 {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // Matches WakaTime's permissive default when no known_hosts exists.
	}
	callback, err := knownhosts.New(files...)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}
	if alias := remoteHostKeyAlias(client); alias != "" {
		aliasHost := net.JoinHostPort(alias, strconv.Itoa(client.Port))
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			if err := callback(hostname, remote, key); err == nil {
				return nil
			}
			return callback(aliasHost, remote, key)
		}, nil
	}
	return callback, nil
}

func remoteHostKeyAlias(client remoteFileClient) string {
	alias := strings.TrimSpace(sshConfigValueForHosts("HostKeyAlias", client.OriginalHost, client.Host))
	if alias == "" {
		return ""
	}
	expanded, err := expandHomeStrict(alias)
	if err != nil {
		return alias
	}
	return expanded
}

func remoteStrictHostKeyChecking(client remoteFileClient) string {
	strict := strings.TrimSpace(sshConfigValueForHosts("StrictHostKeyChecking", client.OriginalHost, client.Host))
	if strict == "" {
		strict = "ask"
	}
	strict = strings.ToLower(strict)
	if strict == "accept-new" || strict == "off" {
		return "no"
	}
	return strict
}

func remoteKnownHostsFiles(client remoteFileClient) []string {
	var files []string
	seen := map[string]struct{}{}
	for _, host := range []string{client.OriginalHost, client.Host} {
		for _, knownHostsPath := range sshConfigValues(host, "UserKnownHostsFile") {
			expanded, err := expandHomeStrict(knownHostsPath)
			if err != nil {
				continue
			}
			if _, err := os.Stat(expanded); err != nil {
				continue
			}
			if _, ok := seen[expanded]; ok {
				continue
			}
			seen[expanded] = struct{}{}
			files = append(files, expanded)
		}
	}
	for _, knownHostsPath := range knownHostsFiles() {
		if _, ok := seen[knownHostsPath]; ok {
			continue
		}
		seen[knownHostsPath] = struct{}{}
		files = append(files, knownHostsPath)
	}
	return files
}

func knownHostsFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "known_hosts"),
		filepath.Join(home, ".ssh", "known_hosts2"),
	}
	var files []string
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			files = append(files, candidate)
		}
	}
	return files
}

func downloadRemoteFileSCP(client remoteFileClient, localPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()
	args := []string{"-B"}
	if client.Port != remoteDefaultPort {
		args = append(args, "-P", strconv.Itoa(client.Port))
	}
	args = append(args, client.scpSource(), localPath)
	cmd := exec.CommandContext(ctx, "scp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (client remoteFileClient) scpSource() string {
	host := client.Host
	if client.User != "" {
		host = client.User + "@" + host
	}
	return host + ":" + client.Path
}
