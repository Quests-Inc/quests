package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 / authenticator apps use HMAC-SHA1
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP per RFC 6238: HMAC-SHA1, 6 digits, 30-second steps — what every authenticator
// app (Google Authenticator, Aegis, 1Password, …) speaks.
const (
	totpDigits = 6
	totpPeriod = 30
	// totpSkew accepts the previous and the next step as well: clock drift between a phone and the server.
	totpSkew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a fresh 160-bit secret, base32 (the form apps import).
func NewTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return b32.EncodeToString(raw), nil
}

// totpCode is the code for one time step.
func totpCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, bin%mod), nil
}

// StepAt is the 30-second step number of t.
func StepAt(t time.Time) int64 { return t.Unix() / totpPeriod }

// MatchTOTP checks a typed code against the secret around time now. It returns the step the
// code belongs to; the caller must refuse a step it has already accepted (replay).
func MatchTOTP(secret, code string, now time.Time) (step int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	cur := StepAt(now)
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		want, err := totpCode(secret, cur+d)
		if err != nil {
			return 0, false
		}
		// Compare every candidate in constant time; keep the match, do not return early on a hit.
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			step, ok = cur+d, true
		}
	}
	return step, ok
}

// TOTPURI is the otpauth:// link an authenticator app imports (shown as a QR code).
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpPeriod))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
