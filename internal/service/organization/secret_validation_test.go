package organization

import (
	"testing"
)

// TestValidateSecretName tests the ValidateSecretName function for correct validation of secret names.
func TestValidateSecretName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "valid uppercase with underscores",
			input:     "DATABASE_URL",
			wantError: false,
		},
		{
			name:      "valid single letter",
			input:     "A",
			wantError: false,
		},
		{
			name:      "valid with numbers",
			input:     "API_KEY_123",
			wantError: false,
		},
		{
			name:      "valid long name",
			input:     "MY_SUPER_LONG_SECRET_NAME_WITH_NUMBERS_123",
			wantError: false,
		},
		{
			name:      "invalid lowercase",
			input:     "database_url",
			wantError: true,
		},
		{
			name:      "invalid starts with number",
			input:     "123_API_KEY",
			wantError: true,
		},
		{
			name:      "invalid starts with underscore",
			input:     "_SECRET",
			wantError: true,
		},
		{
			name:      "invalid contains hyphen",
			input:     "API-KEY",
			wantError: true,
		},
		{
			name:      "invalid contains space",
			input:     "API KEY",
			wantError: true,
		},
		{
			name:      "invalid contains lowercase",
			input:     "API_Key",
			wantError: true,
		},
		{
			name:      "invalid empty string",
			input:     "",
			wantError: true,
		},
		{
			name:      "invalid too long",
			input:     "A" + "B" + string(make([]byte, 300)), // > 255 chars
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretName(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateSecretName(%q) error = %v, wantError %v", tt.input, err, tt.wantError)
			}
		})
	}
}
