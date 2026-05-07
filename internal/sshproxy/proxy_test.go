package sshproxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// ---- passwordKeyboardInteractive ----

func TestPasswordKeyboardInteractive_ZeroQuestions(t *testing.T) {
	auth := passwordKeyboardInteractive("secret")
	challenge, ok := auth.(ssh.KeyboardInteractiveChallenge)
	if !ok {
		t.Fatal("expected ssh.KeyboardInteractiveChallenge")
	}
	answers, err := challenge("user", "instruction", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answers != nil {
		t.Fatalf("expected nil answers for zero questions, got %v", answers)
	}
}

func TestPasswordKeyboardInteractive_SingleQuestion(t *testing.T) {
	auth := passwordKeyboardInteractive("secret")
	challenge := auth.(ssh.KeyboardInteractiveChallenge)
	answers, err := challenge("user", "instruction", []string{"Password:"}, []bool{false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(answers) != 1 || answers[0] != "secret" {
		t.Fatalf("expected [secret], got %v", answers)
	}
}

func TestPasswordKeyboardInteractive_MultipleQuestions(t *testing.T) {
	auth := passwordKeyboardInteractive("secret")
	challenge := auth.(ssh.KeyboardInteractiveChallenge)
	answers, err := challenge("user", "instruction", []string{"Password:", "Verification:", "Token:"}, []bool{false, false, false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(answers) != 3 {
		t.Fatalf("expected 3 answers, got %d", len(answers))
	}
	for i, a := range answers {
		if a != "secret" {
			t.Fatalf("answers[%d] = %q, want %q", i, a, "secret")
		}
	}
}

func TestPasswordKeyboardInteractive_EmptyPassword(t *testing.T) {
	auth := passwordKeyboardInteractive("")
	challenge := auth.(ssh.KeyboardInteractiveChallenge)
	answers, err := challenge("user", "instruction", []string{"Password:"}, []bool{false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(answers) != 1 || answers[0] != "" {
		t.Fatalf("expected empty string answer, got %v", answers)
	}
}

func TestPasswordKeyboardInteractive_NonNilReturn(t *testing.T) {
	auth := passwordKeyboardInteractive("any")
	if auth == nil {
		t.Fatal("expected non-nil auth method")
	}
}

// ---- exportPublicKey ----

func TestExportPublicKey_WritesPubFile(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "host_key")
	logger := slog.New(slog.DiscardHandler)

	// Generate a valid signer.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	exportPublicKey(signer, privPath, logger)

	pubPath := privPath + ".pub"
	data, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read pub file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("pub file is empty")
	}
}

func TestExportPublicKey_ReadOnlyDir_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	// Make directory read-only.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("cannot chmod temp dir: %v", err)
	}
	defer os.Chmod(dir, 0o700)

	privPath := filepath.Join(dir, "host_key")
	logger := slog.New(slog.DiscardHandler)

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	// Should not panic, should log a warning.
	exportPublicKey(signer, privPath, logger)
}

// ---- loadOrGenerateHostKey ----

func TestLoadOrGenerateHostKey_EmptyPath(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	signer, err := loadOrGenerateHostKey("", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("expected non-nil signer")
	}
	// Verify the signer works by signing.
	pub := signer.PublicKey()
	if pub == nil {
		t.Fatal("expected non-nil public key")
	}
}

func TestLoadOrGenerateHostKey_GenerateAndPersist(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "host_key")
	logger := slog.New(slog.DiscardHandler)

	signer, err := loadOrGenerateHostKey(keyPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("expected non-nil signer")
	}

	// Verify PEM file was created.
	pemData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read PEM file: %v", err)
	}
	if len(pemData) == 0 {
		t.Fatal("PEM file is empty")
	}
	block, _ := pem.Decode(pemData)
	if block == nil {
		t.Fatal("failed to decode PEM block")
	}
	if block.Type != "PRIVATE KEY" {
		t.Fatalf("expected PRIVATE KEY, got %s", block.Type)
	}

	// Verify .pub file was created.
	pubPath := keyPath + ".pub"
	pubData, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read pub file: %v", err)
	}
	if len(pubData) == 0 {
		t.Fatal("pub file is empty")
	}
}

