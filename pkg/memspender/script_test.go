package memspender

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/darwayne/chain-grabber/internal/test/testhelpers"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"testing"
	"time"
)

func TestDisam(t *testing.T) {

	strings := []string{
		"160014588c223357e7a9dd5aa08079c99832b3126dc4a6",
	}

	for _, str := range strings {
		data, _ := hex.DecodeString(str)
		t.Log(txscript.DisasmString(data))

		tok := txscript.MakeScriptTokenizer(0, data)
		i := 1
		for tok.Next() {
			t.Log("opcode", i, "is", tok.Opcode(), "with len", len(tok.Data()),
				"with data\t", tok.Data())
			i++
			_ = txscript.OP_RETURN
		}

	}

}

func TestTx2(t *testing.T) {
	tx := testhelpers.TxFromHex(t, "020000000001017a86306745a6ed7a8782eeca70e424864442077341a16378f6619887c1f020930000000000ffffffff019403000000000000160014be4db0e4c1855a201dcfd532a02eb153aa563fa701015100000000")
	t.Log()
	_ = tx
}

func TestClassify(t *testing.T) {
	yo := &Spender{
		cfg:     &chaincfg.TestNet3Params,
		logger:  zap.NewNop(),
		txCache: expirable.NewLRU[chainhash.Hash, TxInfo](5_000, nil, 5*time.Minute),
	}
	yo.cli = mempoolspace.NewRest(mempoolspace.WithNetwork(yo.cfg))

	tests := []struct {
		hex    string
		expect TxClassification
	}{
		{
			hex:    "01000000000101b7c967844d243b24c03e3068cc0cf105febaa67c4f7dff25b93c7250bc1910530100000000fdffffff014f4d0000000000001600140608b7548258c4f00da6265263a848d2c233cc4001015100000000",
			expect: ReplayableSimpleInput,
		},
		{
			hex:    "0200000000010125c0605da7bc34130d44628f92e842ccc6526294f6135513112d86524e4eeaa30000000000ffffffff019adf03000000000017a91445dbde5afb565822339a0ef32bafc4ca08aaed48870248304502210098963e3180ffb1a75129c0a014438057a1809534e7b5204db94334e648bb2ed7022033e012cf574a7cf2088fd3b92020b427e1e89af863ba371ab1427a02426e6cb80121024acfc98cd315e3c3d7d77ddd9621af9959bbf22450272c09935dd6edbb5cb2f000000000",
			expect: UnSpendable,
		},
		{
			hex:    "02000000000101cef40b9c1616aebc9406db94d64469a765d7aeb3da5fc4f1dd9bf6bafccd6b72610000001716001466f8526861b24574089d7ee7e4dad2df680cc04affffffff01fa5f2c00000000001976a914df3db86eddb60cad903e2d88dbd3d34fc1ca3d7e88ac02483045022100c3ecc7fb9b4060abf39827c0fca3f64455817ed1a745fda42140f871c670805902207a15d910ed16eb851a205f95d98051de4d59a0a5ed78fe9982f61deb3ac86e740121026cc61db4d54848c2ee6770cbe1d12808f86169aac8a6f66ee9a9b2163d3e60e900000000",
			expect: UnSpendable,
		},
		{
			hex:    "01000000000101eb12f8f2abf50d07506f26e082a6f3b3c92010714be477db8171d3ecb60141e50000000000ffffffff0122020000000000002200209e9fa1eed8508b057fd425642a490b399ab34a8de3e1e8cdad33e28428ef9d6e0340fe5a7a08c5a62c5561426b571013b7b2379ab6040d6e5559e200e0322e05c6404a86a14ad1e589d6302558e894a255c80b37ebcc024c420e83d00b8d24e5c0c78820fc4c1d5570c9448733dd875ae0f594833e343cd81eccf4d89c065b4316bfb55aac0063036f7264010118746578742f706c61696e3b636861727365743d7574662d3800427b2270223a226272632d3230222c226f70223a227472616e73666572222c227469636b223a2273617473222c22616d74223a223130393835323235312e323539227d6821c1fc4c1d5570c9448733dd875ae0f594833e343cd81eccf4d89c065b4316bfb55a00000000",
			expect: UnSpendable,
		},
		{
			hex:    "01000000018c3f4190b93b32f8a020649410b7611c5682063f87e0fd2edf148696f8f3916f010000006a4730440220765c174c0a61ec6fae91dc52cae0782ddf640bc3141d8e38be107a8969357abb0220362e3d7cd5a240d922d4fbcd421292737a1b241f116793edc6705d408c7b585901210338b318ed588189cde4cf728b780d5e8463d37b4914bd514c86aeb1ed02a775ffffffffff013c2001000000000016001463ea4093e7d6ae2abad60133a45bf7fdfaabed2900000000",
			expect: UnSpendable,
		},
		{
			hex:    "0100000001753a3464129bc037ea677505ddfa68e208e1356bbcdd81b1598eac87a48cec23c00000006a47304402202e61a8cdab4680021d8d7df9eb92fbc9e2a5a608ef3428cd6a1c3674417742560220173714e10f078f4ce9719ba1c32c156b5fae0a27fbbf4fb3d2b4c9f21e0811a301210235e5921aee4e906dac67f4f9a552b3b4cd09dc2ca77ebf11ef8b77551241a066ffffffff0129100100000000001976a914fefba28673e6e4bd721200c0bdb4cc371a57c3b788ac00000000",
			expect: UnSpendable,
		},
		{
			hex:    "01000000000101c6e3919d8bb5f6ef46749797bb73c160f2bf39f90df9ae5c90a00754a98067790100000000ffffffff018e0300000000000016001431eb5da9abf40c8979592a005ad0978553dffdac01403a187a0809b351bef6f0924d6c2c3bb8b89254a06bbc840eee3249d0bb62ef82be96a3de93a8ee613fd0c098a08f1a42859727221e13cc54bb88ff0399bb709e00000000",
			expect: UnSpendable,
		},
	}

	for idx, tt := range tests {
		t.Run(fmt.Sprintf("idx: %d", idx), func(t *testing.T) {
			tx := testhelpers.TxFromHex(t, tt.hex)
			t.Log("txid", tx.TxHash().String())
			result := yo.classifyTx(tx)

			require.Equal(t, tt.expect, result)
		})
	}

}

