package extractor

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"

	oidc "github.com/coreos/go-oidc"
	"github.com/pkg/errors"
)

const tokenFieldIDToken = "id_token"

// ErrMissingIDToken indicates a response that does not contain an id_token.
var ErrMissingIDToken = errors.New("response missing ID token")

// OIDCAuthenticationParams are the parameters required for kubectl to
// authenticate to Kubernetes via OIDC.
type OIDCAuthenticationParams struct {
	Username     string `json:"email" schema:"email"` // TODO(negz): Support other claims.
	ClientID     string `json:"clientID" schema:"clientID"`
	ClientSecret string `json:"clientSecret" schema:"clientSecret"`
	IDToken      string `json:"idToken" schema:"idToken"`
	RefreshToken string `json:"refreshToken" schema:"refreshToken"`
	IssuerURL    string `json:"issuer" schema:"issuer"`
}

// An OIDC extractor performs OIDC validation, extracting and storing the
// information required for Kubernetes authentication along the way.
type OIDC interface {
	Process(ctx context.Context, cfg *oauth2.Config, code string) (*OIDCAuthenticationParams, error)
}

type oidcExtractor struct {
	v *oidc.IDTokenVerifier
	h *http.Client
}

// An Option represents a OIDC extractor option.
type Option func(*oidcExtractor) error

// HTTPClient allows the use of a bespoke context.
func HTTPClient(h *http.Client) Option {
	return func(o *oidcExtractor) error {
		o.h = h
		return nil
	}
}

// NewOIDC creates a new OIDC extractor.
func NewOIDC(v *oidc.IDTokenVerifier, oo ...Option) (OIDC, error) {
	oe := &oidcExtractor{v: v, h: http.DefaultClient}

	for _, o := range oo {
		if err := o(oe); err != nil {
			return nil, errors.Wrap(err, "cannot apply OIDC option")
		}
	}
	return oe, nil
}

func (o *oidcExtractor) Process(ctx context.Context, cfg *oauth2.Config, code string) (*OIDCAuthenticationParams, error) {
	octx := oidc.ClientContext(ctx, o.h)
	token, err := cfg.Exchange(octx, code)
	if err != nil {
		return nil, errors.Wrap(err, "cannot exchange code for token")
	}

	id, ok := token.Extra(tokenFieldIDToken).(string)
	if !ok {
		return nil, ErrMissingIDToken
	}

	idt, err := o.v.Verify(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "cannot verify ID token")
	}

	params := &OIDCAuthenticationParams{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		IDToken:      id,
		RefreshToken: token.RefreshToken,
		IssuerURL:    idt.Issuer,
	}
	if err := idt.Claims(params); err != nil {
		return nil, errors.Wrap(err, "cannot extract claims from ID token")
	}
	return params, nil
}