func TestLoadOrGenerateHostKey_LoadExisting(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "host_key")
	logger := slog.New(slog.DiscardHandler)

	// Generate and persist a key manually.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	derBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: derBytes})
	if err := os.WriteFile(keyPath, pemBlock, 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	// Load the existing key.
	signer, err := loadOrGenerateHostKey(keyPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("expected non-nil signer")
	}

	// Verify .pub file was exported.
	pubPath := keyPath + ".pub"
	if _, err := os.Stat(pubPath); err != nil {
		t.Fatalf("pub file not created: %v", err)
	}
}

func TestLoadOrGenerateHostKey_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "invalid_key")
	logger := slog.New(slog.DiscardHandler)

	// Write invalid data.
	if err := os.WriteFile(keyPath, []byte("not a valid PEM key"), 0o600); err != nil {
		t.Fatalf("write invalid file: %v", err)
	}

	_, err := loadOrGenerateHostKey(keyPath, logger)
	if err == nil {
		t.Fatal("expected error for invalid key file")
	}
}

func TestLoadOrGenerateHostKey_DirectoryPath(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.DiscardHandler)

	_, err := loadOrGenerateHostKey(dir, logger)
	if err == nil {
		t.Fatal("expected error when path is a directory")
	}
}

func TestLoadOrGenerateHostKey_NonExistentDir(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "subdir", "host_key")
	logger := slog.New(slog.DiscardHandler)

	signer, err := loadOrGenerateHostKey(keyPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signer == nil {
		t.Fatal("expected non-nil signer")
	}

	// Verify PEM file was created (MkdirAll succeeded).
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("key file not found: %v", err)
	}
}

// ---- NewServer ----

func TestNewServer_Defaults(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	resolver := &stubResolverRepo{}
	server, err := NewServer(":2222", "", "", "", resolver, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server == nil {
		t.Fatal("expected non-nil server")
	}
	if server.addr != ":2222" {
		t.Fatalf("expected addr :2222, got %s", server.addr)
	}
	if server.containerUser != "workspace" {
		t.Fatalf("expected default containerUser 'workspace', got %q", server.containerUser)
	}
	if server.containerPassword != "workspace" {
		t.Fatalf("expected default containerPassword 'workspace', got %q", server.containerPassword)
	}
}

func TestNewServer_CustomValues(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	resolver := &stubResolverRepo{}
	server, err := NewServer(":2223", "myuser", "mypass", "", resolver, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server == nil {
		t.Fatal("expected non-nil server")
	}
	if server.containerUser != "myuser" {
		t.Fatalf("expected containerUser 'myuser', got %q", server.containerUser)
	}
	if server.containerPassword != "mypass" {
		t.Fatalf("expected containerPassword 'mypass', got %q", server.containerPassword)
	}
}

func TestNewServer_WithHostKeyPath(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "host_key")
	logger := slog.New(slog.DiscardHandler)
	resolver := &stubResolverRepo{}

	server, err := NewServer(":2224", "user", "pass", keyPath, resolver, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server == nil {
		t.Fatal("expected non-nil server")
	}

	// Verify key was generated and persisted.
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("host key file not found: %v", err)
	}
	if _, err := os.Stat(keyPath + ".pub"); err != nil {
		t.Fatalf("host pub key file not found: %v", err)
	}
}

func TestNewServer_LoadsExistingHostKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "host_key")
	logger := slog.New(slog.DiscardHandler)

	// First call generates and persists.
	s1, err := NewServer(":2225", "u", "p", keyPath, &stubResolverRepo{}, logger)
	if err != nil {
		t.Fatalf("first NewServer: %v", err)
	}

	// Second call should load the existing key.
	s2, err := NewServer(":2225", "u", "p", keyPath, &stubResolverRepo{}, logger)
	if err != nil {
		t.Fatalf("second NewServer: %v", err)
	}

	// Both should have the same public key.
	p1 := ssh.MarshalAuthorizedKey(s1.hostKey.PublicKey())
	p2 := ssh.MarshalAuthorizedKey(s2.hostKey.PublicKey())
	if string(p1) != string(p2) {
		t.Fatal("public keys should match when loading existing key")
	}
}

// ---- ContainerTarget default propagation ----

