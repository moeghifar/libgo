package xttp

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	Subject string
	Roles   []string
}

type principalKey struct{}

// SetPrincipal records the caller for later handlers. Call it from your auth
// middleware.
func SetPrincipal(c fiber.Ctx, p Principal) { fiber.Locals(c, principalKey{}, p) }

// PrincipalOf returns the caller recorded by SetPrincipal.
func PrincipalOf(c fiber.Ctx) (Principal, bool) {
	p, ok := c.Locals(principalKey{}).(Principal)
	return p, ok
}

var errUnauthorized = Error{Status: fiber.StatusUnauthorized, Code: "HTTP-401", Title: "Unauthorized", Detail: "missing or invalid credentials"}

// secrets hashes the accepted values, so a comparison takes the same time
// whatever the length of the guess. It panics on an empty list or an empty
// value: a misconfigured secret must stop startup, not open the route.
func secrets(kind string, values []string) [][sha256.Size]byte {
	if len(values) == 0 {
		panic("xttp: " + kind + " needs at least one value")
	}
	out := make([][sha256.Size]byte, len(values))
	for i, v := range values {
		if v == "" {
			panic("xttp: " + kind + " got an empty value")
		}
		out[i] = sha256.Sum256([]byte(v))
	}
	return out
}

// matches compares against every value, without stopping at the first hit.
func matches(accepted [][sha256.Size]byte, got string) bool {
	sum := sha256.Sum256([]byte(got))
	found := 0
	for i := range accepted {
		found |= subtle.ConstantTimeCompare(accepted[i][:], sum[:])
	}
	return found == 1
}

// BearerTokens accepts requests with "Authorization: Bearer <token>" for one
// of tokens, and answers 401 otherwise. It panics on an empty list.
func BearerTokens(tokens ...string) fiber.Handler {
	accepted := secrets("BearerTokens", tokens)
	return func(c fiber.Ctx) error {
		scheme, token, ok := strings.Cut(c.Get(fiber.HeaderAuthorization), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || !matches(accepted, token) {
			c.Set(fiber.HeaderWWWAuthenticate, "Bearer")
			return errUnauthorized.With(errUnauthorized.Detail, nil)
		}
		return c.Next()
	}
}

// APIKey accepts requests whose header holds one of keys, and answers 401
// otherwise. It panics on an empty list.
func APIKey(header string, keys ...string) fiber.Handler {
	accepted := secrets("APIKey", keys)
	return func(c fiber.Ctx) error {
		if !matches(accepted, c.Get(header)) {
			return errUnauthorized.With(errUnauthorized.Detail, nil)
		}
		return c.Next()
	}
}
