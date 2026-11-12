package notification

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// plaintextSMTPServer is an SMTP server that never offers STARTTLS, like a
// misconfigured relay or a connection whose STARTTLS was stripped. It records
// every command it receives.
type plaintextSMTPServer struct {
	addr     string
	mu       sync.Mutex
	commands []string
}

func startPlaintextSMTPServer(t *testing.T) *plaintextSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srv := &plaintextSMTPServer{addr: ln.Addr().String()}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.serve(conn)
		}
	}()
	return srv
}

func (s *plaintextSMTPServer) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 fake ESMTP")
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				write("250 queued")
			}
			continue
		}
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		s.mu.Lock()
		s.commands = append(s.commands, verb)
		s.mu.Unlock()
		switch verb {
		case "EHLO", "HELO":
			write("250-fake")
			write("250 8BITMIME") // no STARTTLS offered
		case "DATA":
			inData = true
			write("354 go ahead")
		case "QUIT":
			write("221 bye")
			return
		default:
			write("250 ok")
		}
	}
}

func (s *plaintextSMTPServer) received(verb string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.commands {
		if c == verb {
			return true
		}
	}
	return false
}

func providerFor(t *testing.T, srv *plaintextSMTPServer, allowInsecure bool) *SMTPProvider {
	t.Helper()
	host, port, err := net.SplitHostPort(srv.addr)
	if err != nil {
		t.Fatal(err)
	}
	return NewSMTPProvider(SMTPConfig{Host: host, Port: port, From: "no-reply@example.com", AllowInsecure: allowInsecure})
}

var linkMessage = Message{
	To: "ada@example.com", Subject: "Reset", EventType: EventPasswordResetRequested,
	Body: "https://ui/reset-password?token=" + secretToken,
}

func TestSMTPRefusesServerWithoutSTARTTLS(t *testing.T) {
	srv := startPlaintextSMTPServer(t)

	err := providerFor(t, srv, false).Send(context.Background(), linkMessage)
	if !errors.Is(err, errNoSTARTTLS) {
		t.Fatalf("Send = %v, want errNoSTARTTLS", err)
	}
	if srv.received("MAIL") || srv.received("DATA") {
		t.Fatal("the message was sent although the server offered no TLS")
	}
}

// A local mail catcher without TLS works only with the explicit development
// opt-in (which configuration refuses outside APP_ENV=development).
func TestSMTPAllowsPlaintextOnlyWithExplicitOptIn(t *testing.T) {
	srv := startPlaintextSMTPServer(t)

	if err := providerFor(t, srv, true).Send(context.Background(), linkMessage); err != nil {
		t.Fatalf("Send with SMTP_ALLOW_INSECURE = %v", err)
	}
	if !srv.received("DATA") {
		t.Fatal("message not delivered to the mail catcher")
	}
}