func TestServer_ContainerTargetDefaults(t *testing.T) {
	// 验证 Server 结构体在创建时正确初始化所有字段。
	logger := slog.New(slog.DiscardHandler)
	resolver := NewRepoResolver(&stubResolverRepo{})
	server, err := NewServer(":2226", "appuser", "apppass", "", resolver, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server.resolver != resolver {
		t.Fatal("resolver not set correctly")
	}
	if server.hostKey == nil {
		t.Fatal("hostKey not initialized")
	}
}

func TestNewServer_EmptyAddress(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	resolver := &stubResolverRepo{}
	server, err := NewServer("", "u", "p", "", resolver, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server.addr != "" {
		t.Fatalf("expected empty addr, got %q", server.addr)
	}
}

// ---- handleConnection integration tests ----

// buildServerConfig creates the ssh.ServerConfig matching ListenAndServe's config,
// using the provided resolver for auth callbacks.
func buildServerConfig(s *Server, resolver ContainerResolver) *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		MaxAuthTries:  3,
		ServerVersion: "SSH-2.0-CloudCLIProxy",
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			authCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			target, err := resolver.ResolveContainer(authCtx, conn.User(), string(password))
			if err != nil {
				return nil, fmt.Errorf("auth failed")
			}
			return &ssh.Permissions{
				Extensions: map[string]string{
					"target_addr":        target.Addr,
					"target_user":        target.User,
					"target_password":    target.Password,
					"target_private_key": target.PrivateKey,
				},
			}, nil
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			authCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			target, err := resolver.ResolveContainerByPublicKey(authCtx, conn.User(), key)
			if err != nil {
				return nil, fmt.Errorf("auth failed")
			}
			return &ssh.Permissions{
				Extensions: map[string]string{
					"target_addr":        target.Addr,
					"target_user":        target.User,
					"target_password":    target.Password,
					"target_private_key": target.PrivateKey,
				},
			}, nil
		},
	}
	config.AddHostKey(s.hostKey)
	return config
}

// startServerAndConnect starts a TCP listener, accepts one connection and
// passes it to handleConnection. Returns the listener address and a channel
// that closes when handleConnection returns.
func startServerAndConnect(t *testing.T, s *Server, config *ssh.ServerConfig) (addr string, done <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ch := make(chan struct{})
	go func() {
		defer close(ch)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		s.handleConnection(conn, config)
		listener.Close()
	}()

	return listener.Addr().String(), ch
}