func TestIPCheck(t *testing.T) {
	//t.Setenv("PROXY_USER", "")
	yo := mempoolspace.NewRest()

	for i := 0; i < 5; i++ {
		t.Log(time.Now().Format(time.RFC3339))
		t.Log(yo.GetIPAddress(context.Background()))
	}

}

func TestHmm(t *testing.T) {
	tx := testhelpers.TxFromHex(t, "0100000001fe75a438b72fdc302b80cc216d66d5e3bbb0359bce3bb4cecf743f5fda1f4eb101000000fdfd000048304502210096b617a5b2bd676ee8d3f8d8d91bf60c599e16382d1e12a61a1f9562c35b2cb102204379706a55c07bb45d20336159f80ebe9786938e34b9309e49ed422e6d2a44470147304402201550a8bb0c28107098289fe6fe64488bdee46800d28bfbb0b0a1e1b2d64b9fb4022004684015095b999185b3da1a23d239452ad73b199a032f71978760f8ae42313f014c6952210265e6f7fb614a369c9230912a3bb09c33c5c5be2e1bcfc2293ecaed46708e0b5c2103f546edf7b434b50aa0115c1c82a0f9a96505d9eff55d2fe3b848c4b51c06b6432102908375f301c7ea583f7e113939eab1164abda4ac27898b1cf78abf1c82f02da953aeffffffff01f8a70000000000001976a914bd63bf79e39f4cd52361c092c3fba9264662285688ac00000000")

	info := txscript.MakeScriptTokenizer(0, tx.TxIn[0].SignatureScript)

	for info.Next() {
		t.Log(info.Opcode(), info.Data())
		_ = txscript.OP_RETURN
	}

	t.Log("REDEEM BELOW!")

	rawRedeem := []byte{82, 33, 2, 101, 230, 247, 251, 97, 74, 54, 156, 146, 48, 145, 42, 59, 176, 156, 51, 197, 197, 190, 46, 27, 207, 194, 41, 62, 202, 237, 70, 112, 142, 11, 92, 33, 3, 245, 70, 237, 247, 180, 52, 181, 10, 160, 17, 92, 28, 130, 160, 249, 169, 101, 5, 217, 239, 245, 93, 47, 227, 184, 72, 196, 181, 28, 6, 182, 67, 33, 2, 144, 131, 117, 243, 1, 199, 234, 88, 63, 126, 17, 57, 57, 234, 177, 22, 74, 189, 164, 172, 39, 137, 139, 28, 247, 138, 191, 28, 130, 240, 45, 169, 83, 174}

	info = txscript.MakeScriptTokenizer(0, rawRedeem)
	for info.Next() {
		t.Log(info.Opcode(), info.Data())
		_ = txscript.OP_RETURN
	}

	t.Log("REDEEM ADDR:")

	t.Log(btcutil.NewAddressScriptHash(rawRedeem, &chaincfg.TestNet3Params))

	t.Log("REDEEM HASH:")
	t.Log(hex.EncodeToString(btcutil.Hash160(rawRedeem)))

	t.Log(txscript.IsPushOnlyScript(tx.TxIn[0].SignatureScript))

	//t.Log(tx.TxHash())

	t.Log(txscript.IsPushOnlyScript([]byte{1, 51}))

}

