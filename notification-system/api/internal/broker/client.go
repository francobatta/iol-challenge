// Package broker is the API module's connection to RabbitMQ, which it only publishes
// deliveries to. The queues themselves are defined in the commons
// module's topology package.
//
// Every publish is persistent and confirmed, so a message the broker has accepted
// survives a broker restart.
package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/commons/message"
	"github.com/francobatta/iol-challenge/notification-system/commons/topology"
)

// ErrRejected reports that the broker refused a publish, which it does when the queue
// the message is for is full.
var ErrRejected = errors.New("publish rejected by the broker")

// A Client publishes to RabbitMQ, reconnecting by itself after the connection is lost.
// It is safe for concurrent use.
type Client struct {
	url                string
	maxSendQueueLength int

	mu       sync.Mutex // guards the fields below and serializes use of pub
	conn     *amqp.Connection
	pub      *amqp.Channel   // in confirm mode
	declared map[string]bool // send queues known to exist
}

// Dial connects to the broker at url and declares the queues and exchanges the system
// uses, apart from the per-app send queues, which are declared on first use.
func Dial(url string) (*Client, error) {
	c := &Client{url: url, maxSendQueueLength: topology.MaxSendQueueLength, declared: make(map[string]bool)}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.channel(); err != nil {
		return nil, err
	}
	return c, nil
}

// Close closes the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.conn.IsClosed() {
		return nil
	}
	return c.conn.Close()
}

// channel returns the publishing channel, connecting first if there is no usable one.
// The caller must hold c.mu.
func (c *Client) channel() (*amqp.Channel, error) {
	if c.pub != nil && !c.pub.IsClosed() {
		return c.pub, nil
	}
	if c.conn == nil || c.conn.IsClosed() {
		conn, err := amqp.Dial(c.url)
		if err != nil {
			return nil, fmt.Errorf("connecting to RabbitMQ: %v", err)
		}
		c.conn = conn
	}
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("opening a RabbitMQ channel: %v", err)
	}
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("enabling publisher confirms: %v", err)
	}
	if err := topology.Declare(ch); err != nil {
		return nil, err
	}
	c.pub = ch
	return ch, nil
}

// publish sends one persistent message and returns the handle on which the broker's
// confirmation arrives. msg carries the properties to send besides the body.
func (c *Client) publish(ctx context.Context, exchange, key string, body any, msg amqp.Publishing) (*amqp.DeferredConfirmation, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding a message for %s: %v", key, err)
	}
	msg.Body = data
	msg.ContentType = "application/json"
	msg.DeliveryMode = amqp.Persistent
	if msg.Headers == nil {
		msg.Headers = amqp.Table{}
	}
	topology.InjectTrace(ctx, msg.Headers)

	c.mu.Lock()
	defer c.mu.Unlock()
	ch, err := c.channel()
	if err != nil {
		return nil, err
	}
	conf, err := ch.PublishWithDeferredConfirmWithContext(ctx, exchange, key, false, false, msg)
	if err != nil {
		return nil, fmt.Errorf("publishing to %s: %v", key, err)
	}
	return conf, nil
}

func wait(ctx context.Context, conf *amqp.DeferredConfirmation) error {
	acked, err := conf.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("waiting for the broker to confirm a publish: %v", err)
	}
	if !acked {
		return ErrRejected
	}
	return nil
}

// PublishDeliveries queues deliveries for the workers of their providers. It returns
// ErrRejected if the broker refused any of them because its queue is full; the others
// have been queued, and since the caller cannot tell which, it must publish them all
// again later.
func (c *Client) PublishDeliveries(ctx context.Context, deliveries []message.Delivery, p notify.Priority) error {
	msg := amqp.Publishing{}
	if p == notify.PriorityHigh {
		msg.Priority = topology.HighPriority
	}
	// Publish everything before waiting, so the broker confirms the batch as a whole
	// instead of one round trip per message.
	confs := make([]*amqp.DeferredConfirmation, 0, len(deliveries))
	for _, d := range deliveries {
		queue := topology.SendQueue(d.Provider, d.AppID)
		if err := c.declareSendQueue(queue); err != nil {
			return err
		}
		m := msg
		m.MessageId = d.MessageID
		m.Headers = amqp.Table{topology.AttemptHeader: int32(0)}
		conf, err := c.publish(ctx, "", queue, d, m)
		if err != nil {
			return err
		}
		confs = append(confs, conf)
	}
	var rejected bool
	for _, conf := range confs {
		switch err := wait(ctx, conf); {
		case errors.Is(err, ErrRejected):
			rejected = true
		case err != nil:
			return err
		}
	}
	if rejected {
		return ErrRejected
	}
	return nil
}

func (c *Client) declareSendQueue(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.declared[name] {
		return nil
	}
	ch, err := c.channel()
	if err != nil {
		return err
	}
	if err := topology.DeclareSendQueue(ch, name, c.maxSendQueueLength); err != nil {
		return err
	}
	c.declared[name] = true
	return nil
}
