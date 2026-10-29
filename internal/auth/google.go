package auth

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/api/idtoken"
)

type GoogleUser struct {
	Sub     string
	Email   string
	Name    string
	Picture string
}

type Verifier interface {
	Verify(ctx context.Context, idToken string) (*GoogleUser, error)
}

var ErrInvalidIDToken = errors.New("invalid google id token")

type googleVerifier struct{ audiences []string }

func NewGoogleVerifier(audiences []string) Verifier {
	return &googleVerifier{audiences: audiences}
}

func (g *googleVerifier) Verify(ctx context.Context, token string) (*GoogleUser, error) {
	for _, aud := range g.audiences {
		p, err := idtoken.Validate(ctx, token, aud)
		if err != nil {
			continue
		}
		email := str(p.Claims["email"])
		verified, _ := p.Claims["email_verified"].(bool)
		if email == "" || !verified || p.Subject == "" {
			return nil, ErrInvalidIDToken
		}
		return &GoogleUser{
			Sub:     p.Subject,
			Email:   strings.ToLower(email),
			Name:    str(p.Claims["name"]),
			Picture: str(p.Claims["picture"]),
		}, nil
	}
	return nil, ErrInvalidIDToken
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

type devVerifier struct{}

func NewDevVerifier() Verifier { return devVerifier{} }

func (devVerifier) Verify(_ context.Context, token string) (*GoogleUser, error) {
	email, ok := strings.CutPrefix(token, "dev:")
	email = strings.ToLower(strings.TrimSpace(email))
	if !ok || !strings.Contains(email, "@") {
		return nil, ErrInvalidIDToken
	}
	local := email[:strings.IndexByte(email, '@')]
	return &GoogleUser{Sub: "dev-" + email, Email: email, Name: local}, nil
}
