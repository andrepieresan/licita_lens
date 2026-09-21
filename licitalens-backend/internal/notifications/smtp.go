package notifications

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     string
	From     string
	User     string
	Password string
	TLSMode  string
	Timeout  time.Duration
}

type SMTPProvider struct {
	cfg SMTPConfig
}

func NewSMTP(cfg SMTPConfig) *SMTPProvider {
	return &SMTPProvider{cfg: normalizeSMTPConfig(cfg)}
}

func (p *SMTPProvider) Type() ChannelType { return Email }

func (p *SMTPProvider) Send(ctx context.Context, channel Channel, message Message) (string, error) {
	if err := SendSMTPText(ctx, p.cfg, channel.Destination, message.Title, message.Body); err != nil {
		return "", err
	}
	return "smtp:" + message.ID, nil
}

// SendSMTPText sends one text/plain message through a bounded SMTP connection.
// TLSMode accepts auto, starttls, implicit or disabled. In auto mode TLS is used
// whenever the server advertises STARTTLS; authenticated remote connections are
// rejected when encryption is unavailable.
func SendSMTPText(ctx context.Context, cfg SMTPConfig, destination, subject, body string) error {
	cfg = normalizeSMTPConfig(cfg)
	fromAddress, err := parseSMTPAddress(cfg.From, "from")
	if err != nil {
		return err
	}
	toAddress, err := parseSMTPAddress(destination, "destination")
	if err != nil {
		return err
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("smtp subject contains a line break")
	}
	if cfg.Host == "" {
		return fmt.Errorf("smtp host not configured")
	}
	if (cfg.User == "") != (cfg.Password == "") {
		return fmt.Errorf("smtp user and password must be configured together")
	}
	if !validTLSMode(cfg.TLSMode) {
		return fmt.Errorf("invalid smtp TLS mode %q", cfg.TLSMode)
	}

	deadline := time.Now().Add(cfg.Timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	address := net.JoinHostPort(cfg.Host, cfg.Port)
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect smtp: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set smtp deadline: %w", err)
	}
	tlsConfig := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	if cfg.TLSMode == "implicit" {
		tlsConnection := tls.Client(connection, tlsConfig)
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("smtp TLS handshake: %w", err)
		}
		connection = tlsConnection
	}

	client, err := smtp.NewClient(connection, cfg.Host)
	if err != nil {
		return fmt.Errorf("initialize smtp: %w", err)
	}
	defer client.Close()
	secure := cfg.TLSMode == "implicit"
	if cfg.TLSMode == "auto" || cfg.TLSMode == "starttls" {
		if advertised, _ := client.Extension("STARTTLS"); advertised {
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("start smtp TLS: %w", err)
			}
			secure = true
		} else if cfg.TLSMode == "starttls" {
			return fmt.Errorf("smtp server does not advertise STARTTLS")
		}
	}
	if cfg.User != "" {
		if !secure && !isLoopbackHost(cfg.Host) {
			return fmt.Errorf("refusing smtp authentication without TLS")
		}
		if err := client.Auth(smtp.PlainAuth("", cfg.User, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("authenticate smtp: %w", err)
		}
	}
	if err := client.Mail(fromAddress); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := client.Rcpt(toAddress); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	payload := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", cfg.From, destination, strings.TrimSpace(subject), strings.TrimSpace(body)))
	if _, err := w.Write(payload); err != nil {
		_ = w.Close()
		return fmt.Errorf("write smtp message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finish smtp message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("quit smtp: %w", err)
	}
	return nil
}

func normalizeSMTPConfig(cfg SMTPConfig) SMTPConfig {
	if cfg.Port == "" {
		cfg.Port = "1025"
	}
	if cfg.From == "" {
		cfg.From = "alerts@licitalens.local"
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "auto"
	}
	cfg.TLSMode = strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return cfg
}

func validTLSMode(mode string) bool {
	return mode == "auto" || mode == "starttls" || mode == "implicit" || mode == "disabled"
}

func parseSMTPAddress(value, field string) (string, error) {
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("smtp %s contains a line break", field)
	}
	address, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid smtp %s: %w", field, err)
	}
	return address.Address, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

type LogProvider struct {
	channelType ChannelType
}

func NewLogProvider(channelType ChannelType) *LogProvider {
	return &LogProvider{channelType: channelType}
}

func (p *LogProvider) Type() ChannelType { return p.channelType }

func (p *LogProvider) Send(_ context.Context, channel Channel, message Message) (string, error) {
	return fmt.Sprintf("log:%s:%s", p.channelType, message.ID), nil
}
