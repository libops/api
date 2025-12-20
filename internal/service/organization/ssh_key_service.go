package organization

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/db"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// SshKeyService implements the LibOps SshKeyService API.
type SshKeyService struct {
	db db.Querier
}

// Compile-time check to ensure SshKeyService implements the interface.
var _ libopsv1connect.SshKeyServiceHandler = (*SshKeyService)(nil)

// NewSshKeyService creates a new SshKeyService instance.
func NewSshKeyService(querier db.Querier) *SshKeyService {
	return &SshKeyService{
		db: querier,
	}
}

// ListSshKeys lists all Ssh keys for an account.
func (s *SshKeyService) ListSshKeys(
	ctx context.Context,
	req *connect.Request[libopsv1.ListSshKeysRequest],
) (*connect.Response[libopsv1.ListSshKeysResponse], error) {
	accountID := req.Msg.AccountId

	if accountID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("account_id is required"))
	}

	_, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	keys, err := s.db.ListSshKeysByAccount(ctx, accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	protoKeys := make([]*libopsv1.SshKey, len(keys))
	for i, key := range keys {
		protoKeys[i] = &libopsv1.SshKey{
			KeyId:       key.PublicID,
			AccountId:   key.AccountPublicID,
			PublicKey:   key.PublicKey,
			Name:        toStringPtr(key.Name),
			Fingerprint: toStringPtr(key.Fingerprint),
		}
	}

	return connect.NewResponse(&libopsv1.ListSshKeysResponse{
		SshKeys: protoKeys,
	}), nil
}

// CreateSshKey creates a new Ssh key for an account.
func (s *SshKeyService) CreateSshKey(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateSshKeyRequest],
) (*connect.Response[libopsv1.CreateSshKeyResponse], error) {
	accountID := req.Msg.AccountId
	publicKey := req.Msg.PublicKey
	name := req.Msg.Name

	if accountID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("account_id is required"))
	}
	if publicKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("public_key is required"))
	}

	accountPublicID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	fingerprint, err := computeSshKeyFingerprint(publicKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid Ssh public key: %w", err))
	}

	keyPublicID := uuid.New()

	params := db.CreateSshKeyParams{
		PublicID:        keyPublicID.String(),
		AccountPublicID: accountPublicID.String(),
		PublicKey:       publicKey,
		Name:            fromStringPtr(name),
		Fingerprint:     sql.NullString{String: fingerprint, Valid: true},
	}

	_, err = s.db.CreateSshKey(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create Ssh key: %w", err))
	}

	createdKey, err := s.db.GetSshKey(ctx, keyPublicID.String())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to retrieve created Ssh key: %w", err))
	}

	protoKey := &libopsv1.SshKey{
		KeyId:       createdKey.PublicID,
		AccountId:   createdKey.AccountPublicID,
		PublicKey:   createdKey.PublicKey,
		Name:        toStringPtr(createdKey.Name),
		Fingerprint: toStringPtr(createdKey.Fingerprint),
	}

	return connect.NewResponse(&libopsv1.CreateSshKeyResponse{
		SshKey: protoKey,
	}), nil
}

// DeleteSshKey deletes an Ssh key.
func (s *SshKeyService) DeleteSshKey(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteSshKeyRequest],
) (*connect.Response[emptypb.Empty], error) {
	keyID := req.Msg.KeyId
	accountID := req.Msg.AccountId

	if keyID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("key_id is required"))
	}
	if accountID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("account_id is required"))
	}

	_, err := uuid.Parse(keyID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid key_id format: %w", err))
	}

	_, err = uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	existingKey, err := s.db.GetSshKey(ctx, keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("ssh key not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	if existingKey.AccountPublicID != accountID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("ssh key does not belong to account"))
	}

	err = s.db.DeleteSshKey(ctx, keyID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete Ssh key: %w", err))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// computeSshKeyFingerprint validates an Ssh public key and computes its fingerprint.
func computeSshKeyFingerprint(publicKey string) (string, error) {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	if err != nil {
		return "", fmt.Errorf("invalid Ssh public key format: %w", err)
	}

	hash := sha256.Sum256(pubKey.Marshal())
	fingerprint := "SHA256:" + base64.RawStdEncoding.EncodeToString(hash[:])

	return fingerprint, nil
}

// toStringPtr converts a sql.NullString to an optional pointer to a string, returning nil if not valid.
func toStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// fromStringPtr converts a *string to a sql.NullString, setting Valid to true if the pointer is not nil.
func fromStringPtr(s *string) sql.NullString {
	if s != nil {
		return sql.NullString{String: *s, Valid: true}
	}
	return sql.NullString{Valid: false}
}
