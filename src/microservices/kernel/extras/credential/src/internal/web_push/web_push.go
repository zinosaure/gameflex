// Package web_push implements the browser-side subset of Web Push:
// aes128gcm message encryption (RFC 8291) and VAPID authentication
// (RFC 8292, ES256) using only the standard library plus x/crypto/hkdf.
package web_push

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/hkdf"
)

const (
	contentEncoding = "aes128gcm"
	pushTTL         = "60"
)

var ErrSubscriptionGone = errors.New("push subscription gone")

type Subscription struct {
	Endpoint string `json:"endpoint"`
	P256DH   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

type Client struct {
	ecdsaPrivateKey *ecdsa.PrivateKey
	publicKey       []byte // uncompressed, for VAPID Authorization header
	contactEmail    string
	httpClient      *http.Client
}

// NewClient builds a VAPID client from a raw (base64url) P-256 private key.
func NewClient(privateKeyBase64URL, contactEmail string) (*Client, error) {
	raw, err := base64.RawURLEncoding.DecodeString(privateKeyBase64URL)
	if err != nil {
		return nil, fmt.Errorf("decode vapid private key: %w", err)
	}

	privateKey, err := ecdh.P256().NewPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("build vapid private key: %w", err)
	}
	ecdsaKey, err := ecdsaPrivateFromBigInt(raw)
	if err != nil {
		return nil, fmt.Errorf("build vapid ecdsa key: %w", err)
	}

	return &Client{
		ecdsaPrivateKey: ecdsaKey,
		publicKey:       privateKey.PublicKey().Bytes(),
		contactEmail:    contactEmail,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// PublicKeyBase64URL exposes the public key for /api/v1/push/vapid-public-key.
func (client *Client) PublicKeyBase64URL() string {
	return base64.RawURLEncoding.EncodeToString(client.publicKey)
}

func ecdsaPrivateFromBigInt(raw []byte) (*ecdsa.PrivateKey, error) {
	key := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: elliptic.P256()},
		D:         new(big.Int).SetBytes(raw),
	}
	key.PublicKey.X, key.PublicKey.Y = key.Curve.ScalarBaseMult(key.D.Bytes())
	return key, nil
}

// recordSize is the aes128gcm record size written in the header (RFC 8188):
// the single record of a push message must fit in it, tag and delimiter
// included — about 4 KiB of payload, far above an invitation.
const recordSize = 4096

// EncryptPayload produces the aes128gcm body of a push message — RFC 8291
// (Message Encryption for Web Push) over RFC 8188, with a key pair and a salt
// of its own for every message.
//
// THE PREVIOUS VERSION WAS NOT THAT (found 2026-10-15, the first time a push
// was followed into a browser): no header at all — the salt, the record size
// and the sender's public key that RFC 8188 puts before the ciphertext —, the
// VAPID key reused as the encryption key, and a key derivation that skipped
// the salted HKDF of RFC 8188. The push service delivers such a message
// without complaint; Chrome drops it at decryption ("The public key included
// in the binary message header must be a valid P-256 ECDH uncompressed point
// that is 65 bytes in length", chrome://gcm-internals). No push notification
// had ever been readable.
func (client *Client) EncryptPayload(subscription Subscription, payload []byte) ([]byte, error) {
	senderKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate message key: %w", err)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	return encryptAES128GCM(subscription, payload, senderKey, salt)
}

