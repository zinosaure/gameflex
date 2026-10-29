package gatewayclient

// LE SCELLEMENT — streamex et Remote Play ne parlent qu'à NOTRE passerelle
// (chantier 34, P9, décidé par le porteur le 2026-10-28 : « Remoteplay et
// Streamex ne doit reconnaître que notre GatewayWebRTC », option 2).
//
// La poignée de main :
//
//	partie → passerelle  register.seal = clé X25519 éphémère de la partie
//	passerelle → partie  registered.proof = « registered:<session> », scellé
//
// Le secret partagé (X25519 entre la clé éphémère et la clé STATIQUE de la
// passerelle) donne par HKDF-SHA256 deux clés AES-256-GCM, une par sens ; le
// sel est l'identifiant de la partie. Seule une passerelle qui tient la clé
// privée retrouve ces clés : une autre ne sait ni prouver, ni lire.
//
// Ensuite TOUT est scellé, dans les deux sens : les images et le son (la
// charge utile, l'en-tête de 9 octets en donnée associée), les messages texte
// dans une enveloppe {"type":"sealed","data":…}. Une passerelle tierce qui
// relaierait la poignée de main vers une vraie, pour obtenir la preuve, ne
// recevrait ensuite que du chiffré.
//
// LE PROTOCOLE EST OUVERT, ET C'EST VOULU : sa sûreté tient à la clé privée
// de la passerelle, qui est fermée, pas au secret de ce code. Le rétro, ouvert,
// ne scelle pas (Dial) : il parle à n'importe quelle passerelle.
//
// Le nonce est un compteur de 64 bits par sens — le WebSocket garde l'ordre,
// et chaque connexion refait sa poignée de main, donc ses compteurs.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// DevGatewayKey — la clé PUBLIQUE de la passerelle de développement. Celle
// d'une version publiée est tirée par tools/publish.sh, pour cette version
// seulement, et posée à la construction (-ldflags -X).
const DevGatewayKey = "oPNWPvwpSNoPpjsdMKRytxmn6wjfdjFZ3irdUI82_Ug"

const sealInfo = "gameflex/gatewayclient: seal v1"

var (
	// ErrGatewayNotRecognized — la passerelle n'a pas prouvé qu'elle tient la
	// clé : ce n'est pas la nôtre, et la partie ne lui envoie rien.
	ErrGatewayNotRecognized = errors.New("gatewayclient: gateway not recognized")
	errMalformedKey         = errors.New("gatewayclient: malformed key")
	errSealBroken           = errors.New("gatewayclient: sealed message does not open")
)

// Seal — une connexion scellée : une clé par sens, et leurs compteurs. Pas
// de verrou à lui : chaque côté écrit déjà sous un seul verrou (`writeMu`) et
// lit sur un seul fil.
type Seal struct {
	send, receive  cipher.AEAD
	sent, received uint64
}

// NewGatewayKeyPair — une paire X25519 pour une passerelle, encodée comme
// les clés voyagent (base64url sans bourrage).
func NewGatewayKeyPair() (private, public string, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	return encodeKey(key.Bytes()), encodeKey(key.PublicKey().Bytes()), nil
}

// ParseGatewayPrivateKey — la clé que la passerelle garde.
func ParseGatewayPrivateKey(text string) (*ecdh.PrivateKey, error) {
	raw, err := decodeKey(text)
	if err != nil {
		return nil, err
	}

	return ecdh.X25519().NewPrivateKey(raw)
}

// ParseGatewayPublicKey — la clé que streamex et Remote Play reconnaissent.
func ParseGatewayPublicKey(text string) (*ecdh.PublicKey, error) {
	raw, err := decodeKey(text)
	if err != nil {
		return nil, err
	}

	return ecdh.X25519().NewPublicKey(raw)
}

// AcceptSeal — côté passerelle : la clé de la partie, reçue dans `register`,
// donne la connexion scellée et la preuve à rendre dans `registered`.
func AcceptSeal(gateway *ecdh.PrivateKey, sessionID, partyKey string) (*Seal, string, error) {
	raw, err := decodeKey(partyKey)
	if err != nil {
		return nil, "", err
	}
	party, err := ecdh.X25519().NewPublicKey(raw)
	if err != nil {
		return nil, "", errMalformedKey
	}
	shared, err := gateway.ECDH(party)
	if err != nil {
		return nil, "", err
	}
	towardGateway, towardParty, err := sealKeys(shared, sessionID, party.Bytes(), gateway.PublicKey().Bytes())
	if err != nil {
		return nil, "", err
	}
	seal := &Seal{send: towardParty, receive: towardGateway}

	return seal, base64.StdEncoding.EncodeToString(seal.close([]byte("registered:"+sessionID), nil)), nil
}

