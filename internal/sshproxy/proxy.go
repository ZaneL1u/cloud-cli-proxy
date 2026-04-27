package sshproxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type ContainerResolver interface {
	ResolveContainer(ctx context.Context, username, password string) (ContainerTarget, error)
	ResolveContainerByPublicKey(ctx context.Context, username string, clientKey ssh.PublicKey) (ContainerTarget, error)
}

type Server struct {
	addr     string
	logger   *slog.Logger
	resolver ContainerResolver
	hostKey  ssh.Signer

	containerUser     string
	containerPassword string
}

func NewServer(addr, containerUser, containerPassword, hostKeyPath string, resolver ContainerResolver, logger *slog.Logger) (*Server, error) {
	signer, err := loadOrGenerateHostKey(hostKeyPath, logger)
	if err != nil {
		return nil, fmt.Errorf("host key: %w", err)
	}

	if containerUser == "" {
		containerUser = "workspace"
	}
	if containerPassword == "" {
		containerPassword = "workspace"
		logger.Warn("SSH proxy using default container password — set CONTAINER_SSH_PASSWORD in production")
	}

	return &Server{
		addr:              addr,
		logger:            logger,
		resolver:          resolver,
		hostKey:           signer,
		containerUser:     containerUser,
		containerPassword: containerPassword,
	}, nil
}

func loadOrGenerateHostKey(path string, logger *slog.Logger) (ssh.Signer, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			signer, err := ssh.ParsePrivateKey(data)
			if err != nil {
				return nil, fmt.Errorf("parse host key %s: %w", path, err)
			}
			logger.Info("SSH proxy loaded persistent host key", "path", path)
			exportPublicKey(signer, path, logger)
			return signer, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read host key %s: %w", path, err)
		}
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate host key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			logger.Warn("cannot create host key directory", "path", path, "error", err)
			return signer, nil
		}
		derBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			logger.Warn("cannot marshal host key", "error", err)
			return signer, nil
		}
		pemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: derBytes})
		if err := os.WriteFile(path, pemBlock, 0o600); err != nil {
			logger.Warn("cannot persist host key", "path", path, "error", err)
		} else {
			logger.Info("SSH proxy generated and persisted new host key", "path", path)
		}
		exportPublicKey(signer, path, logger)
	}

	return signer, nil
}

func exportPublicKey(signer ssh.Signer, privKeyPath string, logger *slog.Logger) {
	pubKeyPath := privKeyPath + ".pub"
	pubKeyStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) + " cloud-cli-proxy-proxy\n"
	if err := os.WriteFile(pubKeyPath, []byte(pubKeyStr), 0o644); err != nil {
		logger.Warn("cannot export proxy public key", "path", pubKeyPath, "error", err)
	} else {
		logger.Info("SSH proxy public key exported", "path", pubKeyPath)
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	config := &ssh.ServerConfig{
		MaxAuthTries:  3,
		ServerVersion: "SSH-2.0-CloudCLIProxy",
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			authCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			target, err := s.resolver.ResolveContainer(authCtx, conn.User(), string(password))
			if err != nil {
				s.logger.Debug("SSH password auth failed", "user", conn.User(), "remote", conn.RemoteAddr(), "reason", err)
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
			authCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			target, err := s.resolver.ResolveContainerByPublicKey(authCtx, conn.User(), key)
			if err != nil {
				s.logger.Debug("SSH pubkey auth failed", "user", conn.User(), "remote", conn.RemoteAddr(), "reason", err)
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

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	defer listener.Close()
	s.logger.Info("SSH proxy listening", "addr", s.addr)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.logger.Error("accept failed", "error", err)
			continue
		}
		go s.handleConnection(conn, config)
	}
}

func (s *Server) handleConnection(netConn net.Conn, config *ssh.ServerConfig) {
	defer netConn.Close()

	sshConn, chans, globalReqs, err := ssh.NewServerConn(netConn, config)
	if err != nil {
		s.logger.Debug("SSH handshake failed", "remote", netConn.RemoteAddr(), "error", err)
		return
	}
	defer sshConn.Close()

	ext := sshConn.Permissions.Extensions
	targetAddr := ext["target_addr"]
	targetUser := ext["target_user"]
	targetPassword := ext["target_password"]
	targetPrivateKey := ext["target_private_key"]
	s.logger.Info("SSH proxy session", "user", sshConn.User(), "target", targetAddr, "container_user", targetUser, "remote", netConn.RemoteAddr())

	go ssh.DiscardRequests(globalReqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "only session channels supported")
			continue
		}
		go s.handleChannel(newChan, targetAddr, targetUser, targetPassword, targetPrivateKey)
	}
}

