package crypto

import (
	"bytes"
	"testing"
)

func TestGenerateEphemeralKeypair(t *testing.T) {
	priv1, pub1, err := GenerateEphemeralKeypair()
	if err != nil {
		t.Fatalf("GenerateEphemeralKeypair() error = %v", err)
	}

	// Check keys are not zero
	var zeroKey [KeySize]byte
	if priv1 == zeroKey {
		t.Error("private key is zero")
	}
	if pub1 == zeroKey {
		t.Error("public key is zero")
	}

	// Generate another keypair and verify they are different
	priv2, pub2, err := GenerateEphemeralKeypair()
	if err != nil {
		t.Fatalf("GenerateEphemeralKeypair() second call error = %v", err)
	}

	if priv1 == priv2 {
		t.Error("two generated private keys are identical")
	}
	if pub1 == pub2 {
		t.Error("two generated public keys are identical")
	}
}

func TestComputeECDH(t *testing.T) {
	// Generate two keypairs
	privA, pubA, err := GenerateEphemeralKeypair()
	if err != nil {
		t.Fatalf("GenerateEphemeralKeypair() A error = %v", err)
	}

	privB, pubB, err := GenerateEphemeralKeypair()
	if err != nil {
		t.Fatalf("GenerateEphemeralKeypair() B error = %v", err)
	}

	// Both sides should derive the same shared secret
	secretA, err := ComputeECDH(privA, pubB)
	if err != nil {
		t.Fatalf("ComputeECDH(A, pubB) error = %v", err)
	}

	secretB, err := ComputeECDH(privB, pubA)
	if err != nil {
		t.Fatalf("ComputeECDH(B, pubA) error = %v", err)
	}

	if secretA != secretB {
		t.Error("shared secrets do not match")
	}

	// Shared secret should not be zero
	var zeroKey [KeySize]byte
	if secretA == zeroKey {
		t.Error("shared secret is zero")
	}
}

func TestComputeECDH_ZeroKey(t *testing.T) {
	priv, _, err := GenerateEphemeralKeypair()
	if err != nil {
		t.Fatalf("GenerateEphemeralKeypair() error = %v", err)
	}

	// Should fail with zero public key
	var zeroKey [KeySize]byte
	_, err = ComputeECDH(priv, zeroKey)
	if err == nil {
		t.Error("ComputeECDH with zero public key should fail")
	}
}

func TestDeriveSessionKey(t *testing.T) {
	privA, pubA, _ := GenerateEphemeralKeypair()
	privB, pubB, _ := GenerateEphemeralKeypair()

	secretA, _ := ComputeECDH(privA, pubB)
	secretB, _ := ComputeECDH(privB, pubA)

	streamID := uint64(42)

	// Derive session keys on both sides
	skA := DeriveSessionKey(secretA, streamID, pubA, pubB, true)  // initiator
	skB := DeriveSessionKey(secretB, streamID, pubA, pubB, false) // responder

	// Keys should be identical
	if skA.Key() != skB.Key() {
		t.Error("derived session keys do not match")
	}

	// Keys should not be zero
	var zeroKey [KeySize]byte
	if skA.Key() == zeroKey {
		t.Error("session key is zero")
	}
}

