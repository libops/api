package siteproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Proxy handles Pub/Sub push notifications and fans out to site controllers
type Proxy struct {
	apiURL     string
	httpClient *http.Client
}

// NewProxy creates a new site proxy
func NewProxy(apiURL string) *Proxy {
	return &Proxy{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PubSubMessage represents a Pub/Sub push message
type PubSubMessage struct {
	Message struct {
		Data        []byte            `json:"data"`
		Attributes  map[string]string `json:"attributes"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// SiteReconciliationRequest represents a site reconciliation request
type SiteReconciliationRequest struct {
	SitePublicID    string   `json:"site_public_id"`
	ProjectPublicID string   `json:"project_public_id"`
	OrgPublicID     string   `json:"org_public_id"`
	RequestType     string   `json:"request_type"` // "ssh_keys", "secrets", "firewall", "deployment", "full"
	EventIDs        []string `json:"event_ids"`
	Timestamp       string   `json:"timestamp"`
}

// Site represents minimal site information needed for routing
type Site struct {
	ID            int64  `json:"id"`
	PublicID      string `json:"public_id"`
	GCPExternalIP string `json:"gcp_external_ip"`
}

// HandlePubSubPush handles incoming Pub/Sub push notifications
func (p *Proxy) HandlePubSubPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Parse Pub/Sub message
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}

	var pubsubMsg PubSubMessage
	if err := json.Unmarshal(body, &pubsubMsg); err != nil {
		slog.Error("Failed to parse Pub/Sub message", "error", err)
		http.Error(w, "Invalid Pub/Sub message", http.StatusBadRequest)
		return
	}

	// Decode reconciliation request
	var req SiteReconciliationRequest
	if err := json.Unmarshal(pubsubMsg.Message.Data, &req); err != nil {
		slog.Error("Failed to parse reconciliation request", "error", err)
		http.Error(w, "Invalid reconciliation request", http.StatusBadRequest)
		return
	}

	slog.Info("Received reconciliation request",
		"message_id", pubsubMsg.Message.MessageID,
		"site_public_id", req.SitePublicID,
		"request_type", req.RequestType,
		"event_count", len(req.EventIDs))

	// Get site details from API (including external IP)
	site, err := p.getSiteDetails(ctx, req.SitePublicID)
	if err != nil {
		slog.Error("Failed to get site details", "site_public_id", req.SitePublicID, "error", err)
		http.Error(w, fmt.Sprintf("Failed to get site details: %v", err), http.StatusInternalServerError)
		return
	}

	// Validate site has external IP
	if site.GCPExternalIP == "" {
		slog.Warn("Site has no external IP, skipping", "site_public_id", req.SitePublicID)
		// Return 200 to ack the message (site not reachable is not a retry-able error)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Fan out to site controller
	if err := p.callSiteController(ctx, site, req); err != nil {
		slog.Error("Failed to call site controller",
			"site_public_id", req.SitePublicID,
			"external_ip", site.GCPExternalIP,
			"error", err)
		// Return 500 to trigger Pub/Sub retry
		http.Error(w, fmt.Sprintf("Failed to notify site: %v", err), http.StatusInternalServerError)
		return
	}

	slog.Info("Successfully notified site controller",
		"site_public_id", req.SitePublicID,
		"request_type", req.RequestType)

	// Acknowledge the message
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// getSiteDetails fetches site details from the API
func (p *Proxy) getSiteDetails(ctx context.Context, sitePublicID string) (*Site, error) {
	endpoint := fmt.Sprintf("%s/admin/sites/%s", p.apiURL, sitePublicID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Note: In production, this should use service account authentication
	// For now, assuming internal network or API key

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var site Site
	if err := json.NewDecoder(resp.Body).Decode(&site); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &site, nil
}

// callSiteController makes an HTTP POST to the site controller
func (p *Proxy) callSiteController(ctx context.Context, site *Site, req SiteReconciliationRequest) error {
	// Determine endpoint based on request type
	var endpoint string
	switch req.RequestType {
	case "ssh_keys":
		endpoint = "/reconcile/ssh-keys"
	case "secrets":
		endpoint = "/reconcile/secrets"
	case "firewall":
		endpoint = "/reconcile/firewall"
	case "deployment":
		endpoint = "/reconcile/deployment"
	case "full":
		endpoint = "/reconcile/general"
	default:
		endpoint = "/reconcile/general"
	}

	url := fmt.Sprintf("http://%s:8080%s", site.GCPExternalIP, endpoint)

	slog.Info("Calling site controller",
		"site_public_id", site.PublicID,
		"url", url,
		"request_type", req.RequestType)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers with event context
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Event-IDs", fmt.Sprintf("%v", req.EventIDs))
	httpReq.Header.Set("X-Request-Type", req.RequestType)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to call site controller: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("site controller returned status %d: %s", resp.StatusCode, string(body))
	}

	slog.Debug("Site controller responded successfully",
		"site_public_id", site.PublicID,
		"status", resp.StatusCode)

	return nil
}
