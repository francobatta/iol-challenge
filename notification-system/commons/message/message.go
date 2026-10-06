// Package message defines the messages the services exchange over RabbitMQ.
package message

type Content struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Delivery struct {
	MessageID string  `json:"message_id"`
	JobID     string  `json:"job_id"`
	AppID     string  `json:"app_id"`
	Provider  string  `json:"provider"`
	Address   string  `json:"address"`
	Content   Content `json:"content"`
}
