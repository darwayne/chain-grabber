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
	key1 := keygen.FromInt(912)
	key2 := keygen.FromInt(200000)
	key3 := keygen.FromInt(300123)
	require.NotNil(t, key1)
	require.NotNil(t, key2)
	require.NotNil(t, key3)

	store := testNetSecretStore(t)

	var pubKeys [][]byte
	for idx, key := range []*btcec.PrivateKey{key1, key2, key3} {
		compressed := key.PubKey().SerializeCompressed()
		known, err := store.HasKnownCompressedKey(compressed)
		require.NoError(t, err)
		require.True(t, known, idx)
		pubKeys = append(pubKeys, compressed)
	}

	gen := addressgen.NewTestNet()
	addr, err := gen.MultiSigScriptHash(0, 3, pubKeys...)
	require.NoError(t, err)
	t.Log("script address", addr.EncodeAddress())

	addr, err = gen.MultiSigWitnessHash(2, 3, pubKeys...)
	require.NoError(t, err)
	t.Log("witness address", addr.EncodeAddress())
}

func TestSpendMultiSig(t *testing.T) {
	spendFromTx := testhelpers.TxFromHex(t, "020000000001010b7fe20f16998ffb02ee9a1d2cc3c4efc7c4274d7b6d18cbeea79fd73e18ac0c0100000000fdffffff02f29000000000000017a91415fc0754e73eb85d1cbce08786fadb7320ecb8dc8754c23402000000001600141ce4c1a202ebbfe1a2c64c853ce99f2af49085020247304402205b26ce13f4e07075fa0661ac27c05308198bdf37eb19fbc60908485c9aa3d91c022028c2288772359f1cfd9a125658f5083ad4134284e1cbae60f122e37ef37d0b850121020419122b06beadf57dfafce754111ed9bc7334b82d127c78e5968c694611c79004732700")

	addr, err := btcutil.DecodeAddress("2MuFU6ZyBLtDNadMA6RnwJdXGWUSUaoKLeS", &chaincfg.TestNet3Params)
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

	tx := memspender.SpendMultiSigTx(spendFromTx, addr, 2, signer, [][]byte{script}, 0)
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

func TestSpendMultiSigRaw(t *testing.T) {
	spendFromTx := testhelpers.TxFromHex(t, "0200000000010118ea78e35577b5d30642798cd7d53e63b3105a590709fc0dc7dd911172d2976f0100000000fdffffff022d9000000000000017a91415fc0754e73eb85d1cbce08786fadb7320ecb8dc8799313402000000001600146a56bc7b145bcf60be59d3bf016d5e3c6d29e98902473044022020aa82ee0a569b49dee602770fd0679a7906d36aec33682d8abec29457e999d30220611d8d770d4d571ee23f373a6d0b1fb1000c6fc7911e239da3f14f1484f91eb10121039bb1ad47054d2d2989a40a68e3f51f54880461d33904e14ff8820b0695d9dcd707732700")

	addr, err := btcutil.DecodeAddress("2MuFU6ZyBLtDNadMA6RnwJdXGWUSUaoKLeS", &chaincfg.TestNet3Params)
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

	tx := memspender.SpendMultiSigTx(spendFromTx, addr, 10, signer, [][]byte{script}, 0)
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
