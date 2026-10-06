package consume

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DefaultDiscoveryInterval is how often a Pool looks for queues of apps that have
// started sending since it last looked.
const DefaultDiscoveryInterval = 5 * time.Second

// A Pool consumes every send queue of one provider.
//
// Each queue gets a subscription of its own with the same prefetch, which is what keeps
// apps from starving each other: however deep an app's backlog, the broker hands this
// worker at most prefetch of its deliveries at a time, so every app with work waiting
// has deliveries in hand, and they compete for the handler's send slots on equal terms.
type Pool struct {
	conn     *amqp.Connection
	queues   func(ctx context.Context) ([]string, error)
	handler  *Handler
	metrics  *Metrics
	prefetch int
	interval time.Duration

	mu         sync.Mutex
	subscribed map[string]*amqp.Channel // by queue name
	wg         sync.WaitGroup           // subscriptions and the deliveries they are handling
}

// NewPool returns a Pool that consumes the queues named by queues, which it calls
// every interval, taking at most prefetch deliveries from each at a time and passing
// them to handler.
func NewPool(conn *amqp.Connection, queues func(context.Context) ([]string, error), handler *Handler, metrics *Metrics, prefetch int, interval time.Duration) *Pool {
	return &Pool{
		conn:       conn,
		queues:     queues,
		handler:    handler,
		metrics:    metrics,
		prefetch:   prefetch,
		interval:   interval,
		subscribed: make(map[string]*amqp.Channel),
	}
}

// Run consumes until ctx is cancelled. It then stops taking deliveries, waits for
// those being sent to finish, and returns nil; the ones it held but had not started go
// back to their queues when the connection is closed.
//
// It returns an error if the connection to the broker is lost.
func (p *Pool) Run(ctx context.Context) error {
	closed := p.conn.NotifyClose(make(chan *amqp.Error, 1))
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		// A failed discovery is not fatal: the queues already subscribed keep being
		// consumed, and the next tick tries again.
		if err := p.discover(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "Could not look for new send queues", "err", err)
		}
		select {
		case <-ctx.Done():
			p.stop()
			return nil
		case err := <-closed:
			p.wg.Wait()
			return fmt.Errorf("the connection to RabbitMQ was lost: %v", err)
		case <-ticker.C:
		}
	}
}

func (p *Pool) discover(ctx context.Context) error {
	names, err := p.queues(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, name := range names {
		p.mu.Lock()
		_, ok := p.subscribed[name]
		p.mu.Unlock()
		if ok {
			continue
		}
		if err := p.subscribe(ctx, name); err != nil {
			errs = append(errs, err)
			continue
		}
		slog.InfoContext(ctx, "Consuming a send queue", "queue", name)
	}
	return errors.Join(errs...)
}

func (p *Pool) subscribe(ctx context.Context, queue string) error {
	// A channel per queue, because prefetch is set per channel.
	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("opening a channel for %s: %v", queue, err)
	}
	if err := ch.Qos(p.prefetch, 0, false); err != nil {
		ch.Close()
		return fmt.Errorf("setting prefetch on %s: %v", queue, err)
	}
	// The queue name doubles as the consumer tag, which stop cancels by.
	msgs, err := ch.Consume(queue, queue, false, false, false, false, nil)
	if err != nil {
		// The channel is already closed by the broker, for example because the queue
		// was deleted between being listed and now.
		return fmt.Errorf("consuming %s: %v", queue, err)
	}
	p.mu.Lock()
	p.subscribed[queue] = ch
	p.metrics.queues.Set(float64(len(p.subscribed)))
	p.mu.Unlock()

	p.wg.Go(func() {
		// The loop ends when the subscription is cancelled, by stop, or when the
		// channel is closed, by a lost connection or a deleted queue.
		for d := range msgs {
			p.wg.Go(func() {
				// At most prefetch of these run per queue: the broker sends no more
				// until one is settled.
				// If settling fails the channel is gone, and the broker delivers the
				// message again.
				if err := settle(d, p.handler.Handle(ctx, messageFrom(queue, d))); err != nil {
					slog.ErrorContext(ctx, "Could not settle a delivery; it will be handled again", "queue", queue, "err", err)
				}
			})
		}
		// Forget the queue, so that if it still exists the next discovery subscribes
		// to it again.
		p.mu.Lock()
		delete(p.subscribed, queue)
		p.metrics.queues.Set(float64(len(p.subscribed)))
		p.mu.Unlock()
	})
	return nil
}

// settle tells the broker what the handler decided about d. The difference between
// rejecting and nacking with requeue matters: only a reject counts as a failed
// delivery, which is what the broker delays and limits.
func settle(d amqp.Delivery, v Verdict) error {
	switch v {
	case Sent:
		return d.Ack(false)
	case Retry:
		return d.Reject(true)
	case Dead:
		return d.Reject(false)
	default:
		return d.Nack(false, true)
	}
}

func (p *Pool) stop() {
	p.mu.Lock()
	for queue, ch := range p.subscribed {
		// Cancelling stops new deliveries and closes the subscription's Go channel,
		// which ends its loop. The AMQP channel stays open for the acknowledgements
		// of deliveries still being handled.
		if err := ch.Cancel(queue, false); err != nil {
			slog.Warn("Could not cancel a subscription", "queue", queue, "err", err)
		}
	}
	p.mu.Unlock()
	p.wg.Wait()
}
