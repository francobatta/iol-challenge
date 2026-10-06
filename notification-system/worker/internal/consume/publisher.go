package consume

import (
	"context"
	"errors"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/francobatta/iol-challenge/notification-system/commons/topology"
)

// An AMQPPublisher is the Publisher that talks to RabbitMQ. It is safe for concurrent
// use.
type AMQPPublisher struct {
	mu sync.Mutex // serializes use of ch
	ch *amqp.Channel
}

// NewAMQPPublisher returns a Publisher on a channel of its own on conn. It declares
// the queues and exchanges it publishes to.
func NewAMQPPublisher(conn *amqp.Connection) (*AMQPPublisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("opening a RabbitMQ channel: %v", err)
	}
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("enabling publisher confirms: %v", err)
	}
	if err := topology.Declare(ch); err != nil {
		return nil, err
	}
	return &AMQPPublisher{ch: ch}, nil
}

func (p *AMQPPublisher) publish(ctx context.Context, exchange, key string, msg amqp.Publishing) error {
	msg.DeliveryMode = amqp.Persistent
	msg.ContentType = "application/json"

	// Only the publish itself is serialized. Waiting for the confirmation is not, so
	// that concurrent publishes share round trips to the broker.
	p.mu.Lock()
	conf, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, key, false, false, msg)
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("publishing to %s: %v", key, err)
	}
	acked, err := conf.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("waiting for the broker to confirm a publish to %s: %v", key, err)
	}
	if !acked {
		return fmt.Errorf("the broker rejected a publish to %s", key)
	}
	return nil
}

func (p *AMQPPublisher) PublishRetry(ctx context.Context, tier int, msg Message) error {
	tiers := topology.RetryTiers()
	if tier < 0 || tier >= len(tiers) {
		return errors.New("no such retry tier")
	}
	out := republish(ctx, msg)
	out.Headers[topology.AttemptHeader] = int32(msg.Attempt + 1)
	// The routing key is where the tier sends the message when its delay is over.
	return p.publish(ctx, tiers[tier].Name, msg.Queue, out)
}

func (p *AMQPPublisher) PublishDead(ctx context.Context, msg Message, reason string) error {
	out := republish(ctx, msg)
	out.Headers[topology.AttemptHeader] = int32(msg.Attempt)
	out.Headers["x-reason"] = reason
	out.Headers["x-origin-queue"] = msg.Queue
	return p.publish(ctx, "", topology.DeadQueue, out)
}

// republish returns msg as a message to publish, under the trace of ctx.
func republish(ctx context.Context, msg Message) amqp.Publishing {
	headers := amqp.Table{}
	topology.InjectTrace(ctx, headers)
	return amqp.Publishing{Body: msg.Body, Priority: msg.Priority, Headers: headers}
}

func messageFrom(queue string, d amqp.Delivery) Message {
	msg := Message{
		Queue:    queue,
		Body:     d.Body,
		Attempt:  topology.Attempt(d.Headers),
		Priority: d.Priority,
		Headers:  make(map[string]string),
	}
	for k, v := range d.Headers {
		if s, ok := v.(string); ok {
			msg.Headers[k] = s
		}
	}
	return msg
}

func extractContext(ctx context.Context, headers map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(headers))
}
