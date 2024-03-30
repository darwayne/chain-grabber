package memspender_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/lightnode"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/darwayne/chain-grabber/internal/test/testhelpers"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/darwayne/chain-grabber/pkg/keygen"
	"github.com/darwayne/chain-grabber/pkg/memspender"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
	"github.com/darwayne/chain-grabber/pkg/txmonitor"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"hash"
	"os"
	"testing"
	"time"
)

const addressToSendTo = "tb1qx844m2dt7sxgj72e9gq945yhs4falldvyvl2dn"

func TestSpenderOnTestNet(t *testing.T) {
	//t.Setenv("PROXY_USER", "")
	testNetwork(t, false)
}

func TestSpenderOnMainNet(t *testing.T) {
	//t.Setenv("PROXY_USER", "")
	testNetwork(t, true)
}

func TestRawScriptDecoder(t *testing.T) {
	script, err := hex.DecodeString("16001475c6f01e9b9d75dd19d0d915d5073fc28cc0367e")
	require.NoError(t, err)

	tok := txscript.MakeScriptTokenizer(0, script)
	var spendTable bool
	var lastOp byte
	var firstOp byte
	for tok.Next() {
		opCode := tok.Opcode()
		lastOp = opCode
		data := tok.Data()
		_ = data
		if !spendTable && (opCode == txscript.OP_TRUE ||
			(opCode >= txscript.OP_DATA_1 && opCode <= txscript.OP_DATA_75)) {
			spendTable = true
			continue
		}

		if spendTable && opCode == txscript.OP_RETURN {
			spendTable = false
			break
		}
	}

	if lastOp == txscript.OP_RETURN || firstOp == txscript.OP_FALSE {
		spendTable = false
	}

	require.False(t, spendTable)

}

func TestOpTrueScript(t *testing.T) {
	anyoneCanSpendScript, err := txscript.NewScriptBuilder().
		AddData([]byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")).
		AddOp(txscript.OP_TRUE).Script()
	require.NoError(t, err)

	//for _, op := range anyoneCanSpendScript {
	//	if op == txscript.OP_TRUE {
	//		t.Log("OH SNAP")
	//	}
	//}

	t.Log(anyoneCanSpendScript)

	tok := txscript.MakeScriptTokenizer(0, anyoneCanSpendScript)

	for tok.Next() {
		if tok.Opcode() == txscript.OP_TRUE {
			t.Log("alrighty then")
		}
	}

	t.Log(txscript.NewScriptBuilder().AddData([]byte("0123456789abcdef0123456789abcdef01234567")).Script())
}

// Calculate the hash of hasher over buf.
func calcHash(buf []byte, hasher hash.Hash) []byte {
	_, _ = hasher.Write(buf)
	return hasher.Sum(nil)
}

func TestBadScript(t *testing.T) {

}

func TestSimpleScript(t *testing.T) {
	hexVal := fmt.Sprintf("%x", "hello world")
	anyoneCanSpendScript, err := txscript.NewScriptBuilder().
		AddData([]byte(hexVal)).
		AddData([]byte(hexVal)).
		//AddOp(txscript.OP_EQUAL).
		AddOp(txscript.OP_EQUAL).
		Script()
	require.NoError(t, err)

	// Hash the script to obtain the script hash
	scriptHash := calcHash(anyoneCanSpendScript, sha256.New())

	var addr btcutil.Address
	addr, err = btcutil.NewAddressScriptHash(anyoneCanSpendScript, &chaincfg.TestNet3Params)
	require.NoError(t, err)

	t.Log("testnet script address", addr.String())

	addr, err = btcutil.NewAddressWitnessScriptHash(scriptHash, &chaincfg.TestNet3Params)
	require.NoError(t, err)

	t.Log("testnet witness address", addr.String())

	addr, err = btcutil.NewAddressScriptHash(anyoneCanSpendScript, &chaincfg.MainNetParams)
	require.NoError(t, err)

	t.Log("mainNet script address", addr.String())

	addr, err = btcutil.NewAddressWitnessScriptHash(scriptHash, &chaincfg.MainNetParams)
	require.NoError(t, err)

	t.Log("mainNet taproot address", addr.String())
}

func TestSpendSimpleScript(t *testing.T) {
	cli := mempoolspace.NewRest(mempoolspace.WithNetwork(&chaincfg.TestNet3Params))
	tx := wire.NewMsgTx(wire.TxVersion)
	outPoint, err := wire.NewOutPointFromString("3358506c52451e0deb5098c4797dadb1401f36e23001f5f7cfb76674baa0fffb:0")
	require.NoError(t, err)

	isWitness := false

	hexVal := fmt.Sprintf("%x", "i wonder who satoshi is")
	anyoneCanSpendScript, err := txscript.NewScriptBuilder().
		AddData([]byte(hexVal)).
		AddData([]byte(hexVal)).
		//AddOp(txscript.OP_EQUAL).
		AddOp(txscript.OP_EQUAL).
		Script()
	require.NoError(t, err)
	_ = anyoneCanSpendScript

	scriptToUse := anyoneCanSpendScript

	if !isWitness {
		scriptToUse, err = txscript.NewScriptBuilder().
			AddData(anyoneCanSpendScript).Script()
		require.NoError(t, err)
	}

	in := wire.NewTxIn(outPoint, scriptToUse, nil)
	if isWitness {
		in = wire.NewTxIn(outPoint, nil, [][]byte{scriptToUse})
	}
	in.Sequence = 0xffffffff - 2
	tx.AddTxIn(in)
	// use a different address to see if grabber will grab
	const testAddress = "tb1qqcytw4yztrz0qrdxyefx82zg6tpr8nzqxncw5p"
	addr, err := btcutil.DecodeAddress(testAddress, &chaincfg.TestNet3Params)
	require.NoError(t, err)
	receiverScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)
	info, err := cli.GetTransaction(context.Background(), outPoint.Hash)
	require.NoError(t, err)
	val := info.TxOut[int(outPoint.Index)].Value
	tx.AddTxOut(wire.NewTxOut(val-(int64(txhelper.VBytes(tx)*4)), receiverScript))

	//logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	weight := blockchain.GetTransactionWeight(btcutil.NewTx(tx))

	// Convert weight to vbytes
	vbytes := weight / blockchain.WitnessScaleFactor

	// Print the total vbytes
	fmt.Printf("Total vbytes: %d\n", vbytes)
	fmt.Println("weight", weight)

	fmt.Println()

	//if 1 == 1 {
	//	return
	//}

	var buff bytes.Buffer
	writer := hex.NewEncoder(&buff)
	err = tx.Serialize(writer)
	require.NoError(t, err)
	str := buff.String()
	//pub := getPublisher(t, true, logger)
	//logger.Warn("woah there")
	//pub.Publish(&str)
	err = cli.
		BroadcastHex(context.Background(), str)

	fmt.Println("sending:\n===", str, "\n====")
	fmt.Println("err", err)

	select {
	case <-time.After(30 * time.Second):
	case <-sigutil.Done():
	}

}