func TestLogic(t *testing.T) {
	tx := testhelpers.TxFromHex(t, "0100000001b6c197488d2463177e556af103e15faaa7250c02e7257cd5e44c397957086d2501000000020151fdffffff019c3900000000000016001431eb5da9abf40c8979592a005ad0978553dffdac00000000")
	require.True(t, len(tx.TxIn) == 1 && len(tx.TxIn[0].SignatureScript) > 0 && len(tx.TxOut) == 1)
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(tx.TxOut[0].PkScript, &chaincfg.TestNet3Params)
	require.NoError(t, err)
	result := isReplayable(tx.TxIn[0].SignatureScript) && err == nil &&
		len(addresses) == 1
	require.True(t, result)

	result = result && !(addresses[0].String() == "tb1qx844m2dt7sxgj72e9gq945yhs4falldvyvl2dn" || addresses[0].EncodeAddress() == "tb1qx844m2dt7sxgj72e9gq945yhs4falldvyvl2dn")

	require.False(t, result)
}

func TestScriptDetector(t *testing.T) {
	tx := testhelpers.TxFromHex(t, "0100000001fe75a438b72fdc302b80cc216d66d5e3bbb0359bce3bb4cecf743f5fda1f4eb101000000fdfd000048304502210096b617a5b2bd676ee8d3f8d8d91bf60c599e16382d1e12a61a1f9562c35b2cb102204379706a55c07bb45d20336159f80ebe9786938e34b9309e49ed422e6d2a44470147304402201550a8bb0c28107098289fe6fe64488bdee46800d28bfbb0b0a1e1b2d64b9fb4022004684015095b999185b3da1a23d239452ad73b199a032f71978760f8ae42313f014c6952210265e6f7fb614a369c9230912a3bb09c33c5c5be2e1bcfc2293ecaed46708e0b5c2103f546edf7b434b50aa0115c1c82a0f9a96505d9eff55d2fe3b848c4b51c06b6432102908375f301c7ea583f7e113939eab1164abda4ac27898b1cf78abf1c82f02da953aeffffffff01f8a70000000000001976a914bd63bf79e39f4cd52361c092c3fba9264662285688ac00000000")

	script := tx.TxIn[0].SignatureScript
	script, _ = hex.DecodeString("0151") //[]byte{0x01, 0x51}
	tok := txscript.MakeScriptTokenizer(0, script)

	var redeemScript []byte
	for tok.Next() {
		redeemScript = tok.Data()
	}
	_ = txscript.OP_TRUE

	tok = txscript.MakeScriptTokenizer(0, redeemScript)
	for tok.Next() {
		switch tok.Opcode() {
		case txscript.OP_CHECKSIG, txscript.OP_CHECKMULTISIG, txscript.OP_CHECKMULTISIGVERIFY,
			txscript.OP_CHECKSIGVERIFY, txscript.OP_CHECKSIGADD:

		}
	}

	t.Log("redeem script is")
	t.Log(txscript.DisasmString(redeemScript))
}

