package notifications

import (
	"context"
	"errors"
	"time"
)

type ChannelType string

const (
	ExpoPush ChannelType = "expo_push"
	Email    ChannelType = "email"
	WhatsApp ChannelType = "whatsapp"
)

type Channel struct {
	ID, OrganizationID, Destination string
	Type                            ChannelType
	Enabled                         bool
	ConsentedAt, OptedOutAt         time.Time
}

func (c Channel) CanSend() bool {
	return c.Enabled && c.Destination != "" && c.OptedOutAt.IsZero() && (c.Type != WhatsApp || !c.ConsentedAt.IsZero())
}

type Message struct{ ID, OrganizationID, OpportunityID, Title, Body, SourceURL string }
type Provider interface {
	Type() ChannelType
	Send(context.Context, Channel, Message) (providerMessageID string, err error)
}

type Dispatcher struct{ providers map[ChannelType]Provider }

func New(providers ...Provider) *Dispatcher {
	result := &Dispatcher{providers: map[ChannelType]Provider{}}
	for _, provider := range providers {
		result.providers[provider.Type()] = provider
	}
	return result
}
func (d *Dispatcher) Send(ctx context.Context, channel Channel, message Message) (string, error) {
	if !channel.CanSend() {
		return "", errors.New("notification channel suppressed")
	}
	provider, ok := d.providers[channel.Type]
	if !ok {
		return "", errors.New("notification provider unavailable")
	}
	return provider.Send(ctx, channel, message)
}
