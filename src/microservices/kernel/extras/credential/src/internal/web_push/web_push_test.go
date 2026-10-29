package web_push

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// LE CHIFFREMENT EST CELUI DE LA RFC, OCTET POUR OCTET : le vecteur de son
// annexe A (RFC 8291, « Intermediate Values for Encryption »). La version
// d'avant ne produisait ni l'en-tête ni la bonne dérivation, et Chrome jetait
// chaque notification au déchiffrement (2026-10-15).
func TestEncryptionMatchesRFC8291AppendixA(t *testing.T) {
	decode := func(value string) []byte {
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	senderKey, err := ecdh.P256().NewPrivateKey(decode("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	subscription := Subscription{
		P256DH: "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
		Auth:   "BTBZMqHH6r4Tts7J_aSIgg",
	}

	body, err := encryptAES128GCM(subscription, []byte("When I grow up, I want to be a watermelon"), senderKey, decode("DGv6ra1nlYgDCS1FRnbzlw"))
	if err != nil {
		t.Fatal(err)
	}

	const want = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if got := base64.RawURLEncoding.EncodeToString(body); got != want {
		t.Fatalf("message chiffré :\n got %s\nwant %s", got, want)
	}
}

// CHAQUE MESSAGE A SA CLÉ ET SON SEL (RFC 8291 §3.1) : deux chiffrements du
// même texte n'ont ni le même sel ni la même clé d'émetteur, et l'en-tête
// porte une clé publique non compressée de 65 octets — ce que Chrome exige.
func TestEveryMessageHasItsOwnKeyAndSalt(t *testing.T) {
	receiver, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	vapid, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(base64.RawURLEncoding.EncodeToString(vapid.Bytes()), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	subscription := Subscription{
		P256DH: base64.RawURLEncoding.EncodeToString(receiver.PublicKey().Bytes()),
		Auth:   base64.RawURLEncoding.EncodeToString(auth),
	}

	first, err := client.EncryptPayload(subscription, []byte(`{"type":"invite"}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.EncryptPayload(subscription, []byte(`{"type":"invite"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{first, second} {
		if body[20] != 65 || body[21] != 0x04 {
			t.Fatalf("en-tête : longueur de clé %d, premier octet %#x — want 65 et 0x04", body[20], body[21])
		}
	}
	if string(first[:16]) == string(second[:16]) || string(first[21:86]) == string(second[21:86]) {
		t.Fatal("deux messages partagent leur sel ou leur clé d'émetteur")
	}
}
