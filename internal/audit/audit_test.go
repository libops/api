package audit

import (
	"context"
	"net/http"
	"testing"
)

// TestAuditConstants verifies audit event and entity type constants.
func TestAuditConstants(t *testing.T) {
	events := []Event{
		UserLoginSuccess,
		UserLoginFailure,
		APIKeyCreate,
		APIKeyDelete,
		OrganizationCreate,
		OrganizationUpdate,
		OrganizationDelete,
		ProjectCreate,
		ProjectUpdate,
		ProjectDelete,
		AccountCreate,
		AccountUpdate,
		AccountDelete,
		SiteCreate,
		SiteUpdate,
		SiteDelete,
		DeploymentSuccess,
		DeploymentFailure,
		SSHKeyCreate,
		SSHKeyDelete,
		AuthorizationFailure,
		OrganizationSecretCreateSuccess,
		ProjectSecretCreateSuccess,
		SiteSecretCreateSuccess,
	}

	for _, event := range events {
		if event == "" {
			t.Error("Event constant is empty")
		}
	}

	entityTypes := []EntityType{
		AccountEntityType,
		OrganizationEntityType,
		ProjectEntityType,
		SiteEntityType,
		SSHKeyEntityType,
		APIKeyEntityType,
	}

	for _, entityType := range entityTypes {
		if entityType == "" {
			t.Error("EntityType constant is empty")
		}
	}
}

// TestExtractSourceIP tests the extraction of the client's source IP from the request context.
func TestExtractSourceIP(t *testing.T) {
	tests := []struct {
		name     string
		setupCtx func() context.Context
		wantIP   string
	}{
		{
			name: "X-Forwarded-For with single IP",
			setupCtx: func() context.Context {
				req := &http.Request{
					Header: http.Header{
						"X-Forwarded-For": []string{"192.168.1.100"},
					},
				}
				return context.WithValue(context.Background(), "http_request", req) //nolint:staticcheck // Test uses string key to match production middleware
			},
			wantIP: "192.168.1.100",
		},
		{
			name: "X-Forwarded-For with multiple IPs",
			setupCtx: func() context.Context {
				req := &http.Request{
					Header: http.Header{
						"X-Forwarded-For": []string{"192.168.1.100, 10.0.0.1, 172.16.0.1"},
					},
				}
				return context.WithValue(context.Background(), "http_request", req) //nolint:staticcheck // Test uses string key to match production middleware
			},
			wantIP: "192.168.1.100",
		},
		{
			name: "X-Real-IP when X-Forwarded-For absent",
			setupCtx: func() context.Context {
				req, _ := http.NewRequest("GET", "/", nil)
				req.Header.Set("X-Real-IP", "10.0.0.50")
				return context.WithValue(context.Background(), "http_request", req) //nolint:staticcheck // Test uses string key to match production middleware
			},
			wantIP: "10.0.0.50",
		},
		{
			name: "RemoteAddr as fallback",
			setupCtx: func() context.Context {
				req := &http.Request{
					Header:     http.Header{},
					RemoteAddr: "172.16.5.10:54321",
				}
				return context.WithValue(context.Background(), "http_request", req) //nolint:staticcheck // Test uses string key to match production middleware
			},
			wantIP: "172.16.5.10",
		},
		{
			name: "IPv6 address in RemoteAddr",
			setupCtx: func() context.Context {
				req := &http.Request{
					Header:     http.Header{},
					RemoteAddr: "[2001:db8::1]:8080",
				}
				return context.WithValue(context.Background(), "http_request", req) //nolint:staticcheck // Test uses string key to match production middleware
			},
			wantIP: "[2001:db8::1]",
		},
		{
			name: "No request in context",
			setupCtx: func() context.Context {
				return context.Background()
			},
			wantIP: "unknown",
		},
		{
			name: "X-Forwarded-For takes precedence over X-Real-IP",
			setupCtx: func() context.Context {
				req := &http.Request{
					Header: http.Header{
						"X-Forwarded-For": []string{"192.168.1.100"},
						"X-Real-IP":       []string{"10.0.0.50"},
					},
					RemoteAddr: "172.16.5.10:54321",
				}
				return context.WithValue(context.Background(), "http_request", req) //nolint:staticcheck // Test uses string key to match production middleware
			},
			wantIP: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			gotIP := ExtractSourceIP(ctx)
			if gotIP != tt.wantIP {
				t.Errorf("ExtractSourceIP() = %v, want %v", gotIP, tt.wantIP)
			}
		})
	}
}

// TestExtractUserAgent tests the extraction of the User-Agent header from the request context.
func TestExtractUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		setupCtx  func() context.Context
		wantAgent string
	}{
		{
			name: "User-Agent present",
			setupCtx: func() context.Context {
				req := &http.Request{
					Header: http.Header{
						"User-Agent": []string{"Mozilla/5.0 (X11; Linux x86_64) Chrome/91.0"},
					},
				}
				return context.WithValue(context.Background(), "http_request", req) //nolint:staticcheck // Test uses string key to match production middleware
			},
			wantAgent: "Mozilla/5.0 (X11; Linux x86_64) Chrome/91.0",
		},
		{
			name: "User-Agent empty",
			setupCtx: func() context.Context {
				req := &http.Request{
					Header: http.Header{},
				}
				return context.WithValue(context.Background(), "http_request", req) //nolint:staticcheck // Test uses string key to match production middleware
			},
			wantAgent: "",
		},
		{
			name: "No request in context",
			setupCtx: func() context.Context {
				return context.Background()
			},
			wantAgent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			gotAgent := ExtractUserAgent(ctx)
			if gotAgent != tt.wantAgent {
				t.Errorf("ExtractUserAgent() = %v, want %v", gotAgent, tt.wantAgent)
			}
		})
	}
}
