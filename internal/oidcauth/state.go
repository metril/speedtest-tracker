package oidcauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// stateTTL bounds how long an encoded state cookie value remains valid.
const stateTTL = 5 * time.Minute

// ErrStateInvalid is returned by StateCodec.Decode when s is malformed,
// tampered with, or expired.
var ErrStateInvalid = errors.New("oidcauth: invalid state")

// StateCodec signs and verifies the opaque value stored in the OIDC login
// flow's state cookie, so the server needs no session storage between the
// /auth/oidc/start redirect and the /auth/oidc/callback request.
type StateCodec struct {
	key []byte
}

// NewStateCodec returns a StateCodec keyed with fresh random bytes.
func NewStateCodec() (*StateCodec, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("oidcauth: generate state key: %w", err)
	}
	return &StateCodec{key: key}, nil
}

// Encode packs state, the PKCE verifier and returnTo into a signed,
// self-expiring token (valid for 5 minutes from now).
func (c *StateCodec) Encode(state, verifier, returnTo string) string {
	exp := time.Now().Add(stateTTL).Unix()
	payload := strings.Join([]string{state, verifier, returnTo, strconv.FormatInt(exp, 10)}, "\x00")
	sig := c.sign([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// Decode verifies and unpacks a value produced by Encode. It fails with
// ErrStateInvalid if the signature does not match, the payload is
// malformed, or the encoded expiry is at or before now.
func (c *StateCodec) Decode(s string, now time.Time) (state, verifier, returnTo string, err error) {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return "", "", "", ErrStateInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(s[:dot])
	if err != nil {
		return "", "", "", ErrStateInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(s[dot+1:])
	if err != nil {
		return "", "", "", ErrStateInvalid
	}
	if !hmac.Equal(sig, c.sign(payload)) {
		return "", "", "", ErrStateInvalid
	}

	fields := strings.Split(string(payload), "\x00")
	if len(fields) != 4 {
		return "", "", "", ErrStateInvalid
	}
	exp, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return "", "", "", ErrStateInvalid
	}
	if !now.Before(time.Unix(exp, 0)) {
		return "", "", "", ErrStateInvalid
	}
	return fields[0], fields[1], fields[2], nil
}

func (c *StateCodec) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write(payload)
	return mac.Sum(nil)
}
