// Package events provides functionality for sending and receiving CloudEvents via Google Cloud Pub/Sub.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	pubsub "cloud.google.com/go/pubsub/v2"
	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	cloudevents "github.com/cloudevents/sdk-go/v2"
)

// PubSubConfig holds configuration for Google Cloud Pub/Sub.
type PubSubConfig struct {
	ProjectID string
	TopicID   string
}

// PubSubSender implements a simple CloudEvents sender using Pub/Sub directly.
type PubSubSender struct {
	publisher *pubsub.Publisher
	client    *pubsub.Client
}

// NewPubSubSender creates a sender that publishes CloudEvents to a Pub/Sub topic.
func NewPubSubSender(ctx context.Context, projectID, topicID string) (*PubSubSender, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if topicID == "" {
		return nil, fmt.Errorf("topic_id is required")
	}

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	// In v2, use TopicAdminClient to check and create topics
	topicPath := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	_, err = client.TopicAdminClient.GetTopic(ctx, &pb.GetTopicRequest{
		Topic: topicPath,
	})
	if err != nil {
		// Topic doesn't exist, create it
		_, err = client.TopicAdminClient.CreateTopic(ctx, &pb.Topic{
			Name: topicPath,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create topic: %w", err)
		}
	}

	publisher := client.Publisher(topicID)

	return &PubSubSender{
		publisher: publisher,
		client:    client,
	}, nil
}

// Send publishes a CloudEvent to Pub/Sub.
func (s *PubSubSender) Send(ctx context.Context, event cloudevents.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	result := s.publisher.Publish(ctx, &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"ce-specversion": event.SpecVersion(),
			"ce-type":        event.Type(),
			"ce-source":      event.Source(),
			"ce-id":          event.ID(),
		},
	})

	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	return nil
}

// NewNoOpClient creates a CloudEvents client that discards all events
// Useful for testing or when events are disabled.
func NewNoOpClient() cloudevents.Client {
	client, _ := cloudevents.NewClientHTTP()
	return client
}

// EnsureTopic creates a Pub/Sub topic if it doesn't already exist
// This is a helper for setup/initialization.
func EnsureTopic(ctx context.Context, projectID, topicID string) error {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to create pubsub client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Error("unable to close pubsub client", "err", err)
		}
	}()

	// In v2, use TopicAdminClient to check and create topics
	topicPath := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	_, err = client.TopicAdminClient.GetTopic(ctx, &pb.GetTopicRequest{
		Topic: topicPath,
	})
	if err != nil {
		// Topic doesn't exist, create it
		_, err = client.TopicAdminClient.CreateTopic(ctx, &pb.Topic{
			Name: topicPath,
		})
		if err != nil {
			return fmt.Errorf("failed to create topic: %w", err)
		}
	}

	return nil
}
