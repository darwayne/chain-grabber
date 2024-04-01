package addressgen

import (
	"crypto/sha256"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"hash"
)

func NewTestNet() *Generator {
	return &Generator{params: &chaincfg.TestNet3Params}
}

func NewMainNet() *Generator {
	return &Generator{params: &chaincfg.MainNetParams}
}

type Generator struct {
	params *chaincfg.Params
}

func (g *Generator) MultiSigScriptHash(n int, m int, serializedKeys ...[]byte) (btcutil.Address, error) {
	script, err := g.MultiSigScript(n, m, serializedKeys...)
	if err != nil {
		return nil, err
	}

	return btcutil.NewAddressScriptHash(script, g.params)
}
func (g *Generator) MultiSigWitnessHash(n int, m int, serializedKeys ...[]byte) (btcutil.Address, error) {
	script, err := g.MultiSigScript(n, m, serializedKeys...)
	if err != nil {
		return nil, err
	}

	script = calcHash(script, sha256.New())

	return btcutil.NewAddressWitnessScriptHash(script, g.params)
}

func (g *Generator) MultiSigScript(n int, m int, serializedKeys ...[]byte) ([]byte, error) {
	builder := txscript.NewScriptBuilder()
	builder.AddOp(base + byte(n))
	for _, key := range serializedKeys {
		builder.AddData(key)
	}
	builder.AddOp(base + byte(m)).AddOp(txscript.OP_CHECKMULTISIG)

	return builder.Script()
}

const base = txscript.OP_1 - 1

// Calculate the hash of hasher over buf.
func calcHash(buf []byte, hasher hash.Hash) []byte {
	_, _ = hasher.Write(buf)
	return hasher.Sum(nil)
}
