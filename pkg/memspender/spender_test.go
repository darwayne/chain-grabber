package memspender_test

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"github.com/darwayne/chain-grabber/pkg/memspender"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
	"github.com/darwayne/chain-grabber/pkg/txmonitor"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"hash"
	"os"
	"sync"
	"testing"
	"time"
)

const addressToSendTo = "tb1qx844m2dt7sxgj72e9gq945yhs4falldvyvl2dn"

func TestSpenderOnTestNet(t *testing.T) {
	t.Setenv("PROXY_USER", "")
	testNetwork(t, false)
}

func TestSpenderOnMainNet(t *testing.T) {
	t.Setenv("PROXY_USER", "")
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
	outPoint, err := wire.NewOutPointFromString("8d0cb50eec9ae9310da38f5139668353f3563f233fd62f6e34e7ab43fdad253d:1")
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

	addressToUse := addressToSendTo
	if isMainNet {
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

	go publisher.Connect(ctx)
	go func() {
		l.Info("generating keys")
		err := spender.GenerateKeys([2]int{0, 20})
		if err != nil {
			l.Warn("error generating keys", zap.String("err", err.Error()))
			return
		}

		l.Info("key generation complete!")

		go spender.Start(ctx)

		go n.ConnectV2()
		go m.Start(ctx, n)

		// give enough time to get fee

		//time.Sleep(5 * time.Second)
		//l.Info("attempting to spend")
		//err = spender.SpendAddress(context.Background(), "mkTG2b1eE2EJDeKRDqZtPgasECc9dJCVdG")
		//l.Info("spend attempt complete")
		//l.Error("error?", zap.Error(err))
	}()

	select {
	case <-ctx.Done():
		return
	case <-n.Done():
		return
	case <-sigutil.Done():
		return
	}
}

func getPublisher(t *testing.T, isTestNet bool, logger *zap.Logger) *broadcaster.Broker[*string] {
	t.Helper()
	broker := broadcaster.NewBroker[*string]()
	go broker.Start()
	addClientsToBroker(t, broker, isTestNet, logger)

	return broker
}

func addClientsToBroker(t *testing.T, broker *broadcaster.Broker[*string], isTestNet bool, logger *zap.Logger) {
	for _, c := range electrumClients(t, isTestNet, logger) {
		cli := c
		server := c.Server
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			sub := broker.Subscribe()
			for {
				select {
				case msg := <-sub:
					res, err := cli.Broadcast(*msg)
					if err != nil {
						logger.Warn("error broadcasting to node", zap.Error(err), zap.String("server", server))
					} else {
						logger.Info("successfully broadcasted", zap.String("txid", res),
							zap.String("server", server))
					}
				case <-ticker.C:
					_, err := cli.Ping()
					if err != nil {
						logger.Warn("error pinging reconnecting", zap.String("server", server))
						cli.Reconnect()
					} else {
						//logger.Info("ping success", zap.String("server", server))
					}

				}
			}
		}()
	}
}

func electrumClients(t *testing.T, isTestnet bool, logger *zap.Logger) []*broadcaster.ElectrumClient {
	var addresses []string
	if isTestnet {
		addresses = append(addresses, knownTestNetElectrumNodes...)
	} else {
		addresses = append(addresses, knownMainNetElectrumNodes...)
	}

	var mu sync.RWMutex
	var clients []*broadcaster.ElectrumClient
	var group errgroup.Group

	for _, address := range addresses {
		server := address
		group.Go(func() error {
			cli := broadcaster.NewElectrumClient()
			if err := cli.Connect(server); err != nil {
				logger.Warn("error connecting", zap.Error(err),
					zap.String("server", server))
				return nil
				//return err
			}

			mu.Lock()
			clients = append(clients, cli)
			mu.Unlock()

			return nil
		})
	}

	err := group.Wait()
	require.NoError(t, err)

	return clients
}

var knownTestNetElectrumNodes = []string{
	"testnet.qtornado.com:51002",
	"v22019051929289916.bestsrv.de:50002",
	//"testnet.hsmiths.com:53012",
	"blockstream.info:993",
	"electrum.blockstream.info:60002",
	"testnet.aranguren.org:51002",
}

var knownMainNetElectrumNodes = []string{
	"xtrum.com:50002",
	"blockstream.info:700",
	"electrum.bitaroo.net:50002",
	"electrum0.snel.it:50002",
	"btc.electroncash.dk:60002",
	"e.keff.org:50002",
	//"2electrumx.hopto.me:56022",
	"blkhub.net:50002",
	"bolt.schulzemic.net:50002",
	//"vmd71287.contaboserver.net:50002",
	"smmalis37.ddns.net:50002",
	"electrumx.alexridevski.net:50002",
	//"f.keff.org:50002",
	"mainnet.foundationdevices.com:50002",
	//"assuredly.not.fyi:50002",
	//"vmd104014.contaboserver.net:50002",
	//"ex05.axalgo.com:50002",
	//"exs.ignorelist.com:50002",
	"eai.coincited.net:50002",
	//"2ex.digitaleveryware.com:50002",
}
