package service

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestPointerHelpers tests the pointer conversion helper functions.
func TestPointerHelpers(t *testing.T) {
	t.Run("stringPtr", func(t *testing.T) {
		s := "test"
		ptr := stringPtr(s)
		assert.NotNil(t, ptr)
		assert.Equal(t, "test", *ptr)
	})

	t.Run("int32Ptr", func(t *testing.T) {
		i := int32(42)
		ptr := int32Ptr(i)
		assert.NotNil(t, ptr)
		assert.Equal(t, int32(42), *ptr)
	})

	t.Run("int64Ptr", func(t *testing.T) {
		i := int64(123456)
		ptr := int64Ptr(i)
		assert.NotNil(t, ptr)
		assert.Equal(t, int64(123456), *ptr)
	})

	t.Run("float64Ptr", func(t *testing.T) {
		f := 3.14
		ptr := float64Ptr(f)
		assert.NotNil(t, ptr)
		assert.Equal(t, 3.14, *ptr)
	})
}

// TestValueHelpers tests the pointer dereference with default value helpers.
func TestValueHelpers(t *testing.T) {
	t.Run("stringValue with nil returns default", func(t *testing.T) {
		result := stringValue(nil, "default")
		assert.Equal(t, "default", result)
	})

	t.Run("stringValue with value returns value", func(t *testing.T) {
		value := "custom"
		result := stringValue(&value, "default")
		assert.Equal(t, "custom", result)
	})

	t.Run("int32Value with nil returns default", func(t *testing.T) {
		result := int32Value(nil, 10)
		assert.Equal(t, int32(10), result)
	})

	t.Run("int32Value with value returns value", func(t *testing.T) {
		value := int32(42)
		result := int32Value(&value, 10)
		assert.Equal(t, int32(42), result)
	})

	t.Run("int64Value with nil returns default", func(t *testing.T) {
		result := int64Value(nil, 100)
		assert.Equal(t, int64(100), result)
	})

	t.Run("int64Value with value returns value", func(t *testing.T) {
		value := int64(999)
		result := int64Value(&value, 100)
		assert.Equal(t, int64(999), result)
	})

	t.Run("boolValue with nil returns default", func(t *testing.T) {
		result := boolValue(nil, true)
		assert.True(t, result)
	})

	t.Run("boolValue with value returns value", func(t *testing.T) {
		value := false
		result := boolValue(&value, true)
		assert.False(t, result)
	})
}

// TestTimeConversions tests time conversion between database and proto formats.
func TestTimeConversions(t *testing.T) {
	t.Run("timeToProto with valid time", func(t *testing.T) {
		now := time.Now().UTC()
		nullTime := sql.NullTime{Time: now, Valid: true}

		proto := timeToProto(nullTime)

		assert.NotNil(t, proto)
		assert.Equal(t, now.Unix(), proto.AsTime().Unix())
	})

	t.Run("timeToProto with null time returns nil", func(t *testing.T) {
		nullTime := sql.NullTime{Valid: false}

		proto := timeToProto(nullTime)

		assert.Nil(t, proto)
	})

	t.Run("protoToTime with valid timestamp", func(t *testing.T) {
		now := time.Now().UTC()
		proto := timestamppb.New(now)

		nullTime := protoToTime(proto)

		assert.True(t, nullTime.Valid)
		assert.Equal(t, now.Unix(), nullTime.Time.Unix())
	})

	t.Run("protoToTime with nil returns invalid", func(t *testing.T) {
		nullTime := protoToTime(nil)

		assert.False(t, nullTime.Valid)
	})
}

// TestPageTokenParsing tests pagination token parsing and generation.
func TestPageTokenParsing(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantValue int
		wantErr   bool
	}{
		{
			name:      "parses valid token",
			token:     "100",
			wantValue: 100,
			wantErr:   false,
		},
		{
			name:      "empty token returns 0",
			token:     "",
			wantValue: 0,
			wantErr:   false,
		},
		{
			name:      "invalid number returns error",
			token:     "not-a-number",
			wantValue: 0,
			wantErr:   true,
		},
		{
			name:      "parses zero",
			token:     "0",
			wantValue: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePageToken(tt.token)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantValue, got)
		})
	}
}