// offerSeal — côté partie : la clé éphémère à envoyer dans `register`, et de
// quoi finir quand la passerelle répond.
func offerSeal(gateway *ecdh.PublicKey, sessionID string) (string, func(proof string) (*Seal, error), error) {
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, err
	}
	finish := func(proof string) (*Seal, error) {
		shared, err := ephemeral.ECDH(gateway)
		if err != nil {
			return nil, err
		}
		towardGateway, towardParty, err := sealKeys(shared, sessionID, ephemeral.PublicKey().Bytes(), gateway.Bytes())
		if err != nil {
			return nil, err
		}
		seal := &Seal{send: towardGateway, receive: towardParty}
		sealed, err := base64.StdEncoding.DecodeString(proof)
		if err != nil {
			return nil, ErrGatewayNotRecognized
		}
		opened, err := seal.open(sealed, nil)
		if err != nil || string(opened) != "registered:"+sessionID {
			return nil, ErrGatewayNotRecognized
		}

		return seal, nil
	}

	return encodeKey(ephemeral.PublicKey().Bytes()), finish, nil
}

// SealFrame — une image ou un paquet de son : l'en-tête en clair (le genre,
// la durée), lié à la charge utile scellée.
func (seal *Seal) SealFrame(kind byte, duration time.Duration, payload []byte) []byte {
	header := make([]byte, 9, 9+len(payload)+seal.send.Overhead())
	header[0] = kind
	binary.BigEndian.PutUint64(header[1:9], uint64(duration))

	return append(header, seal.close(payload, header)...)
}

// OpenFrame — l'inverse, côté passerelle.
func (seal *Seal) OpenFrame(frame []byte) (byte, time.Duration, []byte, error) {
	if len(frame) < 9 {
		return 0, 0, nil, errSealBroken
	}
	payload, err := seal.open(frame[9:], frame[:9])
	if err != nil {
		return 0, 0, nil, err
	}

	return frame[0], time.Duration(binary.BigEndian.Uint64(frame[1:9])), payload, nil
}

// SealText — un message texte, dans son enveloppe.
func (seal *Seal) SealText(message []byte) []byte {
	envelope, _ := json.Marshal(map[string]string{
		"type": "sealed",
		"data": base64.StdEncoding.EncodeToString(seal.close(message, nil)),
	})

	return envelope
}

// OpenText — l'inverse. Un message en clair sur une connexion scellée ne
// s'ouvre pas : il n'a pu venir que d'un tiers.
func (seal *Seal) OpenText(envelope []byte) ([]byte, error) {
	var sealed struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	if json.Unmarshal(envelope, &sealed) != nil || sealed.Type != "sealed" {
		return nil, errSealBroken
	}
	raw, err := base64.StdEncoding.DecodeString(sealed.Data)
	if err != nil {
		return nil, errSealBroken
	}

	return seal.open(raw, nil)
}

func (seal *Seal) close(plaintext, additional []byte) []byte {
	nonce := counterNonce(seal.sent)
	seal.sent++

	return seal.send.Seal(nil, nonce, plaintext, additional)
}

func (seal *Seal) open(sealed, additional []byte) ([]byte, error) {
	opened, err := seal.receive.Open(nil, counterNonce(seal.received), sealed, additional)
	if err != nil {
		return nil, errSealBroken
	}
	seal.received++

	return opened, nil
}

func counterNonce(counter uint64) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce[4:], counter)

	return nonce
}

// sealKeys — HKDF-SHA256 (RFC 5869) sur le secret partagé, salé par la
// partie, lié aux deux clés publiques : 32 octets vers la passerelle, 32 vers
// la partie.
func sealKeys(shared []byte, sessionID string, partyKey, gatewayKey []byte) (cipher.AEAD, cipher.AEAD, error) {
	extract := hmac.New(sha256.New, []byte(sessionID))
	extract.Write(shared)
	pseudoRandom := extract.Sum(nil)

	info := append(append([]byte(sealInfo), partyKey...), gatewayKey...)
	material := []byte{}
	block := []byte{}
	for counter := byte(1); len(material) < 64; counter++ {
		expand := hmac.New(sha256.New, pseudoRandom)
		expand.Write(block)
		expand.Write(info)
		expand.Write([]byte{counter})
		block = expand.Sum(nil)
		material = append(material, block...)
	}

	towardGateway, err := gcm(material[:32])
	if err != nil {
		return nil, nil, err
	}
	towardParty, err := gcm(material[32:64])
	if err != nil {
		return nil, nil, err
	}

	return towardGateway, towardParty, nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

func encodeKey(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeKey(text string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(raw) != 32 {
		return nil, errMalformedKey
	}

	return raw, nil
}
