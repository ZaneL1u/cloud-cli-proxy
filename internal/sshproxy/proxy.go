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
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ContainerResolver maps an SSH login (host_short_id + entry_password)
// to the target container's SSH address and credentials.
type ContainerResolver interface {
	ResolveContainer(ctx context.Context, hostShortID, password string) (ContainerTarget, error)
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
	}

	return signer, nil
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
				s.logger.Debug("SSH auth failed", "user", conn.User(), "remote", conn.RemoteAddr(), "reason", err)
				return nil, fmt.Errorf("auth failed")
			}
			return &ssh.Permissions{
				Extensions: map[string]string{
					"target_addr":     target.Addr,
					"target_user":     target.User,
					"target_password": target.Password,
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
	s.logger.Info("SSH proxy session", "user", sshConn.User(), "target", targetAddr, "container_user", targetUser, "remote", netConn.RemoteAddr())

	go ssh.DiscardRequests(globalReqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "only session channels supported")
			continue
		}
		go s.handleChannel(newChan, targetAddr, targetUser, targetPassword)
	}
}

func (s *Server) handleChannel(newChan ssh.NewChannel, targetAddr, targetUser, targetPassword string) {
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

	targetConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	targetClient, err := ssh.Dial("tcp", targetAddr, targetConfig)
	if err != nil {
		s.logger.Error("dial container SSH failed", "addr", targetAddr, "error", err)
		fmt.Fprintf(clientChan.Stderr(), "Failed to connect to container: connection refused\r\n")
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