func (s *Server) handleChannel(newChan ssh.NewChannel, targetAddr, targetUser, targetPassword, targetPrivateKey string) {
	clientChan, clientReqs, err := newChan.Accept()
	if err != nil {
		s.logger.Error("accept channel failed", "error", err)
		return
	}
	defer clientChan.Close()

	user := targetUser
	if user == "" {
		user = s.containerUser
	}

	pass := targetPassword
	if pass == "" {
		pass = s.containerPassword
	}

	var authMethods []ssh.AuthMethod
	if targetPrivateKey != "" && (strings.Contains(targetPrivateKey, "BEGIN OPENSSH PRIVATE KEY") || strings.Contains(targetPrivateKey, "BEGIN RSA PRIVATE KEY") || strings.Contains(targetPrivateKey, "BEGIN EC PRIVATE KEY") || strings.Contains(targetPrivateKey, "BEGIN DSA PRIVATE KEY")) {
		signer, err := ssh.ParsePrivateKey([]byte(targetPrivateKey))
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
			s.logger.Debug("SSH proxy using private key auth for container", "addr", targetAddr, "user", user)
		} else {
			s.logger.Warn("SSH proxy failed to parse target private key, falling back to password", "error", err)
		}
	}
	// 密码 fallback：始终追加，确保私钥失败后仍有退路
	authMethods = append(authMethods, ssh.Password(pass), passwordKeyboardInteractive(pass), ssh.PublicKeys(s.hostKey))

	targetConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	targetClient, err := ssh.Dial("tcp", targetAddr, targetConfig)
	if err != nil {
		s.logger.Error("dial container SSH failed", "addr", targetAddr, "user", user, "error", err)
		fmt.Fprintf(clientChan.Stderr(), "Failed to connect to container: %s\r\n", err.Error())
		return
	}
	defer targetClient.Close()

	targetChan, targetReqs, err := targetClient.OpenChannel("session", nil)
	if err != nil {
		s.logger.Error("open target session failed", "addr", targetAddr, "error", err)
		fmt.Fprintf(clientChan.Stderr(), "Failed to open session on container\r\n")
		return
	}
	defer targetChan.Close()

	// Forward requests from client to target (pty-req, shell, exec, env, window-change).
	go func() {
		for req := range clientReqs {
			ok, err := targetChan.SendRequest(req.Type, req.WantReply, req.Payload)
			if err != nil {
				return
			}
			if req.WantReply {
				req.Reply(ok, nil)
			}
		}
	}()

	// Forward requests from target to client (exit-status, exit-signal).
	go func() {
		for req := range targetReqs {
			ok, err := clientChan.SendRequest(req.Type, req.WantReply, req.Payload)
			if err != nil {
				return
			}
			if req.WantReply {
				req.Reply(ok, nil)
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(targetChan, clientChan)
		targetChan.CloseWrite()
	}()

	go func() {
		defer wg.Done()
		io.Copy(clientChan, targetChan)
		clientChan.CloseWrite()
	}()

	go io.Copy(clientChan.Stderr(), targetChan.Stderr())

	wg.Wait()
	s.logger.Debug("SSH proxy channel closed", "target", targetAddr)
}

func passwordKeyboardInteractive(password string) ssh.AuthMethod {
	return ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
		if len(questions) == 0 {
			return nil, nil
		}
		answers := make([]string, len(questions))
		for i := range questions {
			answers[i] = password
		}
		return answers, nil
	})
}
