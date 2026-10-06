// Package topology is the RabbitMQ layout the notification system runs on: the queues
// and exchanges, how they are declared, and the headers its messages carry.
//
// Every service declares the layout through this package, so they always agree on it.
// RabbitMQ refuses a declaration that differs from an existing queue, which means a
// change to the arguments here needs the queues to be deleted or migrated.
//
// Every queue is a durable quorum queue, so a message the broker has accepted survives
// a broker restart.
package topology

import (
	"context"
	"fmt"
	"maps"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
)

const (
	// DeadQueue holds deliveries that will not be retried, for inspection.
	DeadQueue = "notify.dead"

	// MaxSendQueueLength bounds the backlog of one app on one provider. Past it the
	// broker rejects publishes, which is what makes fan-out pause.
	MaxSendQueueLength = 100_000
	// HighPriority is the AMQP priority of deliveries of high-priority jobs. Normal
	// deliveries carry none.
	HighPriority = 9
	// AttemptHeader counts how many times a message has been retried. Read it with
	// Attempt.
	AttemptHeader = "x-attempt"

	// deliveryLimit is how many times a delivery may be handed to a worker that then
	// dies or returns it, before the broker moves it to DeadQueue.
	deliveryLimit = 20
)

// SendQueue returns the name of the queue holding an app's deliveries for a provider.
// Messages are a JSON message.Delivery.
func SendQueue(provider, appID string) string {
	return SendQueuePrefix(provider) + appID
}

// SendQueuePrefix returns what the names of a provider's send queues start with.
func SendQueuePrefix(provider string) string {
	return "notify.send." + provider + "."
}

// A RetryTier delays messages. A message published to the exchange Name with a queue
// name as routing key reappears in that queue after Delay.
type RetryTier struct {
	Name  string // of both the exchange and the queue behind it
	Delay time.Duration
}

// RetryTiers returns the retry tiers, shortest delay first.
func RetryTiers() []RetryTier {
	return []RetryTier{
		{Name: "notify.retry.5s", Delay: 5 * time.Second},
		{Name: "notify.retry.30s", Delay: 30 * time.Second},
		{Name: "notify.retry.2m", Delay: 2 * time.Minute},
		{Name: "notify.retry.10m", Delay: 10 * time.Minute},
	}
}

func quorumArgs(extra amqp.Table) amqp.Table {
	args := amqp.Table{"x-queue-type": "quorum"}
	maps.Copy(args, extra)
	return args
}

// Declare declares every queue and exchange but the per-app send queues, which are
// declared with DeclareSendQueue when an app first sends.
func Declare(ch *amqp.Channel) error {
	if _, err := ch.QueueDeclare(DeadQueue, true, false, false, false, quorumArgs(nil)); err != nil {
		return fmt.Errorf("declaring queue %s: %v", DeadQueue, err)
	}
	for _, tier := range RetryTiers() {
		if err := ch.ExchangeDeclare(tier.Name, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declaring exchange %s: %v", tier.Name, err)
		}
		// An expired message is dead-lettered to the default exchange under the routing
		// key it arrived with, which is the name of the queue it has to go back to.
		// At-least-once dead-lettering keeps it here while that queue is full; the
		// broker only offers it together with reject-publish.
		args := quorumArgs(amqp.Table{
			"x-message-ttl":          tier.Delay.Milliseconds(),
			"x-dead-letter-exchange": "",
			"x-dead-letter-strategy": "at-least-once",
			"x-overflow":             "reject-publish",
		})
		if _, err := ch.QueueDeclare(tier.Name, true, false, false, false, args); err != nil {
			return fmt.Errorf("declaring queue %s: %v", tier.Name, err)
		}
		if err := ch.QueueBind(tier.Name, "#", tier.Name, false, nil); err != nil {
			return fmt.Errorf("binding queue %s: %v", tier.Name, err)
		}
	}
	return nil
}

// DeclareSendQueue declares a send queue that rejects publishes once it holds
// maxLength messages, normally MaxSendQueueLength.
func DeclareSendQueue(ch *amqp.Channel, name string, maxLength int) error {
	args := quorumArgs(amqp.Table{
		"x-max-length":     int64(maxLength),
		"x-overflow":       "reject-publish",
		"x-delivery-limit": int64(deliveryLimit),
		// The default exchange routes by queue name, so this dead-letters to DeadQueue.
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": DeadQueue,
		"x-dead-letter-strategy":    "at-least-once",
	})
	if _, err := ch.QueueDeclare(name, true, false, false, false, args); err != nil {
		return fmt.Errorf("declaring queue %s: %v", name, err)
	}
	return nil
}

// Attempt returns how many times the message with these headers has been retried.
func Attempt(headers amqp.Table) int {
	// The client decodes an integer header as the narrowest type that holds it.
	switch n := headers[AttemptHeader].(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	}
	return 0
}

// InjectTrace writes the trace of ctx into the headers of a message to publish.
func InjectTrace(ctx context.Context, headers amqp.Table) {
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(headers))
}

// ExtractTrace returns ctx continuing the trace carried in a received message's headers.
func ExtractTrace(ctx context.Context, headers amqp.Table) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, headerCarrier(headers))
}

type headerCarrier amqp.Table

func (c headerCarrier) Get(key string) string {
	s, _ := c[key].(string)
	return s
}

func (c headerCarrier) Set(key, value string) { c[key] = value }

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