// TestGeneratePageToken tests page token generation.
func TestGeneratePageToken(t *testing.T) {
	tests := []struct {
		name   string
		offset int
	}{
		{
			name:   "generates token for offset 0",
			offset: 0,
		},
		{
			name:   "generates token for offset 50",
			offset: 50,
		},
		{
			name:   "generates token for offset 1000",
			offset: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := GeneratePageToken(tt.offset)

			assert.NotEmpty(t, token)

			parsed, err := ParsePageToken(token)
			assert.NoError(t, err)
			assert.Equal(t, tt.offset, parsed)
		})
	}
}

// TestIsValidMemberRole tests member role validation.
func TestIsValidMemberRole(t *testing.T) {
	tests := []struct {
		name string
		role string
		want bool
	}{
		{
			name: "owner is valid",
			role: "owner",
			want: true,
		},
		{
			name: "developer is valid",
			role: "developer",
			want: true,
		},
		{
			name: "read is valid",
			role: "read",
			want: true,
		},
		{
			name: "invalid role returns false",
			role: "admin",
			want: false,
		},
		{
			name: "empty role returns false",
			role: "",
			want: false,
		},
		{
			name: "case sensitive - Owner is invalid",
			role: "Owner",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidMemberRole(tt.role)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNullStringConversions tests SQL null string conversions.
func TestNullStringConversions(t *testing.T) {
	t.Run("toNullString with non-empty string", func(t *testing.T) {
		result := toNullString("test")

		assert.True(t, result.Valid)
		assert.Equal(t, "test", result.String)
	})

	t.Run("toNullString with empty string", func(t *testing.T) {
		result := toNullString("")

		assert.False(t, result.Valid)
	})

	t.Run("FromNullString with valid string", func(t *testing.T) {
		ns := sql.NullString{String: "hello", Valid: true}

		result := FromNullString(ns)

		assert.Equal(t, "hello", result)
	})

	t.Run("FromNullString with invalid returns empty", func(t *testing.T) {
		ns := sql.NullString{Valid: false}

		result := FromNullString(ns)

		assert.Equal(t, "", result)
	})

	t.Run("FromNullStringPtr with valid string", func(t *testing.T) {
		ns := sql.NullString{String: "world", Valid: true}

		result := FromNullStringPtr(ns)

		assert.NotNil(t, result)
		assert.Equal(t, "world", *result)
	})

	t.Run("FromNullStringPtr with invalid returns nil", func(t *testing.T) {
		ns := sql.NullString{Valid: false}

		result := FromNullStringPtr(ns)

		assert.Nil(t, result)
	})
}

// TestNullInt64Conversions tests SQL null int64 conversions.
func TestNullInt64Conversions(t *testing.T) {
	t.Run("toNullInt64 with non-zero value", func(t *testing.T) {
		result := toNullInt64(42)

		assert.True(t, result.Valid)
		assert.Equal(t, int64(42), result.Int64)
	})

	t.Run("toNullInt64 with zero value", func(t *testing.T) {
		result := toNullInt64(0)

		assert.False(t, result.Valid)
	})

	t.Run("fromNullInt64 with valid value", func(t *testing.T) {
		ni := sql.NullInt64{Int64: 999, Valid: true}

		result := fromNullInt64(ni)

		assert.Equal(t, int64(999), result)
	})

	t.Run("fromNullInt64 with invalid returns zero", func(t *testing.T) {
		ni := sql.NullInt64{Valid: false}

		result := fromNullInt64(ni)

		assert.Equal(t, int64(0), result)
	})
}

// TestNullBoolConversion tests SQL null bool conversion.
func TestNullBoolConversion(t *testing.T) {
	t.Run("toNullBool with true", func(t *testing.T) {
		result := toNullBool(true)

		assert.True(t, result.Valid)
		assert.True(t, result.Bool)
	})

	t.Run("toNullBool with false", func(t *testing.T) {
		result := toNullBool(false)

		assert.True(t, result.Valid)
		assert.False(t, result.Bool)
	})
}

// TestPtrToString tests pointer to string conversion.
func TestPtrToString(t *testing.T) {
	t.Run("ptrToString with value", func(t *testing.T) {
		s := "hello"
		result := ptrToString(&s)

		assert.Equal(t, "hello", result)
	})

	t.Run("ptrToString with nil", func(t *testing.T) {
		result := ptrToString(nil)

		assert.Equal(t, "", result)
	})
}

// TestParseUUID tests UUID parsing with field name context.
func TestParseUUID(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name      string
		id        string
		fieldName string
		wantErr   bool
	}{
		{
			name:      "parses valid UUID",
			id:        validUUID,
			fieldName: "project_id",
			wantErr:   false,
		},
		{
			name:      "returns error for invalid UUID",
			id:        "not-a-uuid",
			fieldName: "site_id",
			wantErr:   true,
		},
		{
			name:      "returns error for empty string",
			id:        "",
			fieldName: "org_id",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseUUID(tt.id, tt.fieldName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.fieldName)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, validUUID, result.String())
		})
	}
}

