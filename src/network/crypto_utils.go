package network

import (
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
)

func NewKey() *ecdsa.PrivateKey {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic("couldn't generate key: " + err.Error())
	}
	return key
}


func PublicKeyToId(pubKey *ecdsa.PublicKey) []byte {
	buf := make([]byte, 64)
	math.ReadBits(pubKey.X, buf[:32])
	math.ReadBits(pubKey.Y, buf[32:])
	return crypto.Keccak256(buf)
}
