// Package publisher handles publishing reconciliation requests to Pub/Sub
package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"cloud.google.com/go/pubsub"
)

const (
	// SiteReconciliationTopic is the Pub/Sub topic for site reconciliation requests
	SiteReconciliationTopic = "libops-control-plane"
)

// PubSubPublisher publishes messages to Google Cloud Pub/Sub
type PubSubPublisher struct {
	client *pubsub.Client
	topics map[string]*pubsub.Topic
}

// NewPubSubPublisher creates a new Pub/Sub publisher
func NewPubSubPublisher(ctx context.Context, projectID string) (*PubSubPublisher, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	publisher := &PubSubPublisher{
		client: client,
		topics: make(map[string]*pubsub.Topic),
	}

	// Initialize topics
	if err := publisher.ensureTopic(ctx, SiteReconciliationTopic); err != nil {
		return nil, fmt.Errorf("failed to initialize topic %s: %w", SiteReconciliationTopic, err)
	}

	return publisher, nil
}

// ensureTopic ensures a topic exists
func (p *PubSubPublisher) ensureTopic(ctx context.Context, topicID string) error {
	topic := p.client.Topic(topicID)
	exists, err := topic.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if topic exists: %w", err)
	}

	if !exists {
		slog.Info("Creating Pub/Sub topic", "topic", topicID)
		topic, err = p.client.CreateTopic(ctx, topicID)
		if err != nil {
			return fmt.Errorf("failed to create topic: %w", err)
		}
	}

	p.topics[topicID] = topic
	return nil
}

// PublishSiteReconciliation publishes a site reconciliation request
func (p *PubSubPublisher) PublishSiteReconciliation(ctx context.Context, req SiteReconciliationRequest) error {
	topic, ok := p.topics[SiteReconciliationTopic]
	if !ok {
		return fmt.Errorf("topic %s not initialized", SiteReconciliationTopic)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal reconciliation request: %w", err)
	}

	result := topic.Publish(ctx, &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"site_public_id":    req.SitePublicID,
			"org_public_id":     req.OrgPublicID,
			"project_public_id": req.ProjectPublicID,
			"request_type":      req.RequestType,
		},
	})

	// Block and get the result
	messageID, err := result.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	slog.Info("Published site reconciliation request",
		"site_public_id", req.SitePublicID,
		"request_type", req.RequestType,
		"event_count", len(req.EventIDs),
		"message_id", messageID)

	return nil
}

// Close closes the Pub/Sub client
func (p *PubSubPublisher) Close() error {
	// Stop all topics
	for _, topic := range p.topics {
		topic.Stop()
	}

	return p.client.Close()
}

// SiteReconciliationRequest represents a request to reconcile a site
type SiteReconciliationRequest struct {
	SitePublicID    string   `json:"site_public_id"`
	ProjectPublicID string   `json:"project_public_id"`
	OrgPublicID     string   `json:"org_public_id"`
	RequestType     string   `json:"request_type"` // "ssh_keys", "secrets", "firewall", "full"
	EventIDs        []string `json:"event_ids"`    // Original event IDs that triggered this
	Timestamp       string   `json:"timestamp"`
}