// encryptAES128GCM — EncryptPayload with its randomness handed in, which is
// what lets the RFC 8291 test vector (Appendix A) check it byte for byte.
func encryptAES128GCM(subscription Subscription, payload []byte, senderKey *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(payload)+1+16 > recordSize {
		return nil, fmt.Errorf("push payload of %d bytes does not fit one %d-byte record", len(payload), recordSize)
	}
	receiverPublicRaw, err := base64.RawURLEncoding.DecodeString(subscription.P256DH)
	if err != nil {
		return nil, fmt.Errorf("decode p256dh: %w", err)
	}
	receiverPublic, err := ecdh.P256().NewPublicKey(receiverPublicRaw)
	if err != nil {
		return nil, fmt.Errorf("build p256dh: %w", err)
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(subscription.Auth)
	if err != nil {
		return nil, fmt.Errorf("decode auth: %w", err)
	}
	ecdhSecret, err := senderKey.ECDH(receiverPublic)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}
	senderPublicRaw := senderKey.PublicKey().Bytes()

	// RFC 8291 §3.3-3.4 — the input keying material, bound to both public
	// keys and to the subscription's authentication secret.
	prkKey := hkdf.Extract(sha256.New, ecdhSecret, authSecret)
	keyInfo := append(append([]byte("WebPush: info\x00"), receiverPublicRaw...), senderPublicRaw...)
	ikm := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.Expand(sha256.New, prkKey, keyInfo), ikm); err != nil {
		return nil, fmt.Errorf("derive input keying material: %w", err)
	}

	// RFC 8188 §2.2 — the content key and the nonce, under this message's salt.
	prk := hkdf.Extract(sha256.New, ikm, salt)
	contentKey := make([]byte, 16)
	if _, err := io.ReadFull(hkdf.Expand(sha256.New, prk, []byte("Content-Encoding: aes128gcm\x00")), contentKey); err != nil {
		return nil, fmt.Errorf("derive content key: %w", err)
	}
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(hkdf.Expand(sha256.New, prk, []byte("Content-Encoding: nonce\x00")), nonce); err != nil {
		return nil, fmt.Errorf("derive nonce: %w", err)
	}

	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return nil, fmt.Errorf("build aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build gcm: %w", err)
	}

	// One record, the last: the payload, then the 0x02 delimiter, no padding.
	plaintext := append(append([]byte{}, payload...), 0x02)

	// The header: salt (16), record size (uint32), key id length, key id —
	// the sender's public key, which the receiver needs to derive the same
	// secret.
	header := make([]byte, 0, 16+4+1+len(senderPublicRaw))
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, recordSize)
	header = append(header, byte(len(senderPublicRaw)))
	header = append(header, senderPublicRaw...)

	return gcm.Seal(header, nonce, plaintext, nil), nil
}

// Send delivers a payload to one subscription. Returns
// ErrSubscriptionGone when the push service answers 404/410 (the caller
// deletes the subscription) and a plain error for transport failures.
func (client *Client) Send(subscription Subscription, payload []byte) error {
	body, err := client.EncryptPayload(subscription, payload)
	if err != nil {
		return err
	}

	authorization, err := client.vapidAuthorization(subscription.Endpoint)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPost, subscription.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}
	request.Header.Set("Content-Encoding", contentEncoding)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("TTL", pushTTL)
	request.Header.Set("Authorization", authorization)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("push request: %w", err)
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return nil
	case http.StatusNotFound, http.StatusGone:
		return ErrSubscriptionGone
	default:
		return fmt.Errorf("push service answered %d", response.StatusCode)
	}
}

// vapidAuthorization builds the "vapid t=<jwt>, k=<key>" header value.
func (client *Client) vapidAuthorization(endpoint string) (string, error) {
	origin, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}
	audience := origin.Scheme + "://" + origin.Host

	header := map[string]any{"typ": "JWT", "alg": "ES256"}
	claims := map[string]any{
		"aud": audience,
		"exp": time.Now().Add(12 * time.Hour).Unix(),
		"sub": "mailto:" + client.contactEmail,
	}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))

	r, s, err := ecdsa.Sign(rand.Reader, client.ecdsaPrivateKey, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign vapid jwt: %w", err)
	}
	// ES256 signature: raw r||s, 32 bytes each, ASN.1 unpacked.
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])

	jwt := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	return "vapid t=" + jwt + ", k=" + base64.RawURLEncoding.EncodeToString(client.publicKey), nil
}
