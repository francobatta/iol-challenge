package consume

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/francobatta/iol-challenge/notification-system/commons/topology"
)

func messageFrom(queue string, d amqp.Delivery) Message {
	msg := Message{
		Queue:   queue,
		Body:    d.Body,
		Attempt: topology.DeliveryCount(d.Headers),
		Headers: make(map[string]string),
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
