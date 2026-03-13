package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// EventType identifies the kind of real-time event.
type EventType string

const (
	EventTaskCreated    EventType = "task.created"
	EventTaskUpdated    EventType = "task.updated"
	EventTaskTransition EventType = "task.transition"
	EventTaskDeleted    EventType = "task.deleted"
	EventAgentOutput    EventType = "agent.output"
	EventAgentStatus    EventType = "agent.status"
	EventSessionStarted EventType = "session.started"
	EventSessionEnded   EventType = "session.ended"
	EventTokenUsage     EventType = "token.usage"
)

// Event is a pub/sub message envelope.
type Event struct {
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// PubSub provides Redis-backed pub/sub for board and agent streams.
type PubSub struct {
	client *redis.Client
}

// NewPubSub creates a new PubSub backed by the given Redis client.
func NewPubSub(client *redis.Client) *PubSub {
	return &PubSub{client: client}
}

// boardChannel returns the Redis channel name for a project's board stream.
func boardChannel(projectID string) string {
	return "steerlane:board:" + projectID
}

// agentChannel returns the Redis channel name for a session's agent stream.
func agentChannel(sessionID string) string {
	return "steerlane:agent:" + sessionID
}

// PublishBoardEvent publishes a board event for the given project.
func (p *PubSub) PublishBoardEvent(ctx context.Context, projectID string, eventType EventType, payload any) error {
	return p.publish(ctx, boardChannel(projectID), eventType, payload)
}

// PublishAgentEvent publishes an agent event for the given session.
func (p *PubSub) PublishAgentEvent(ctx context.Context, sessionID string, eventType EventType, payload any) error {
	return p.publish(ctx, agentChannel(sessionID), eventType, payload)
}

func (p *PubSub) publish(ctx context.Context, channel string, eventType EventType, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	evt := Event{Type: eventType, Payload: data}
	msg, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return p.client.Publish(ctx, channel, msg).Err()
}

// Subscription wraps a Redis pub/sub subscription with typed event delivery.
type Subscription struct {
	pubsub *redis.PubSub
}

// SubscribeBoard subscribes to board events for a project.
func (p *PubSub) SubscribeBoard(ctx context.Context, projectID string) *Subscription {
	sub := p.client.Subscribe(ctx, boardChannel(projectID))
	return &Subscription{pubsub: sub}
}

// SubscribeAgent subscribes to agent events for a session.
func (p *PubSub) SubscribeAgent(ctx context.Context, sessionID string) *Subscription {
	sub := p.client.Subscribe(ctx, agentChannel(sessionID))
	return &Subscription{pubsub: sub}
}

// Channel returns a Go channel that receives parsed events.
func (s *Subscription) Channel() <-chan Event {
	ch := make(chan Event, 64)
	go func() {
		defer close(ch)
		for msg := range s.pubsub.Channel() {
			var evt Event
			if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
				continue // skip malformed messages
			}
			ch <- evt
		}
	}()
	return ch
}

// Close unsubscribes and closes the subscription.
func (s *Subscription) Close() error {
	return s.pubsub.Close()
}