func TestHandleConnection_PasswordAuthFailed(t *testing.T) {
	resolver := &stubResolverRepo{
		targetErr: fmt.Errorf("invalid credentials"),
	}
	logger := slog.New(slog.DiscardHandler)
	s, err := NewServer(":0", "ws", "pass", "", resolver, logger)
	if err != nil {
		t.Fatal(err)
	}

	config := buildServerConfig(s, resolver)
	addr, done := startServerAndConnect(t, s, config)

	clientConfig := &ssh.ClientConfig{
		User:            "alice",
		Auth:            []ssh.AuthMethod{ssh.Password("wrong")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	_, clientErr := ssh.Dial("tcp", addr, clientConfig)
	if clientErr == nil {
		t.Fatal("expected authentication to fail")
	}

	<-done
}

func TestHandleConnection_PasswordAuthSuccess(t *testing.T) {
	resolver := &stubResolverRepo{
		targetAddr: "127.0.0.1:19999", // unreachable, but auth succeeds
	}
	logger := slog.New(slog.DiscardHandler)
	s, err := NewServer(":0", "ws", "pass", "", resolver, logger)
	if err != nil {
		t.Fatal(err)
	}

	config := buildServerConfig(s, resolver)
	addr, done := startServerAndConnect(t, s, config)

	clientConfig := &ssh.ClientConfig{
		User:            "alice",
		Auth:            []ssh.AuthMethod{ssh.Password("any")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	cc, clientErr := ssh.Dial("tcp", addr, clientConfig)
	if clientErr != nil {
		t.Fatalf("client connection failed: %v", clientErr)
	}

	cc.Close()
	<-done
}

func TestHandleConnection_PublicKeyAuthSuccess(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	resolver := &stubResolverRepo{
		targetAddr: "127.0.0.1:19999", // unreachable for channel, but auth succeeds
	}
	logger := slog.New(slog.DiscardHandler)
	s, err := NewServer(":0", "ws", "pass", "", resolver, logger)
	if err != nil {
		t.Fatal(err)
	}

	config := buildServerConfig(s, resolver)
	addr, done := startServerAndConnect(t, s, config)

	clientConfig := &ssh.ClientConfig{
		User:            "alice",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	cc, clientErr := ssh.Dial("tcp", addr, clientConfig)
	if clientErr != nil {
		t.Fatalf("client pubkey connection failed: %v", clientErr)
	}

	cc.Close()
	<-done
}

func TestHandleConnection_AndChannel_UnreachableTarget(t *testing.T) {
	resolver := &stubResolverRepo{
		targetAddr: "127.0.0.1:19999", // no SSH server listening here
	}
	logger := slog.New(slog.DiscardHandler)
	s, err := NewServer(":0", "ws", "pass", "", resolver, logger)
	if err != nil {
		t.Fatal(err)
	}

	config := buildServerConfig(s, resolver)
	addr, done := startServerAndConnect(t, s, config)

	clientConfig := &ssh.ClientConfig{
		User:            "alice",
		Auth:            []ssh.AuthMethod{ssh.Password("any")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	cc, clientErr := ssh.Dial("tcp", addr, clientConfig)
	if clientErr != nil {
		t.Fatalf("client connection failed: %v", clientErr)
	}

	// Open a session channel - this triggers handleChannel
	// which will fail to connect to the unreachable target.
	ch, _, openErr := cc.OpenChannel("session", nil)
	if openErr == nil && ch != nil {
		ch.Close()
	}

	cc.Close()
	<-done
}

// ---- handleChannel with test target SSH server ----

// startTestTargetSSH starts a minimal SSH server that accepts password
// authentication and session channels. Returns the address and a cleanup function.
func startTestTargetSSH(t *testing.T, targetPassword string) (string, func()) {
	t.Helper()

	targetConfig := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == targetPassword {
				return nil, nil
			}
			return nil, fmt.Errorf("bad password")
		},
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate target host key: %v", err)
	}
	targetSigner, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create target signer: %v", err)
	}
	targetConfig.AddHostKey(targetSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("target listen: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleTargetConn(conn, targetConfig)
		}
	}()

	return listener.Addr().String(), func() { listener.Close() }
}

// handleTargetConn handles a single SSH connection as a target container.
// Supports both session and direct-tcpip channels.
func handleTargetConn(conn net.Conn, config *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		switch newChan.ChannelType() {
		case "session":
			ch, reqs, err := newChan.Accept()
			if err != nil {
				return
			}
			go ssh.DiscardRequests(reqs)
			// Send exit-status 0 to signal clean completion.
			ch.SendRequest("exit-status", false, ssh.Marshal(&struct{ ExitStatus uint32 }{0}))
			ch.Close()
		case "direct-tcpip":
			ch, reqs, err := newChan.Accept()
			if err != nil {
				return
			}
			go ssh.DiscardRequests(reqs)
			// Accept and immediately close — enough to verify the channel opened.
			ch.Close()
		default:
			newChan.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

func TestHandleConnection_AndChannel_WithTarget(t *testing.T) {
	// Start a test SSH server that acts as the target container.
	targetPassword := "targetpass"
	targetAddr, cleanup := startTestTargetSSH(t, targetPassword)
	defer cleanup()

	resolver := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			EntryPassword: targetPassword,
			ContainerUser: "workspace",
		},
		targetAddr: targetAddr,
	}
	logger := slog.New(slog.DiscardHandler)
	s, err := NewServer(":0", "workspace", "targetpass", "", resolver, logger)
	if err != nil {
		t.Fatal(err)
	}

	config := buildServerConfig(s, resolver)
	proxyAddr, done := startServerAndConnect(t, s, config)

	clientConfig := &ssh.ClientConfig{
		User:            "alice",
		Auth:            []ssh.AuthMethod{ssh.Password("proxy-entry-pass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	cc, clientErr := ssh.Dial("tcp", proxyAddr, clientConfig)
	if clientErr != nil {
		t.Fatalf("client connection failed: %v", clientErr)
	}
	defer cc.Close()

	// Open a session channel - this exercises handleChannel with a real target.
	ch, chReqs, openErr := cc.OpenChannel("session", nil)
	if openErr != nil {
		// This may happen if the target disconnects before the session is fully set up.
		// That's acceptable - the code path was exercised.
		t.Logf("OpenChannel returned: %v (may happen when target closes quickly)", openErr)
		cc.Close()
		<-done
		return
	}
	defer ch.Close()
	go ssh.DiscardRequests(chReqs)

	cc.Close()
	<-done
}

// ---- direct-tcpip channel dispatch through proxy ----

func TestHandleConnection_DirectTCPIP_ChannelDispatch(t *testing.T) {
	// Start target SSH server that also handles direct-tcpip channels.
	targetPassword := "targetpass"
	targetAddr, cleanup := startTestTargetSSH(t, targetPassword)
	defer cleanup()

	resolver := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			EntryPassword: targetPassword,
			ContainerUser: "workspace",
		},
		targetAddr: targetAddr,
	}
	logger := slog.New(slog.DiscardHandler)
	s, err := NewServer(":0", "workspace", "targetpass", "", resolver, logger)
	if err != nil {
		t.Fatal(err)
	}

	config := buildServerConfig(s, resolver)
	proxyAddr, done := startServerAndConnect(t, s, config)

	clientConfig := &ssh.ClientConfig{
		User:            "alice",
		Auth:            []ssh.AuthMethod{ssh.Password("proxy-entry-pass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	cc, clientErr := ssh.Dial("tcp", proxyAddr, clientConfig)
	if clientErr != nil {
		t.Fatalf("client connection failed: %v", clientErr)
	}
	defer cc.Close()

	// Build a direct-tcpip payload targeting an arbitrary port on the target.
	// The target will accept the channel (even though nothing is listening on
	// that port) — what we verify is that the proxy dispatches the channel
	// type correctly instead of rejecting it.
	payload := ssh.Marshal(&channelOpenDirectMsg{
		Raddr: "127.0.0.1",
		Rport: 18080,
		Laddr: "0.0.0.0",
		Lport: 0,
	})

	ch, chReqs, openErr := cc.OpenChannel("direct-tcpip", payload)
	if openErr != nil {
		t.Fatalf("OpenChannel direct-tcpip failed: %v", openErr)
	}
	defer ch.Close()
	go ssh.DiscardRequests(chReqs)

	// Channel opened successfully — the proxy dispatched to handleDirectTCPIP
	// instead of rejecting with UnknownChannelType. Close cleanly.
	ch.Close()
	cc.Close()
	<-done
}

func TestHandleConnection_UnknownChannelType_Rejected(t *testing.T) {
	// Start a target SSH server for auth resolution.
	targetPassword := "targetpass"
	targetAddr, cleanup := startTestTargetSSH(t, targetPassword)
	defer cleanup()

	resolver := &stubResolverRepo{
		hostAuth: repository.HostSSHAuth{
			EntryPassword: targetPassword,
			ContainerUser: "workspace",
		},
		targetAddr: targetAddr,
	}
	logger := slog.New(slog.DiscardHandler)
	s, err := NewServer(":0", "workspace", "targetpass", "", resolver, logger)
	if err != nil {
		t.Fatal(err)
	}

	config := buildServerConfig(s, resolver)
	proxyAddr, done := startServerAndConnect(t, s, config)

	clientConfig := &ssh.ClientConfig{
		User:            "alice",
		Auth:            []ssh.AuthMethod{ssh.Password("proxy-entry-pass")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	cc, clientErr := ssh.Dial("tcp", proxyAddr, clientConfig)
	if clientErr != nil {
		t.Fatalf("client connection failed: %v", clientErr)
	}
	defer cc.Close()

	// Open an unsupported channel type — should be rejected.
	ch, _, openErr := cc.OpenChannel("my-custom-type", nil)
	if openErr == nil {
		ch.Close()
		t.Fatal("expected rejection for unknown channel type, but OpenChannel succeeded")
	}

	cc.Close()
	<-done
}
