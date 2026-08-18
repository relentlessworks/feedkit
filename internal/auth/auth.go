package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/relentlessworks/feedkit/internal/model"
	"github.com/relentlessworks/feedkit/internal/store"
)

// Auth handles OTP-based authentication.
type Auth struct {
	store *store.Store
}

// New creates a new Auth handler.
func New(s *store.Store) *Auth {
	return &Auth{store: s}
}

// RequestOTP generates a 6-digit OTP and saves it. If no SMTP is configured,
// the code is logged to stderr.
func (a *Auth) RequestOTP(email string) (string, error) {
	code := generateOTP()
	otp := &model.OTP{
		Email:     email,
		Code:      code,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := a.store.SaveOTP(otp); err != nil {
		return "", err
	}
	// Dev mode: log OTP to stderr
	fmt.Fprintf(os.Stderr, "[feedkit] OTP for %s: %s\n", email, code)
	return code, nil
}

// VerifyOTP validates the OTP and returns a bearer token. It also creates
// a workspace if one doesn't exist for the email.
func (a *Auth) VerifyOTP(email, code string) (*model.Token, *model.Workspace, error) {
	otp, err := a.store.GetOTP(email)
	if err != nil {
		return nil, nil, fmt.Errorf("no OTP found for this email")
	}
	if otp.Code != code {
		return nil, nil, fmt.Errorf("invalid OTP code")
	}
	if time.Now().After(otp.ExpiresAt) {
		a.store.DeleteOTP(email)
		return nil, nil, fmt.Errorf("OTP expired")
	}
	a.store.DeleteOTP(email)

	// Find or create workspace
	ws, err := a.store.GetWorkspaceByEmail(email)
	if err != nil {
		ws = &model.Workspace{
			ID:        model.GenerateID(),
			Handle:    model.GenerateHandle("ws"),
			Email:     email,
			Name:      email,
			Plan:      "free",
			CreatedAt: time.Now(),
		}
		if err := a.store.CreateWorkspace(ws); err != nil {
			return nil, nil, err
		}
	}

	// Create token
	tok := &model.Token{
		ID:          model.GenerateID(),
		Handle:      model.GenerateHandle("tok"),
		WorkspaceID: ws.ID,
		Token:       model.GenerateToken(),
		CreatedAt:   time.Now(),
	}
	if err := a.store.CreateToken(tok); err != nil {
		return nil, nil, err
	}
	return tok, ws, nil
}

// ValidateToken checks a bearer token and returns the associated workspace.
func (a *Auth) ValidateToken(token string) (*model.Workspace, error) {
	tok, err := a.store.GetToken(token)
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	ws, err := a.store.GetWorkspaceByHandle("")
	_ = ws
	// Find workspace by ID
	for _, w := range a.store.ListWorkspaces() {
		if w.ID == tok.WorkspaceID {
			return w, nil
		}
	}
	return nil, fmt.Errorf("workspace not found")
}

func generateOTP() string {
	max := big.NewInt(1000000)
	n, _ := rand.Int(rand.Reader, max)
	return fmt.Sprintf("%06d", n)
}
