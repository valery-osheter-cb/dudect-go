package dudect

import (
	"crypto/rand"
	"math/big"
	"testing"
)
import "github.com/decred/dcrd/dcrec/secp256k1/v4"

func TestDudect(t *testing.T) {

	p := new(big.Int)
	p.SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	const iterations = 100000
	d := NewDudectContext(iterations)

	var a [iterations]secp256k1.FieldVal64
	var z [iterations]secp256k1.FieldVal64 // zeroes

	for {
		var bytes [32]byte
		for i := 0; i < iterations; i++ {
			x, _ := rand.Int(rand.Reader, p)
			x.FillBytes(bytes[:])
			a[i].SetBytes(&bytes)
		}

		state := d.Round(
			func(index int) { a[index].Square() },
			func(index int) { z[index].Square() })

		if state == LeakageFound {
			break
		}
	}
}
