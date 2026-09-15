package notifications

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

type SMTPConfig struct {
	Host string
	Port string
	From string
}

type SMTPProvider struct {
	cfg SMTPConfig
}

func NewSMTP(cfg SMTPConfig) *SMTPProvider {
	if cfg.Port == "" {
		cfg.Port = "1025"
	}
	if cfg.From == "" {
		cfg.From = "alerts@licitalens.local"
	}
	return &SMTPProvider{cfg: cfg}
}

func (p *SMTPProvider) Type() ChannelType { return Email }

func (p *SMTPProvider) Send(_ context.Context, channel Channel, message Message) (string, error) {
	if p.cfg.Host == "" {
		return "", fmt.Errorf("smtp host not configured")
	}
	addr := p.cfg.Host + ":" + p.cfg.Port
	body := strings.TrimSpace(message.Body)
	subject := strings.TrimSpace(message.Title)
	payload := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", p.cfg.From, channel.Destination, subject, body))
	if err := smtp.SendMail(addr, nil, p.cfg.From, []string{channel.Destination}, payload); err != nil {
		return "", err
	}
	return "smtp:" + message.ID, nil
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
