package broker

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/notify"
	"github.com/francobatta/iol-challenge/notification-system/commons/message"
	"github.com/francobatta/iol-challenge/notification-system/commons/topology"
)

// These tests need a RabbitMQ broker, such as the one "docker compose up -d rabbitmq"
// starts, and are skipped unless AMQP_URL points to it.

// newTestClient returns a Client on the test broker and the ID of an app nobody else
// uses, so that each test works on send queues of its own.
func newTestClient(t *testing.T) (c *Client, appID string) {
	t.Helper()
	url := os.Getenv("AMQP_URL")
	if url == "" {
		t.Skip("AMQP_URL is not set")
	}
	c, err := Dial(url)
	if err != nil {
		t.Fatalf("Setup: Dial failed: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c, "test-" + rand.Text()
}

// deleteQueue removes a test's send queue when the test ends.
func deleteQueue(t *testing.T, c *Client, name string) {
	t.Helper()
	t.Cleanup(func() {
		ch, err := c.conn.Channel()
		if err != nil {
			t.Logf("Cleanup: opening a channel: %v", err)
			return
		}
		defer ch.Close()
		if _, err := ch.QueueDelete(name, false, false, false); err != nil {
			t.Logf("Cleanup: deleting queue %s: %v", name, err)
		}
	})
}

// consume returns the messages of a queue, as a worker would get them.
func consume(t *testing.T, c *Client, queue string) <-chan amqp.Delivery {
	t.Helper()
	ch, err := c.conn.Channel()
	if err != nil {
		t.Fatalf("Setup: opening a channel: %v", err)
	}
	msgs, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		t.Fatalf("Setup: consuming %s: %v", queue, err)
	}
	return msgs
}

func receive(t *testing.T, msgs <-chan amqp.Delivery, within time.Duration) amqp.Delivery {
	t.Helper()
	select {
	case msg, ok := <-msgs:
		if !ok {
			t.Fatal("The consumer was closed while waiting for a message")
		}
		return msg
	case <-time.After(within):
		t.Fatalf("No message arrived within %v", within)
		return amqp.Delivery{}
	}
}

func TestPublishDeliveries(t *testing.T) {
	c, appID := newTestClient(t)
	queue := topology.SendQueue("twilio", appID)
	deleteQueue(t, c, queue)
	want := message.Delivery{MessageID: "j1:e1", JobID: "j1", AppID: appID, Provider: "twilio", Address: "+1", Content: message.Content{Body: "hello"}}

	if err := c.PublishDeliveries(t.Context(), []message.Delivery{want}, notify.PriorityHigh); err != nil {
		t.Fatalf("PublishDeliveries failed: %v", err)
	}
	msgs := consume(t, c, queue)
	msg := receive(t, msgs, 5*time.Second)
	if err := msg.Ack(false); err != nil {
		t.Errorf("Ack failed: %v", err)
	}

	var got message.Delivery
	if err := json.Unmarshal(msg.Body, &got); err != nil {
		t.Fatalf("The message body %q is not a Delivery: %v", msg.Body, err)
	}
	if got != want {
		t.Errorf("Consumed delivery = %+v, want %+v", got, want)
	}
	if msg.MessageId != "j1:e1" || msg.Priority != topology.HighPriority || msg.DeliveryMode != amqp.Persistent || topology.Attempt(msg.Headers) != 0 {
		t.Errorf("Consumed message has ID %q, priority %d, delivery mode %d, attempt %d; want %q, %d, %d, 0",
			msg.MessageId, msg.Priority, msg.DeliveryMode, topology.Attempt(msg.Headers), "j1:e1", topology.HighPriority, amqp.Persistent)
	}
}

func TestPublishDeliveriesToFullQueueIsRejected(t *testing.T) {
	c, appID := newTestClient(t)
	c.maxSendQueueLength = 2
	deleteQueue(t, c, topology.SendQueue("fcm", appID))

	deliveries := make([]message.Delivery, 20)
	for i := range deliveries {
		deliveries[i] = message.Delivery{MessageID: "m", JobID: "j1", AppID: appID, Provider: "fcm", Address: "token"}
	}
	// The limit is enforced loosely: the broker tells publishers that a queue is full
	// a moment after it fills, so a burst sent before that gets through whole. What
	// must hold is that publishing stops being accepted soon after.
	var err error
	for range 10 {
		if err = c.PublishDeliveries(t.Context(), deliveries, notify.PriorityNormal); err != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !errors.Is(err, ErrRejected) {
		t.Errorf("PublishDeliveries(10 batches of 20 to a queue of 2) = %v, want ErrRejected", err)
	}
}

func TestRetryTierReturnsMessageToItsQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the 5 second retry tier")
	}
	c, appID := newTestClient(t)
	queue := topology.SendQueue("apns", appID)
	deleteQueue(t, c, queue)
	if err := c.declareSendQueue(queue); err != nil {
		t.Fatalf("Setup: declaring %s: %v", queue, err)
	}
	msgs := consume(t, c, queue)

	// What a worker does with a delivery that failed: publish it to a retry tier under
	// the name of the queue it came from.
	tier := topology.RetryTiers()[0]
	msg := amqp.Publishing{Headers: amqp.Table{topology.AttemptHeader: int32(1)}, Priority: topology.HighPriority}
	conf, err := c.publish(t.Context(), tier.Name, queue, message.Delivery{MessageID: "j1:e1"}, msg)
	if err != nil {
		t.Fatalf("Publishing to %s failed: %v", tier.Name, err)
	}
	if err := wait(t.Context(), conf); err != nil {
		t.Fatalf("Publishing to %s was not confirmed: %v", tier.Name, err)
	}
	published := time.Now()

	got := receive(t, msgs, tier.Delay+10*time.Second)
	if err := got.Ack(false); err != nil {
		t.Errorf("Ack failed: %v", err)
	}
	if waited := time.Since(published); waited < tier.Delay-time.Second {
		t.Errorf("The message came back after %v, want about %v", waited, tier.Delay)
	}
	if topology.Attempt(got.Headers) != 1 || got.Priority != topology.HighPriority {
		t.Errorf("The returned message has attempt %d and priority %d, want 1 and %d", topology.Attempt(got.Headers), got.Priority, topology.HighPriority)
	}
}
