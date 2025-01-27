// Copyright (C) 2018 Authors
// distributed under Apache 2.0 license

package bgls

// This file is for Knowledge of secret key (Kosk) BLS. You do a proof to
// show that you know the secret key, to avoid the rogue public key attack,
// which is done in the authentication methods here.
//
// Proof of knowledge of the secret key is done in this library through
// doing a BLS signature on the public key itself as a message. There is a
// situation where this doesn't prove knowledge of the secret key. Suppose there
// is a BLS signing oracle for (pk1, sk1). Let pkA = -pk1 + x*g_2.
// Note that sig_pkA(pkA) = -sig_pk1(pkA) + xH(-pk1 + x*g_2)
// Consequently, if pk1 signs on pkA, this doesn't prove that person A knows skA
// and the rogue public key attack is possible as pkA is an authenticated key.
// One solution to fix this is to ensure that it
// is impossible for pk1 to sign the same pkA that is used in authentication.
// The way this is implemented here is to make the sign/verify methods prepend a
// 0x01 byte to any message that is being signed, and to make authentication
// prepend a null to public key before its signed. Since one would only ever
// authenticate their own public key, noone could get a signature from you that
// would work for their own authentication. (As all signatures you give out, other
// than your own authentication, have a 0x01 byte prepended instead of a null byte)
//
// This method sacrifices interoperability between KoskBls and normal BLS,
// however the advantage is that the authentications are aggregatable. They're
// aggregatable, since they are in effect, BLS signatures but all on distinct
// messages since they are distinct public keys.
//
// If you are using Kosk to secure against the rogue public key attack, you are
// intended to use: AggregateSignatures, KeyGen, KoskSign,
// KoskVerifySingleSignature, KoskVerifyMultiSignature
// KoskVerifyMultiSignatureWithMultiplicity, KoskVerifyAggregateSignature

import (
	"math/big"

	. "github.com/PeterCCLiu/bgls/curves" // nolint: golint
)

// Authenticate generates an Aggregatable Authentication for a given secret key.
// It signs the public key generated from sk, with a 0x01 byte prepended to it.
func Authenticate(curve CurveSystem, sk *big.Int) Point {
	return AuthenticateCustHash(curve, sk, curve.HashToG1)
}

// AuthenticateCustHash generates an Aggregatable Authentication for a given secret key.
// It signs the public key generated from sk, with a null byte prepended to it.
// This runs with the specified hash function.
func AuthenticateCustHash(curve CurveSystem, sk *big.Int, hash func([]byte) Point) Point {
	msg := LoadPublicKey(curve, sk).Marshal()
	msg = append(make([]byte, 0), msg...)
	return SignCustHash(sk, msg, hash)
}

// CheckAuthentication verifies that the provided signature is in fact authentication
// for this public key.
func CheckAuthentication(curve CurveSystem, pubkey Point, authentication Point) bool {
	return CheckAuthenticationCustHash(curve, pubkey, authentication, curve.HashToG1)
}

// CheckAuthenticationCustHash verifies that the provided signature is in fact authentication
// for this public key.
func CheckAuthenticationCustHash(curve CurveSystem, pubkey Point, authentication Point, hash func([]byte) Point) bool {
	msg := pubkey.Marshal()
	msg = append(make([]byte, 0), msg...)
	return VerifySingleSignatureCustHash(curve, authentication, pubkey, msg, hash)
}

// KoskSign creates a kosk signature on a message with a private key.
// A kosk signature prepends a 0x01 byte to the message before signing.
func KoskSign(curve CurveSystem, sk *big.Int, msg []byte) Point {
	return KoskSignCustHash(curve, sk, msg, curve.HashToG1)
}

// KoskSignCustHash creates a kosk signature on a message with a private key, using
// a supplied function to hash to point. A kosk signature prepends a 0x01 byte
// to the message before signing.
func KoskSignCustHash(curve CurveSystem, sk *big.Int, msg []byte, hash func([]byte) Point) Point {
	m := append([]byte{1}, msg...)
	return SignCustHash(sk, m, hash)
}

// KoskVerifySingleSignature checks that a single kosk signature is valid.
func KoskVerifySingleSignature(curve CurveSystem, sig Point, pubKey Point, msg []byte) bool {
	return KoskVerifySingleSignatureCustHash(curve, pubKey, msg, sig, curve.HashToG1)
}

// KoskVerifySingleSignatureCustHash checks that a single kosk signature is valid,
// with the supplied hash function.
func KoskVerifySingleSignatureCustHash(curve CurveSystem, pubKey Point, msg []byte,
	sig Point, hash func([]byte) Point) bool {
	m := append([]byte{1}, msg...)
	return VerifySingleSignature(curve, sig, pubKey, m)
}

// KoskVerifyAggregateSignature verifies that the aggregated signature proves
// that all messages were signed by the associated keys.
func KoskVerifyAggregateSignature(curve CurveSystem, aggsig Point, keys []Point, msgs [][]byte) bool {
	newMsgs := make([][]byte, len(msgs))
	for i := 0; i < len(msgs); i++ {
		newMsgs[i] = append([]byte{1}, msgs[i]...)
	}
	return verifyAggSig(curve, aggsig, keys, newMsgs, true)
}

// Verify checks that a single message has been signed by a set of keys
// vulnerable against rogue public-key attack, if keys have not been authenticated
func (m MultiSig) Verify(curve CurveSystem) bool {
	return KoskVerifyMultiSignature(curve, m.sig, m.keys, m.msg)
}

// KoskVerifyMultiSignature checks that the aggregate signature correctly proves
// that a single message has been signed by a set of keys,
// vulnerable against chosen key attack, if keys have not been authenticated
func KoskVerifyMultiSignature(curve CurveSystem, aggsig Point, keys []Point, msg []byte) bool {
	msg2 := append([]byte{1}, msg...)
	return verifyMultiSignature(curve, aggsig, keys, msg2)
}

// KoskVerifyBatchMultiSignature checks that the set of aggregate signatures correctly proves
// that a set of messages has the correct associated pubkey.
// vulnerable against chosen key attack, if keys have not been authenticated
// This is faster than verifying each multisignature individually.
func KoskVerifyBatchMultiSignature(curve CurveSystem, aggsigs []Point, pubkeys [][]Point, msgs [][]byte) bool {
	aggsig := AggregateSignatures(aggsigs)
	keys := make([]Point, len(pubkeys), len(pubkeys))
	for i := 0; i < len(pubkeys); i++ {
		keys[i] = AggregateKeys(pubkeys[i])
	}
	return KoskVerifyAggregateSignature(curve, aggsig, keys, msgs)
}

// KoskVerifyMultiSignatureWithMultiplicity verifies a BLS multi signature where
// multiple copies of each signature may have been included in the aggregation
func KoskVerifyMultiSignatureWithMultiplicity(curve CurveSystem, aggsig Point, keys []Point,
	multiplicity []int64, msg []byte) bool {
	if multiplicity == nil {
		return KoskVerifyMultiSignature(curve, aggsig, keys, msg)
	} else if len(keys) != len(multiplicity) {
		return false
	}
	factors := make([]*big.Int, len(multiplicity))
	for i := 0; i < len(keys); i++ {
		factors[i] = big.NewInt(multiplicity[i])
	}
	scaledKeys := ScalePoints(keys, factors)
	return KoskVerifyMultiSignature(curve, aggsig, scaledKeys, msg)
}
