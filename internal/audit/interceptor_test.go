package audit

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"

	optionsv1 "github.com/libops/api/proto/libops/v1/options"
)

// TestGetSensitiveFieldNames tests the extraction of sensitive field names from protobuf messages.
func TestGetSensitiveFieldNames(t *testing.T) {
	// Note: This test demonstrates the API. In practice, you would test with actual
	// generated proto messages that have the sensitive option set.
	// Since we don't have any real messages with sensitive fields yet,
	// this serves as documentation of how the feature works.
	t.Skip("No proto messages with sensitive fields exist yet")
}

// TestRedactProtoSensitiveFields tests the redaction of sensitive fields within a map representation of a protobuf message.
func TestRedactProtoSensitiveFields(t *testing.T) {
	interceptor := &AuditInterceptor{}

	tests := []struct {
		name            string
		data            map[string]any
		sensitiveFields map[string]bool
		expected        map[string]any
	}{
		{
			name: "redact marked sensitive field",
			data: map[string]any{
				"username": "testuser",
				"password": "secret123",
				"email":    "user@example.com",
			},
			sensitiveFields: map[string]bool{
				"password": true,
			},
			expected: map[string]any{
				"username": "testuser",
				"password": "[REDACTED]",
				"email":    "user@example.com",
			},
		},
		{
			name: "redact multiple sensitive fields",
			data: map[string]any{
				"username": "testuser",
				"password": "secret123",
				"api_key":  "sk_test_12345",
				"email":    "user@example.com",
			},
			sensitiveFields: map[string]bool{
				"password": true,
				"api_key":  true,
			},
			expected: map[string]any{
				"username": "testuser",
				"password": "[REDACTED]",
				"api_key":  "[REDACTED]",
				"email":    "user@example.com",
			},
		},
		{
			name: "redact nested sensitive fields",
			data: map[string]any{
				"user": map[string]any{
					"name":     "John Doe",
					"password": "secret123",
					"email":    "john@example.com",
				},
			},
			sensitiveFields: map[string]bool{
				"password": true,
			},
			expected: map[string]any{
				"user": map[string]any{
					"name":     "John Doe",
					"password": "[REDACTED]",
					"email":    "john@example.com",
				},
			},
		},
		{
			name: "redact in arrays",
			data: map[string]any{
				"users": []any{
					map[string]any{
						"name":     "Alice",
						"password": "secret1",
					},
					map[string]any{
						"name":     "Bob",
						"password": "secret2",
					},
				},
			},
			sensitiveFields: map[string]bool{
				"password": true,
			},
			expected: map[string]any{
				"users": []any{
					map[string]any{
						"name":     "Alice",
						"password": "[REDACTED]",
					},
					map[string]any{
						"name":     "Bob",
						"password": "[REDACTED]",
					},
				},
			},
		},
		{
			name: "no sensitive fields marked",
			data: map[string]any{
				"username": "testuser",
				"email":    "user@example.com",
			},
			sensitiveFields: map[string]bool{},
			expected: map[string]any{
				"username": "testuser",
				"email":    "user@example.com",
			},
		},
		{
			name: "field not in data",
			data: map[string]any{
				"username": "testuser",
			},
			sensitiveFields: map[string]bool{
				"password": true,
			},
			expected: map[string]any{
				"username": "testuser",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interceptor.redactProtoSensitiveFields(tt.data, tt.sensitiveFields)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("redactProtoSensitiveFields() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestSensitiveOptionGeneration verifies that the sensitive protobuf option is correctly generated.
func TestSensitiveOptionGeneration(t *testing.T) {
	if optionsv1.E_Sensitive == nil {
		t.Fatal("E_Sensitive extension not generated")
	}

	ext := optionsv1.E_Sensitive
	if ext.TypeDescriptor().Number() != 50001 {
		t.Errorf("Expected extension number 50001, got %d", ext.TypeDescriptor().Number())
	}

	if ext.TypeDescriptor().FullName() != "libops.v1.options.sensitive" {
		t.Errorf("Expected extension name 'libops.v1.options.sensitive', got %s", ext.TypeDescriptor().FullName())
	}

	extendedType := ext.TypeDescriptor().ContainingMessage()
	expectedType := (&descriptorpb.FieldOptions{}).ProtoReflect().Descriptor()
	if extendedType.FullName() != expectedType.FullName() {
		t.Errorf("Expected extension to extend FieldOptions, but extends %s", extendedType.FullName())
	}
}