// TestParsePagination tests pagination parameter parsing.
func TestParsePagination(t *testing.T) {
	tests := []struct {
		name       string
		pageSize   int32
		pageToken  string
		wantLimit  int32
		wantOffset int32
		wantErr    bool
	}{
		{
			name:       "uses default page size when 0",
			pageSize:   0,
			pageToken:  "",
			wantLimit:  50,
			wantOffset: 0,
			wantErr:    false,
		},
		{
			name:       "uses provided page size",
			pageSize:   10,
			pageToken:  "",
			wantLimit:  10,
			wantOffset: 0,
			wantErr:    false,
		},
		{
			name:       "caps page size at maximum",
			pageSize:   500,
			pageToken:  "",
			wantLimit:  100,
			wantOffset: 0,
			wantErr:    false,
		},
		{
			name:       "parses page token for offset",
			pageSize:   20,
			pageToken:  GeneratePageToken(40),
			wantLimit:  20,
			wantOffset: 40,
			wantErr:    false,
		},
		{
			name:       "returns error for invalid page token",
			pageSize:   10,
			pageToken:  "invalid!!!",
			wantLimit:  0,
			wantOffset: 0,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePagination(tt.pageSize, tt.pageToken)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantLimit, result.Limit)
			assert.Equal(t, tt.wantOffset, result.Offset)
		})
	}
}

// TestMakePaginationResult tests pagination result generation.
func TestMakePaginationResult(t *testing.T) {
	tests := []struct {
		name          string
		resultCount   int
		params        PaginationParams
		wantNextToken bool
	}{
		{
			name:          "no next page when results less than limit",
			resultCount:   5,
			params:        PaginationParams{Limit: 10, Offset: 0},
			wantNextToken: false,
		},
		{
			name:          "has next page when results equal limit",
			resultCount:   10,
			params:        PaginationParams{Limit: 10, Offset: 0},
			wantNextToken: true,
		},
		{
			name:          "has next page with correct offset",
			resultCount:   20,
			params:        PaginationParams{Limit: 20, Offset: 20},
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakePaginationResult(tt.resultCount, tt.params)

			if tt.wantNextToken {
				assert.NotEmpty(t, result.NextPageToken)
				offset, err := ParsePageToken(result.NextPageToken)
				assert.NoError(t, err)
				assert.Equal(t, int32(tt.params.Offset+tt.params.Limit), int32(offset))
			} else {
				assert.Empty(t, result.NextPageToken)
			}
		})
	}
}
