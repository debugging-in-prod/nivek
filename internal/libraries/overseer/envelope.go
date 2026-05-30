package overseer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
	"time"
)

// MarshalEnvelope serializes payload as the Data field of an Envelope of the
// given type and sequence number, computes an HMAC-SHA256 signature over the
// canonical form (Type | Seq | SentAt-RFC3339Nano | Data bytes), and returns
// the marshaled envelope as JSON.
func MarshalEnvelope(typ wire.EnvelopeType, seq uint64, payload any, hmacKey []byte) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	env := wire.Envelope{
		Type:   typ,
		Seq:    seq,
		SentAt: time.Now().UTC(),
		Data:   data,
	}
	env.HMAC = computeHMAC(&env, hmacKey)
	return json.Marshal(env)
}

// UnmarshalEnvelope parses an envelope JSON blob and verifies its HMAC
// signature against the provided key. Caller is responsible for sequence
// number replay protection (compare env.Seq against last-seen on the conn).
func UnmarshalEnvelope(buf []byte, hmacKey []byte) (*wire.Envelope, error) {
	var env wire.Envelope
	if err := json.Unmarshal(buf, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	expected := computeHMAC(&env, hmacKey)
	if !hmac.Equal([]byte(env.HMAC), []byte(expected)) {
		return nil, errors.New("envelope HMAC verification failed")
	}
	return &env, nil
}

func computeHMAC(env *wire.Envelope, key []byte) string {
	h := hmac.New(sha256.New, key)
	fmt.Fprintf(h, "%s\n%d\n%s\n", env.Type, env.Seq, env.SentAt.Format(time.RFC3339Nano))
	h.Write(env.Data)
	return hex.EncodeToString(h.Sum(nil))
}
