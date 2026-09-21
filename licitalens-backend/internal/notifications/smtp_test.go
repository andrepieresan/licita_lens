package notifications

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSendSMTPTextUsesBoundedConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan string, 1)
	go serveTestSMTP(listener, done)
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	err = SendSMTPText(context.Background(), SMTPConfig{Host: host, Port: port, From: "alerts@example.test", TLSMode: "disabled", Timeout: time.Second}, "user@example.test", "Subject", "Body")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-done:
		if !strings.Contains(message, "Subject: Subject") || !strings.Contains(message, "Body") {
			t.Fatalf("unexpected message: %q", message)
		}
	case <-time.After(time.Second):
		t.Fatal("smtp server did not receive the message")
	}
}

func TestSendSMTPTextRejectsHeaderInjection(t *testing.T) {
	err := SendSMTPText(context.Background(), SMTPConfig{Host: "localhost", From: "alerts@example.test"}, "user@example.test", "safe\r\nBcc: attacker@example.test", "body")
	if err == nil || !strings.Contains(err.Error(), "line break") {
		t.Fatalf("expected header validation error, got %v", err)
	}
}

func serveTestSMTP(listener net.Listener, done chan<- string) {
	connection, err := listener.Accept()
	if err != nil {
		return
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	_, _ = fmt.Fprint(connection, "220 test smtp\r\n")
	var message strings.Builder
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if inData {
			if trimmed == "." {
				inData = false
				done <- message.String()
				_, _ = fmt.Fprint(connection, "250 queued\r\n")
				continue
			}
			message.WriteString(line)
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "EHLO"):
			_, _ = fmt.Fprint(connection, "250-test\r\n250 OK\r\n")
		case strings.HasPrefix(trimmed, "MAIL FROM"), strings.HasPrefix(trimmed, "RCPT TO"):
			_, _ = fmt.Fprint(connection, "250 OK\r\n")
		case trimmed == "DATA":
			inData = true
			_, _ = fmt.Fprint(connection, "354 continue\r\n")
		case trimmed == "QUIT":
			_, _ = fmt.Fprint(connection, "221 bye\r\n")
			return
		default:
			_, _ = fmt.Fprint(connection, "250 OK\r\n")
		}
	}
}