func TestPayToAnyoneAddress(t *testing.T) {
	//hexVal := fmt.Sprintf("%x", "hello")
	anyoneCanSpendScript, err := txscript.NewScriptBuilder().
		//AddData([]byte(hexVal)).
		//AddOp(txscript.OP_EQUAL).
		AddOp(txscript.OP_TRUE).
		Script()
	require.NoError(t, err)

	// Hash the script to obtain the script hash
	scriptHash := calcHash(anyoneCanSpendScript, sha256.New())

	var addr btcutil.Address
	addr, err = btcutil.NewAddressScriptHash(anyoneCanSpendScript, &chaincfg.TestNet3Params)
	require.NoError(t, err)

	t.Log("testnet script address", addr.String())

	addr, err = btcutil.NewAddressWitnessScriptHash(scriptHash, &chaincfg.TestNet3Params)
	require.NoError(t, err)

	t.Log("testnet witness address", addr.String())

	addr, err = btcutil.NewAddressScriptHash(anyoneCanSpendScript, &chaincfg.MainNetParams)
	require.NoError(t, err)

	t.Log("mainNet script address", addr.String())

	addr, err = btcutil.NewAddressWitnessScriptHash(scriptHash, &chaincfg.MainNetParams)
	require.NoError(t, err)

	t.Log("mainNet taproot address", addr.String())
}

func TestSatsPerVByte(t *testing.T) {
	tx := testhelpers.TxFromHex(t, "020000000263f43c1a23c0d76fbe5bfd27475a0ac787515cd95254bec3f6553a3358824f94010000006946304302205c7e08d6a4383de762fa7cd6146ae9397f9f0f14579971406c4ebbe67aa645a9021f0e457079165e1cce704b48dfa284dc5b9a209b9449d3efa18843d2611a4f42012103ca3cfe21f7c316ec1ff51aa91f765095a6f218ae0fbec2deda790cb6c20f751cfdffffff73a3a6e349ace8e35d28760b49357ea4736018409565f4b07540812c79fabff4000000006a47304402206a5f40eb4d767bd18a5b544fbbc2f853a3380d648ceb963dfea7838384a9645102202b0f685ffc1845fb3030326c91ffa05a8688fb804c96624c33ec20a75b342028012103ca3cfe21f7c316ec1ff51aa91f765095a6f218ae0fbec2deda790cb6c20f751cfdffffff02bc4c00000000000017a914da1745e9b549bd0bfa1a569971c77eba30cd5a4b87321d0300000000001976a9146a231411dfa9f88dd3f0e3e2ee3d68ef90c3930a88ac91682700")
	var inputValue int64 = 163200 + 61084
	t.Log(txhelper.SatsPerVByte(inputValue, tx))

}

