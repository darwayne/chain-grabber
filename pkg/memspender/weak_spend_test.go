package memspender

import (
	"context"
	"crypto/sha256"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/lightnode"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/darwayne/chain-grabber/internal/test/testhelpers"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/darwayne/chain-grabber/pkg/keygen"
	"github.com/darwayne/chain-grabber/pkg/txmonitor"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"os"
	"testing"
)

func TestWeakSpend(t *testing.T) {
	m := txmonitor.New()
	isMainNet := false
	addressToUse := "tb1qqcytw4yztrz0qrdxyefx82zg6tpr8nzqxncw5p"

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
	cfg := &chaincfg.TestNet3Params

	_, _, _, _ = ctx, n, publisher, m
	start := 2_000_000_000
	end := start + 5
	gen, err := grabber.GenerateKeys(start, end, cfg)
	require.NoError(t, err)

	sec := grabber.NewMemorySecretStore(gen.AddressKeyMap, cfg)

	orginalTx := testhelpers.TxFromHex(t, "020000000001029dffb2e6ccd65b081a7220158d72d73124403b6e8973fe4f7276d9df4a3c7d690000000000fdffffff313ffb5db11c89194848cfa741eb292313e064cb7621636ca4610ead503e8d8a0000000000fdffffff0206370000000000001976a9142edee3d4a559a0b6972685e2974d7bdae9087f7488ace4c0030000000000160014b1f0c4b953a63e2b23a9fc061994a6e31cf198560247304402202e3368b713e18a28358ad14978ad8ad6a441b7ab50420141dda904322ffa4188022055520f5bc1c406987c5fcb1c344278ce033bd64d440bf0283accac3ce92a0f54012103c1f0b6a49a5d24e0b1381dded347f8c5d6d412bd630c7d0d5147ac74797fa419024730440220331181a4b4bfd3e2dca7fa8a454c7cf3bdb8145cd038c068687e7a217c6360de02201ef6c2321d1d779e39dd2be3763e8207e03d274a68a0ff557e69322db1d62051012103c1f0b6a49a5d24e0b1381dded347f8c5d6d412bd630c7d0d5147ac74797fa419636c2700")

	addr, err := btcutil.DecodeAddress(addressToUse, &chaincfg.TestNet3Params)
	require.NoError(t, err)
	receiverScript, err := txscript.PayToAddrScript(addr)

	_, _, _ = sec, orginalTx, receiverScript
}

func TestSegwitWeakKey(t *testing.T) {
	l, err := zap.NewDevelopment()
	require.NoError(t, err)

	addressToUse := "tb1qqcytw4yztrz0qrdxyefx82zg6tpr8nzqxncw5p"

	m := txmonitor.New()
	pub := broadcaster.NewBroker[*string]()
	data := pub.Subscribe()
	go pub.Start()

	cli := mempoolspace.NewRest(mempoolspace.WithNetwork(&chaincfg.TestNet3Params))

	spender, err := New(m.Subscribe(), true, pub, addressToUse, l, cli)
	require.NoError(t, err)

	secretStore, err := keygen.NewReaderFromDir("./testdata/sampledbs", true)
	require.NoError(t, err)
	spender.SetSecrets(secretStore)

	weakTx := testhelpers.TxFromHex(t, "02000000000101ac7e256964d21fe8d1bd20f80ec9bbf894e7eebae50aeb3f423eef7042b234220100000000fdffffff02b337000000000000160014076001132599b8cb8684983da30d9f0ac25a087ab9440000000000001600143a5c99484b29af9db75c85ed67380775ca1f60aa02473044022007a3c570e0b2e234d116f3f37b88cbe52650a14421216338abe1bb684d44cd8d02201462d8abead27673f37946d7bc674eb22180878607f2ecbdbdbb15bbb5f503960121032646e15301e8540cd0911414eb0c82db68d6af268609636226a476e032c1a04bad6c2700")
	spender.spendWeakKey(context.Background(), weakTx)
	info := <-data
	require.NotNil(t, info)

	crafted := testhelpers.TxFromHex(t, *info)
	in := crafted.TxIn[0]
	script := NewParsedScript(in.SignatureScript, in.Witness...)
	require.True(t, script.IsP2WPKH())
	t.Log(script.Ops)
	for idx, d := range script.Data {
		t.Logf("idx(%d): %x", idx, d)
		t.Logf("idx(%d): hashed %x", idx, btcutil.Hash160(calcHash(d, sha256.New())))
	}
}
