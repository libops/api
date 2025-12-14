package auth

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// requestMessageKey is a private unexported type for context keys used in this package.
type requestMessageKey struct{}

// SetRequestMessage stores the protobuf message in context.
func SetRequestMessage(ctx context.Context, msg proto.Message) context.Context {
	return context.WithValue(ctx, requestMessageKey{}, msg)
}

// GetRequestMessage retrieves the protobuf message from context.
func GetRequestMessage(ctx context.Context) (proto.Message, bool) {
	msg, ok := ctx.Value(requestMessageKey{}).(proto.Message)
	return msg, ok
}

// GetRequestMessageAsJSON converts the stored protobuf message to JSON bytes.
func GetRequestMessageAsJSON(ctx context.Context) ([]byte, bool) {
	msg, ok := GetRequestMessage(ctx)
	if !ok {
		return nil, false
	}

	// Marshal protobuf to JSON
	jsonBytes, err := protojson.Marshal(msg)
	if err != nil {
		return nil, false
	}

	return jsonBytes, true
}

// StoreRequestMessage stores the request message in context for later access.
func StoreRequestMessage(ctx context.Context, req connect.AnyRequest) context.Context {
	// The message is already deserialized by ConnectRPC
	msg, ok := req.Any().(proto.Message)
	if !ok {
		return ctx
	}

	return SetRequestMessage(ctx, msg)
}
