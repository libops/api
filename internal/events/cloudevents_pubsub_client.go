package events

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol"
)

// PubSubCloudEventsClient wraps PubSubSender to implement cloudevents.Client interface.
type PubSubCloudEventsClient struct {
	sender *PubSubSender
}

// NewPubSubCloudEventsClient creates a CloudEvents client that sends events to Pub/Sub.
func NewPubSubCloudEventsClient(sender *PubSubSender) cloudevents.Client {
	return &PubSubCloudEventsClient{sender: sender}
}

// Send transmits a CloudEvent to Pub/Sub.
func (c *PubSubCloudEventsClient) Send(ctx context.Context, event cloudevents.Event) protocol.Result {
	if err := c.sender.Send(ctx, event); err != nil {
		return newPubSubResult(false, err)
	}
	return newPubSubResult(true, nil)
}

// Request is not supported for Pub/Sub (fire-and-forget only).
func (c *PubSubCloudEventsClient) Request(ctx context.Context, event cloudevents.Event) (*cloudevents.Event, protocol.Result) {
	return nil, newPubSubResult(false, fmt.Errorf("request/response not supported for Pub/Sub"))
}

// StartReceiver is not supported for Pub/Sub sender (send-only client).
func (c *PubSubCloudEventsClient) StartReceiver(ctx context.Context, fn any) error {
	return fmt.Errorf("receiver not supported for Pub/Sub sender client")
}

// pubSubResult implements protocol.Result for Pub/Sub operations.
type pubSubResult struct {
	success bool
	err     error
}

// newPubSubResult creates a new PubSubResult to convey the outcome of a Pub/Sub operation.
func newPubSubResult(success bool, err error) protocol.Result {
	return &pubSubResult{success: success, err: err}
}

// Error returns the error message if the Pub/Sub operation failed.
func (r *pubSubResult) Error() string {
	if r.err != nil {
		return r.err.Error()
	}
	return ""
}

// StatusCode returns an HTTP-like status code indicating the success or failure of the Pub/Sub operation.
func (r *pubSubResult) StatusCode() int {
	if r.success {
		return 200
	}
	if r.err != nil {
		return 500
	}
	return 0
}