func TestSpendScript(t *testing.T) {
	cli := mempoolspace.NewRest(mempoolspace.WithNetwork(&chaincfg.TestNet3Params))
	tx := wire.NewMsgTx(wire.TxVersion)
	outPoint, err := wire.NewOutPointFromString("9fc021f55392de91d4ca20ff709048b6638af9046cab2f1111f3b034264663a4:0")
	require.NoError(t, err)

	scriptSig, err := txscript.NewScriptBuilder().AddOp(txscript.OP_TRUE).Script()
	require.NoError(t, err)
	_ = scriptSig

	isWitness := false

	in := wire.NewTxIn(outPoint, []byte{
		txscript.OP_DATA_1, txscript.OP_TRUE,
	}, nil)
	if isWitness {
		in = wire.NewTxIn(outPoint, nil, [][]byte{{txscript.OP_TRUE}})
	}
	in.Sequence = 0xffffffff - 2
	tx.AddTxIn(in)
	// use a different address to see if grabber will grab
	const testAddress = "tb1qqcytw4yztrz0qrdxyefx82zg6tpr8nzqxncw5p"
	addr, err := btcutil.DecodeAddress(testAddress, &chaincfg.TestNet3Params)
	require.NoError(t, err)
	receiverScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)
	info, err := cli.GetTransaction(context.Background(), outPoint.Hash)
	require.NoError(t, err)
	val := info.TxOut[int(outPoint.Index)].Value
	tx.AddTxOut(wire.NewTxOut(val-(int64(txhelper.VBytes(tx)*4)), receiverScript))

	//logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	weight := blockchain.GetTransactionWeight(btcutil.NewTx(tx))

	// Convert weight to vbytes
	vbytes := weight / blockchain.WitnessScaleFactor

	// Print the total vbytes
	fmt.Printf("Total vbytes: %d\n", vbytes)
	fmt.Println("weight", weight)

	fmt.Println()

	//if 1 == 1 {
	//	return
	//}

	var buff bytes.Buffer
	writer := hex.NewEncoder(&buff)
	err = tx.Serialize(writer)
	require.NoError(t, err)
	str := buff.String()
	//pub := getPublisher(t, true, logger)
	//logger.Warn("woah there")
	//pub.Publish(&str)
	err = cli.
		BroadcastHex(context.Background(), str)

	fmt.Println("sending:\n===", str, "\n====")
	fmt.Println("err", err)

	select {
	case <-time.After(30 * time.Second):
	case <-sigutil.Done():
	}

}

func TestIt(t *testing.T) {
	rawHex := "020000000108089fe473652979417cc56a757ec2af439305ac9d0b3c55ca368aa216777d3f010000000201510000001001e803000000000000232102de153317307164e7c9918791c7787d9833a3a8201bdff880e631e490cf9a087cac00000000"
	tx := testhelpers.TxFromHex(t, rawHex)

	for idx, out := range tx.TxOut {
		class, addresses, numSigs, err := txscript.ExtractPkScriptAddrs(out.PkScript, &chaincfg.MainNetParams)
		require.NoError(t, err)

		fmt.Println("idx:", idx, class, addresses, numSigs, "\n====")

		_, _, _ = class, addresses, numSigs
	}

}

func testNetwork(t *testing.T, isMainNet bool) {
	m := txmonitor.New()

	params := &chaincfg.TestNet3Params
	addressToUse := addressToSendTo
	if isMainNet {
		params = &chaincfg.MainNetParams
		addressToUse = os.Getenv("MAIN_NET_ADDRESS")
		require.NotEmpty(t, addressToUse)
	}

	ctx := context.Background()

	l, err := zap.NewDevelopment()
	require.NoError(t, err)
	var n *lightnode.Node
	if isMainNet {
		n, err = lightnode.NewMainNet(l)
	} else {
		n, err = lightnode.NewTestNet(l)
	}

	require.NoError(t, err)

	publisher := broadcaster.New(!isMainNet, l)

	spender, err := memspender.New(m.Subscribe(), !isMainNet, publisher.Broker, addressToUse, l)
	require.NoError(t, err)

	db, err := sql.Open("sqlite3", "../keygen/testdata/sampledbs/keydbv2.sqlite")
	require.NoError(t, err)
	secretStore := keygen.NewSQLReader(db, params)

	spender.SetSecrets(secretStore)
	t.Cleanup(func() {
		secretStore.Close()
	})

	go publisher.Connect(ctx)
	go spender.Start(ctx)

	go n.ConnectV2()
	go m.Start(ctx, n)

	select {
	case <-ctx.Done():
		return
	case <-n.Done():
		return
	case <-sigutil.Done():
		return
	}
}
