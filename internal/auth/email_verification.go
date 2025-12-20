package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/libops/api/db"
)

// EmailVerificationToken represents a pending email verification.
type EmailVerificationToken struct {
	Email        string
	Token        string
	PasswordHash string // Temporarily stored until verification
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// EmailVerifier handles email verification for new accounts.
type EmailVerifier struct {
	db          db.Querier
	emailSender EmailSender
	apiBaseURL  string
}

// EmailSender interface for sending emails.
type EmailSender interface {
	SendEmail(to, subject, body string) error
}

// NewEmailVerifier creates a new email verification handler.
func NewEmailVerifier(querier db.Querier, sender EmailSender, apiBaseURL string) *EmailVerifier {
	return &EmailVerifier{
		db:          querier,
		emailSender: sender,
		apiBaseURL:  apiBaseURL,
	}
}

// CreateVerificationToken creates a new verification token for an email.
func (v *EmailVerifier) CreateVerificationToken(ctx context.Context, email string, passwordHash string) (*EmailVerificationToken, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	expiresAt := time.Now().Add(24 * time.Hour) // Token valid for 24 hours

	err := v.db.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
		Email:        email,
		Token:        token,
		PasswordHash: passwordHash,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store verification token: %w", err)
	}

	verification := &EmailVerificationToken{
		Email:        email,
		Token:        token,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
		ExpiresAt:    expiresAt,
	}

	return verification, nil
}

// VerifyToken verifies an email verification token.
func (v *EmailVerifier) VerifyToken(ctx context.Context, email, token string) error {
	_, err := v.db.GetEmailVerificationToken(ctx, db.GetEmailVerificationTokenParams{
		Email: email,
		Token: token,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("invalid or expired verification token")
		}
		return fmt.Errorf("failed to verify token: %w", err)
	}

	return nil
}

// GetToken retrieves a verification token.
func (v *EmailVerifier) GetToken(ctx context.Context, email, token string) (*EmailVerificationToken, error) {
	row, err := v.db.GetEmailVerificationToken(ctx, db.GetEmailVerificationTokenParams{
		Email: email,
		Token: token,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no pending verification for email: %s", email)
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	return &EmailVerificationToken{
		Email:        row.Email,
		Token:        row.Token,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt.Time,
		ExpiresAt:    row.ExpiresAt,
	}, nil
}

// DeleteToken removes a verification token.
func (v *EmailVerifier) DeleteToken(ctx context.Context, email string) error {
	err := v.db.DeleteEmailVerificationToken(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	return nil
}

// SendVerificationEmail sends a verification email to the user.
func (v *EmailVerifier) SendVerificationEmail(email, token string) error {
	verificationURL := fmt.Sprintf("%s/auth/verify?email=%s&token=%s", v.apiBaseURL, email, token)

	subject := "Verify your libops account"
	body := fmt.Sprintf(`
Hello,

Thank you for signing up for libops!

Please verify your email address by clicking the link below:

%s

This link will expire in 24 hours.

If you did not create an account, please ignore this email.

Best regards,
The libops Team
`, verificationURL)

	if v.emailSender == nil {
		// For development/testing - just log the verification URL
		fmt.Printf("=== EMAIL VERIFICATION ===\n")
		fmt.Printf("To: %s\n", email)
		fmt.Printf("Subject: %s\n", subject)
		fmt.Printf("Verification URL: %s\n", verificationURL)
		fmt.Printf("========================\n")
		return nil
	}

	return v.emailSender.SendEmail(email, subject, body)
}

// CleanupExpiredTokens removes expired verification tokens
// This should be called periodically (e.g., via a cron job).
func (v *EmailVerifier) CleanupExpiredTokens(ctx context.Context) error {
	err := v.db.CleanupExpiredVerificationTokens(ctx)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}
	return nil
}
