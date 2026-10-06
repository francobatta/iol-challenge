// Package message defines the messages the services exchange over RabbitMQ.
//
// The files in testdata pin their JSON form: a change that alters it fails the tests
// of this package.
package message

type Content struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// A Delivery is one notification for one endpoint, as fan-out publishes it and
// the workers receive it.
type Delivery struct {
	// MessageID is the same however many times the delivery is published, so a
	// provider can use it to drop duplicates.
	MessageID string  `json:"message_id"`
	JobID     string  `json:"job_id"`
	AppID     string  `json:"app_id"`
	Provider  string  `json:"provider"`
	Address   string  `json:"address"`
	Content   Content `json:"content"`
}
