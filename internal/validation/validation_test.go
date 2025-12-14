package validation

import (
	"context"
	"testing"
)

func TestEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"valid email", "user@example.com", false},
		{"valid email with subdomain", "user@mail.example.com", false},
		{"valid email with plus", "user+tag@example.com", false},
		{"empty email", "", true},
		{"no @ symbol", "userexample.com", true},
		{"no domain", "user@", true},
		{"no local part", "@example.com", true},
		{"no TLD", "user@example", true},
		{"multiple @", "user@@example.com", true},
		{"too long", string(make([]byte, 260)) + "@example.com", true},
		{"local part too long", string(make([]byte, 70)) + "@example.com", true},
		{"spaces", "user name@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Email(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("Email() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{"valid IPv4 CIDR", "192.168.1.0/24", false},
		{"valid IPv4 CIDR /32", "10.0.0.1/32", false},
		{"valid IPv6 CIDR", "2001:db8::/32", false},
		{"empty CIDR", "", true},
		{"invalid format", "192.168.1.0", true},
		{"invalid IP", "999.999.999.999/24", true},
		{"invalid mask", "192.168.1.0/33", true},
		{"no mask", "192.168.1.0/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("CIDR() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIPAddress(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv4 loopback", "127.0.0.1", false},
		{"valid IPv6", "2001:db8::1", false},
		{"valid IPv6 loopback", "::1", false},
		{"empty IP", "", true},
		{"invalid format", "999.999.999.999", true},
		{"partial IP", "192.168.1", true},
		{"with CIDR", "192.168.1.0/24", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IPAddress(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("IPAddress() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUUID(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{"valid UUID v4", "550e8400-e29b-41d4-a716-446655440000", false},
		{"valid UUID v1", "6ba7b810-9dad-11d1-80b4-00c04fd430c8", false},
		{"empty UUID", "", true},
		{"invalid format", "not-a-uuid", true},
		{"missing hyphens", "550e8400e29b41d4a716446655440000", false}, // google.UUID accepts this format
		{"too short", "550e8400-e29b-41d4", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UUID(tt.uuid)
			if (err != nil) != tt.wantErr {
				t.Errorf("UUID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStringLength(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		value     string
		minLength int
		maxLength int
		wantErr   bool
	}{
		{"valid length", "name", "John Doe", 1, 100, false},
		{"at min length", "name", "J", 1, 100, false},
		{"at max length", "name", string(make([]byte, 100)), 1, 100, false},
		{"too short", "name", "", 1, 100, true},
		{"too long", "name", string(make([]byte, 101)), 1, 100, true},
		{"no min constraint", "name", "", 0, 100, false},
		{"no max constraint", "name", string(make([]byte, 1000)), 1, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := StringLength(tt.field, tt.value, tt.minLength, tt.maxLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringLength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGCPProjectID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid project ID", "my-project-123", false},
		{"valid with numbers", "project123", false},
		{"valid min length", "abc123", false},
		{"valid max length", "abcdefghij12345678901234567890", false},
		{"empty", "", true},
		{"too short", "abc12", true},
		{"too long", "a" + string(make([]byte, 30)), true},
		{"starts with number", "123project", true},
		{"contains uppercase", "My-Project", true},
		{"ends with hyphen", "my-project-", true},
		{"starts with hyphen", "-my-project", true},
		{"contains underscore", "my_project", true},
		{"contains space", "my project", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := GCPProjectID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("GCPProjectID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGitHubRepo(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		wantErr bool
	}{
		{"valid repo", "owner/repo", false},
		{"valid with hyphens", "my-org/my-repo", false},
		{"valid with numbers", "user123/repo456", false},
		{"valid with dots", "owner/repo.name", false},
		{"empty", "", true},
		{"no slash", "ownerrepo", true},
		{"too many slashes", "owner/org/repo", true},
		{"empty owner", "/repo", true},
		{"empty repo", "owner/", true},
		{"owner starts with hyphen", "-owner/repo", true},
		{"owner ends with hyphen", "owner-/repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := GitHubRepo(tt.repo)
			if (err != nil) != tt.wantErr {
				t.Errorf("GitHubRepo() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPasswordComplexity(t *testing.T) {
	tests := []struct {
		name    string
		pwd     string
		wantErr bool
	}{
		{"valid password", "MyP@ssw0rd123!", false},
		{"valid complex", "Tr0ub4dor&3Extra!", false},
		{"empty", "", true},
		{"too short", "Short1!", true},
		{"no uppercase", "myp@ssw0rd123!", true},
		{"no lowercase", "MYP@SSW0RD123!", true},
		{"no digit", "MyPassword!", true},
		{"no special", "MyPassword123", true},
		{"too long", "MyP@ssw0rd" + string(make([]byte, 70)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PasswordComplexity(tt.pwd)
			if (err != nil) != tt.wantErr {
				t.Errorf("PasswordComplexity() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPort(t *testing.T) {
	tests := []struct {
		name    string
		port    int32
		wantErr bool
	}{
		{"valid port 80", 80, false},
		{"valid port 443", 443, false},
		{"valid port 8080", 8080, false},
		{"valid port 1", 1, false},
		{"valid port 65535", 65535, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"too large", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Port(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("Port() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSSHPublicKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			"valid RSA key",
			"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDZmDhO9z0YHQP3MCsP5I1234567890abcdefghijklmnopqrstuvwxyz user@host",
			false,
		},
		{
			"valid ED25519 key",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl user@host",
			false,
		},
		{
			"valid ECDSA key",
			"ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAAABBBHRi26I7Q7FJ1LxKkF9x user@host",
			false,
		},
		{"empty", "", true},
		{"invalid prefix", "invalid AAAAB3NzaC1yc2E", true},
		{"too short", "ssh-rsa ABC", true},
		{"only key type", "ssh-rsa", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SSHPublicKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("SSHPublicKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGitHubRepoIsPublic(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		wantErr bool
	}{
		{
			name:    "valid public repository",
			repo:    "libops/api",
			wantErr: false,
		},
		{
			name:    "invalid format",
			repo:    "invalid-repo",
			wantErr: true,
		},
		{
			name:    "empty string",
			repo:    "",
			wantErr: true,
		},
		{
			name:    "non-existent repository",
			repo:    "thisdoesnotexist123456789/repo999999999",
			wantErr: true,
		},
		{
			name:    "private repository example",
			repo:    "libops/build",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := GitHubRepoIsPublic(ctx, tt.repo)
			if (err != nil) != tt.wantErr {
				t.Errorf("GitHubRepoIsPublic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
