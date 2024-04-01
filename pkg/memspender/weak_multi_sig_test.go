package memspender_test

import (
	"context"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/darwayne/chain-grabber/internal/test/testhelpers"
	"github.com/darwayne/chain-grabber/pkg/addressgen"
	"github.com/darwayne/chain-grabber/pkg/keygen"
	"github.com/darwayne/chain-grabber/pkg/memspender"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMultiSigAddressGen(t *testing.T) {
	key1 := keygen.FromInt(1)
	key2 := keygen.FromInt(2)
	key3 := keygen.FromInt(3)
	require.NotNil(t, key1)
	require.NotNil(t, key2)
	require.NotNil(t, key3)

	store := testNetSecretStore(t)

	var pubKeys [][]byte
	for _, key := range []*btcec.PrivateKey{key1, key2, key3} {
		compressed := key.PubKey().SerializeCompressed()
		known, err := store.HasKnownCompressedKey(compressed)
		require.NoError(t, err)
		require.True(t, known)
		pubKeys = append(pubKeys, compressed)
	}

	gen := addressgen.NewTestNet()
	addr, err := gen.MultiSigScriptHash(2, 3, pubKeys...)
	require.NoError(t, err)
	t.Log("script address", addr.EncodeAddress())

	addr, err = gen.MultiSigWitnessHash(2, 3, pubKeys...)
	require.NoError(t, err)
	t.Log("witness address", addr.EncodeAddress())
}

func TestSpendMultiSig(t *testing.T) {
	spendFromTx := testhelpers.TxFromHex(t, "020000000001015982b2497751836ac805fe96d1208a8e505f7aca66567bf7bd64de3263ebaa1f0100000000fdffffff02f03600000000000022002012c2ffbc6ec1cf5d746dfbd49b1063356212ea55f43023ffc0145934af20c5727981000000000000160014e3a265503db8c15654ab51a6bec12461330b735002473044022010140b5f75e21d055bedcf53dab534b28f60a7e211772806bbb58bd5b59df85802202b314ff25668604f102d8ed64e3a6187407ffaf852727c2fbe053e3f9dada90e0121021eae0b45956be67b0a728b11bdc5e9cc3c6896effc0a77b66e840413b76022bc78702700")

	addr, err := btcutil.DecodeAddress("mijXqYcsZnU6RqfaYML4ajvkKngJTuc6xV", &chaincfg.TestNet3Params)
	require.NoError(t, err)
	signer := testNetSecretStore(t)

	key1 := keygen.FromInt(1)
	key2 := keygen.FromInt(2)
	key3 := keygen.FromInt(3)
	require.NotNil(t, key1)
	require.NotNil(t, key2)
	require.NotNil(t, key3)

	store := testNetSecretStore(t)

	var pubKeys [][]byte
	for _, key := range []*btcec.PrivateKey{key1, key2, key3} {
		compressed := key.PubKey().SerializeCompressed()
		known, err := store.HasKnownCompressedKey(compressed)
		require.NoError(t, err)
		require.True(t, known)
		pubKeys = append(pubKeys, compressed)
	}

	gen := addressgen.NewTestNet()
	script, err := gen.MultiSigScript(2, 3, pubKeys...)
	require.NoError(t, err)

	tx := memspender.SpendMultiSigTx(spendFromTx, addr, 5, signer, [][]byte{script}, 0)
	require.NotNil(t, tx)

	var originalValue int64
	var sentValue int64

	for idx, out := range spendFromTx.TxOut {
		_ = idx
		originalValue += out.Value
	}

	for _, out := range tx.TxOut {
		sentValue += out.Value
	}

	t.Log("prev outpoint is", tx.TxIn[0].PreviousOutPoint)

	t.Log(txscript.DisasmString(tx.TxIn[0].SignatureScript))

	t.Log("original value", btcutil.Amount(originalValue))
	t.Log("sent value", btcutil.Amount(sentValue))

	/*
		err:
		unexpected status code: 400
		        	            	body:sendrawtransaction RPC error: {"code":-26,"message":"mandatory-script-verify-flag-failed (Non-canonical DER signature)"}
	*/

	encoded := txhelper.ToString(tx)
	t.Log("encoded tx is", encoded)

	if 1 == 1 {
		//return
	}
	err = mempoolspace.NewRest(mempoolspace.WithNetwork(&chaincfg.TestNet3Params)).WithDebugging().WithTrace().
		BroadcastHex(context.Background(), encoded)
	require.NoError(t, err)

}

//func TestMultiSigning(t *testing.T) {
//	tx := testhelpers.TxFromHex(t, "020000000128371083445e998b9000a0e147c18da1885ca87597a643e6e7af9e87e0f83fa201000000fc0047304402202f5695bf4c26838aca8591cd576204dd9bb48a2ad8dbdf8f5b3feb726f398ca702207e1dc513797f780b4265654a68deeb0cc5a5b63dd14545ef0b7be2c0a16e42bd014730440220346cbf9f754e0460dc10170510402fc2bebc73231f752db74e772732897556bd02205bb62dcfb5d1f9ab194d49db0f3f9606b63ef07eacfd3cc0db9d11abc67d5faa014c6952210279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f817982102c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee52102f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f953aeffffffff01a2030000000000001976a9145e9458cd40e1b53688d623aacc074b6c28558d1188ac00000000")
//	verifier := testhelpers.TxFromHex(t, "020000000128371083445e998b9000a0e147c18da1885ca87597a643e6e7af9e87e0f83fa201000000fc0047304402202f5695bf4c26838aca8591cd576204dd9bb48a2ad8dbdf8f5b3feb726f398ca702207e1dc513797f780b4265654a68deeb0cc5a5b63dd14545ef0b7be2c0a16e42bd014730440220346cbf9f754e0460dc10170510402fc2bebc73231f752db74e772732897556bd02205bb62dcfb5d1f9ab194d49db0f3f9606b63ef07eacfd3cc0db9d11abc67d5faa014c6952210279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f817982102c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee52102f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f953aeffffffff01a2030000000000001976a9145e9458cd40e1b53688d623aacc074b6c28558d1188ac00000000")
//
//	_, _ = tx, verifier
//	t.Log(txscript.DisasmString(verifier.TxIn[0].SignatureScript))
//
//	sendTo, err := bt
//
//	txscript.RawTxInSignature()
//}