func TestDeriveSessionKey_UniquePerStream(t *testing.T) {
	priv, pub, _ := GenerateEphemeralKeypair()
	secret, _ := ComputeECDH(priv, pub) // Self-ECDH for testing

	sk1 := DeriveSessionKey(secret, 1, pub, pub, true)
	sk2 := DeriveSessionKey(secret, 2, pub, pub, true)

	if sk1.Key() == sk2.Key() {
		t.Error("session keys for different streams should be different")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	privA, pubA, _ := GenerateEphemeralKeypair()
	privB, pubB, _ := GenerateEphemeralKeypair()

	secretA, _ := ComputeECDH(privA, pubB)
	secretB, _ := ComputeECDH(privB, pubA)

	streamID := uint64(123)

	skA := DeriveSessionKey(secretA, streamID, pubA, pubB, true)  // initiator
	skB := DeriveSessionKey(secretB, streamID, pubA, pubB, false) // responder

	// Test initiator -> responder
	plaintext := []byte("Hello, World!")
	ciphertext, err := skA.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Ciphertext should be larger by EncryptionOverhead
	if len(ciphertext) != len(plaintext)+EncryptionOverhead {
		t.Errorf("ciphertext length = %d, want %d", len(ciphertext), len(plaintext)+EncryptionOverhead)
	}

	// Ciphertext should be different from plaintext
	if bytes.Equal(ciphertext[NonceSize:NonceSize+len(plaintext)], plaintext) {
		t.Error("ciphertext contains plaintext (encryption did nothing)")
	}

	// Responder should be able to decrypt
	decrypted, err := skB.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_BothDirections(t *testing.T) {
	privA, pubA, _ := GenerateEphemeralKeypair()
	privB, pubB, _ := GenerateEphemeralKeypair()

	secretA, _ := ComputeECDH(privA, pubB)
	secretB, _ := ComputeECDH(privB, pubA)

	streamID := uint64(456)

	skA := DeriveSessionKey(secretA, streamID, pubA, pubB, true)  // initiator
	skB := DeriveSessionKey(secretB, streamID, pubA, pubB, false) // responder

	// Initiator -> Responder
	msg1 := []byte("Message from initiator")
	enc1, _ := skA.Encrypt(msg1)
	dec1, err := skB.Decrypt(enc1)
	if err != nil {
		t.Fatalf("Decrypt initiator->responder error = %v", err)
	}
	if !bytes.Equal(dec1, msg1) {
		t.Errorf("initiator->responder: got %q, want %q", dec1, msg1)
	}

	// Responder -> Initiator
	msg2 := []byte("Reply from responder")
	enc2, _ := skB.Encrypt(msg2)
	dec2, err := skA.Decrypt(enc2)
	if err != nil {
		t.Fatalf("Decrypt responder->initiator error = %v", err)
	}
	if !bytes.Equal(dec2, msg2) {
		t.Errorf("responder->initiator: got %q, want %q", dec2, msg2)
	}
}

func TestEncryptDecrypt_MultipleMessages(t *testing.T) {
	privA, pubA, _ := GenerateEphemeralKeypair()
	privB, pubB, _ := GenerateEphemeralKeypair()

	secretA, _ := ComputeECDH(privA, pubB)
	secretB, _ := ComputeECDH(privB, pubA)

	skA := DeriveSessionKey(secretA, 789, pubA, pubB, true)
	skB := DeriveSessionKey(secretB, 789, pubA, pubB, false)

	// Send multiple messages
	messages := []string{
		"First message",
		"Second message",
		"Third message with more content",
		"",                                    // Empty message
		string(make([]byte, 16000)),           // Large message
	}

	for i, msg := range messages {
		enc, err := skA.Encrypt([]byte(msg))
		if err != nil {
			t.Fatalf("Encrypt message %d error = %v", i, err)
		}

		dec, err := skB.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt message %d error = %v", i, err)
		}

		if !bytes.Equal(dec, []byte(msg)) {
			t.Errorf("message %d: got len=%d, want len=%d", i, len(dec), len(msg))
		}
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	priv, pub, _ := GenerateEphemeralKeypair()
	secret, _ := ComputeECDH(priv, pub)
	sk := DeriveSessionKey(secret, 1, pub, pub, true)

	// Ciphertext shorter than overhead should fail
	shortCiphertext := make([]byte, EncryptionOverhead-1)
	_, err := sk.Decrypt(shortCiphertext)
	if err == nil {
		t.Error("Decrypt with short ciphertext should fail")
	}
}

func TestDecrypt_Tampered(t *testing.T) {
	privA, pubA, _ := GenerateEphemeralKeypair()
	privB, pubB, _ := GenerateEphemeralKeypair()

	secretA, _ := ComputeECDH(privA, pubB)
	secretB, _ := ComputeECDH(privB, pubA)

	skA := DeriveSessionKey(secretA, 1, pubA, pubB, true)
	skB := DeriveSessionKey(secretB, 1, pubA, pubB, false)

	plaintext := []byte("Secret message")
	ciphertext, _ := skA.Encrypt(plaintext)

	// Tamper with ciphertext
	ciphertext[NonceSize+5] ^= 0xFF

	_, err := skB.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt with tampered ciphertext should fail")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	privA, pubA, _ := GenerateEphemeralKeypair()
	_, pubB, _ := GenerateEphemeralKeypair()
	_, pubC, _ := GenerateEphemeralKeypair()

	secretAB, _ := ComputeECDH(privA, pubB)
	secretAC, _ := ComputeECDH(privA, pubC)

	skAB := DeriveSessionKey(secretAB, 1, pubA, pubB, true)
	skAC := DeriveSessionKey(secretAC, 1, pubA, pubC, true) // Different key

	plaintext := []byte("Secret message")
	ciphertext, _ := skAB.Encrypt(plaintext)

	// Try to decrypt with wrong key
	_, err := skAC.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt with wrong key should fail")
	}
}

func TestZeroBytes(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	ZeroBytes(data)

	for i, b := range data {
		if b != 0 {
			t.Errorf("byte %d = %d, want 0", i, b)
		}
	}
}

func TestZeroKey(t *testing.T) {
	key := [KeySize]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	ZeroKey(&key)

	var zeroKey [KeySize]byte
	if key != zeroKey {
		t.Error("key was not zeroed")
	}
}

func TestEncryptionOverhead(t *testing.T) {
	// Verify the constant matches reality
	if EncryptionOverhead != NonceSize+TagSize {
		t.Errorf("EncryptionOverhead = %d, want %d", EncryptionOverhead, NonceSize+TagSize)
	}
	if EncryptionOverhead != 28 {
		t.Errorf("EncryptionOverhead = %d, want 28", EncryptionOverhead)
	}
}

func BenchmarkEncrypt(b *testing.B) {
	priv, pub, _ := GenerateEphemeralKeypair()
	secret, _ := ComputeECDH(priv, pub)
	sk := DeriveSessionKey(secret, 1, pub, pub, true)

	plaintext := make([]byte, 1400) // Typical MTU-sized payload

	b.ResetTimer()
	b.SetBytes(int64(len(plaintext)))

	for i := 0; i < b.N; i++ {
		_, _ = sk.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	priv, pub, _ := GenerateEphemeralKeypair()
	secret, _ := ComputeECDH(priv, pub)
	sk := DeriveSessionKey(secret, 1, pub, pub, true)

	plaintext := make([]byte, 1400)
	ciphertext, _ := sk.Encrypt(plaintext)

	b.ResetTimer()
	b.SetBytes(int64(len(plaintext)))

	for i := 0; i < b.N; i++ {
		sk.recvNonce = 0 // Reset for benchmark
		_, _ = sk.Decrypt(ciphertext)
	}
}

func BenchmarkKeyExchange(b *testing.B) {
	for i := 0; i < b.N; i++ {
		privA, pubA, _ := GenerateEphemeralKeypair()
		_, pubB, _ := GenerateEphemeralKeypair()

		secretA, _ := ComputeECDH(privA, pubB)
		_ = DeriveSessionKey(secretA, uint64(i), pubA, pubB, true)
	}
}