func TestDisasm(t *testing.T) {
	raw, err := hex.DecodeString("00483045022100890c330acd28957301cdf63d8f6f87275d25d473b15903b64dc694f7065ed4cf0220263a7fff456b5a1e2445a127991aee1d103dbee3fe55e713860af5afd854bc44014830450221008cb0a70910211f49919442f82650720cfa0d9905766813bf7ebf7e3846921c7802207cc79b7d90794aa917b3a381b5bede0ca0a4aca9ccce2b226e4ab4d34d6f433d014c6952210265e6f7fb614a369c9230912a3bb09c33c5c5be2e1bcfc2293ecaed46708e0b5c2103f546edf7b434b50aa0115c1c82a0f9a96505d9eff55d2fe3b848c4b51c06b6432102908375f301c7ea583f7e113939eab1164abda4ac27898b1cf78abf1c82f02da953ae")
	require.NoError(t, err)
	t.Log(txscript.DisasmString(raw))
}

func TestScriptEvaluation(t *testing.T) {
	fundingTx := testhelpers.TxFromHex(t, "02000000000101947b5dac93a6f051f653ee1feee6cfd9570bc2c3943c8eba2ec029994248b9490100000000fdffffff02123d00000000000017a914da1745e9b549bd0bfa1a569971c77eba30cd5a4b879054000000000000160014f35690697d71036bff6be1203a850811a152db7a02473044022006223c810db30024856ef71c732cd1bf70138047ef7ea7a23c163896ddd9d88202200390e41a3cd9148ada944af05ae975387bd2e721a8b9e9ab49969397907a63440121035fa817c1eca6f6f17f1163ef19b230acb3219d7f383102419576e4b248037819a5682700")
	spendingTx := testhelpers.TxFromHex(t, "01000000016dd6b918ec8e16ce93fd8c0b87b6921ef6701b3dd480cf710cb95b05428f86fe00000000020151ffffffff01a83c0000000000001600140608b7548258c4f00da6265263a848d2c233cc4000000000")
	_, _ = fundingTx, spendingTx

	tok := txscript.MakeScriptTokenizer(0, fundingTx.TxOut[0].PkScript)

	idx := 1
	for tok.Next() {
		fmt.Printf("op(%d) is %d\n", idx, tok.Opcode())
		idx++
	}

	// TODO: manually set this up
	fetcher := txscript.NewMultiPrevOutFetcher(map[wire.OutPoint]*wire.TxOut{
		fundingTx.TxIn[0].PreviousOutPoint: spendingTx.TxOut[0],
	})

	script, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_DATA_1).
		AddOp(txscript.OP_TRUE).Script()
	require.NoError(t, err)

	//hashCache := txscript.NewTxSigHashes(spendingTx, fetcher)
	vm, err := txscript.NewEngine(script, spendingTx,
		0, txscript.StandardVerifyFlags, nil, nil,
		spendingTx.TxOut[0].Value, fetcher)

	require.NoError(t, err)
	t.Log(vm.Execute())
}

func TestFee(t *testing.T) {
	val, err := mempoolspace.NewRest().GetFee(context.Background())
	require.NoError(t, err)
	require.NotNil(t, val)
	t.Logf("%+v", *val)
}