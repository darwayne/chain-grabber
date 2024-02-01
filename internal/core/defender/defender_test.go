package defender

import (
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/lightnode"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestTransactionInfo_Tx(t *testing.T) {
	txToDefend := "02000000000106c981fca14d129ab41405cc749578413f23c645148a3b1d1ffd6ca6848d564d5b0000000000fdffffffc981fca14d129ab41405cc749578413f23c645148a3b1d1ffd6ca6848d564d5b0100000000fdffffffd57aa2c8e8132ae56efd7779614e2e81330a3569f23d83f063d1d88d2e6573a70000000000fdffffffd57aa2c8e8132ae56efd7779614e2e81330a3569f23d83f063d1d88d2e6573a70100000000fdffffff3e5229aee8269948e4f6c2113a6eb3cb086a2863da0656367caa4b4d80c18ef10000000000fdffffff3e5229aee8269948e4f6c2113a6eb3cb086a2863da0656367caa4b4d80c18ef10100000000fdffffff0167b3090900000000160014d9e02c0052c03978e6fe70ac2d94d3a708f08b00024730440220679326695c6b223cb2444013c53dbdbec28fb501677bbc4caab967234ff4496b02204b957debfb8e87bcee93427c7525cbbeec70acdc15fc980898275f380498abb4012103b06f9f20aa97c1c5fa64d8c9309e61319e8e123ccda7b7a5e515a07e1c2059ae02473044022069d4710522ae39baabbcafa150b05decaaacbaaaef5432d313b0f0dbf79ae78002200985f6f608a6f0fd7fb2aeffe2ebe2a7e4016f5e724e6fdea8844751da663ae6012103d47d26e19a211bc4cd822fb111cfc6c95fa090309a7968be22320968514f32550247304402207b33ee2c5c23fa4b55b4e0885543f5bb1b236ec17bf40077880e3b83d52cb6c5022007155d99006ac5e26365204023a659e0458789547bfd4089d00c263ca508e67b012102a0612113e548097982c3c63bbb2b099f9a8b082e97182b74c73edb83a3f121b00247304402200b8cd6fdbf55b9d2455c9f7b732efb732c397e3a6b3804528b6f4ddea625e08c02200c0853a0d13e65810f7ee278a54a1e8854f9faa435eb3145ab900384da7e50b601210277bbd748ba49e1d25ba927779671dc7196ee9b4f0bbc1bbb466dfb9997e6fca102473044022062140fffc549ff198f5f172413000017d7e086091b7b2ce426b7ffb174b0213e02202a2379a88997d9d0904490282ca2df15774df3f812ad9d2c007aa78b89eac15b012102a0612113e548097982c3c63bbb2b099f9a8b082e97182b74c73edb83a3f121b00247304402202b00b09df92dfc16e25dce98496ae6b1c4c70bf0d4b5a2f4eaf3b18eb917ffb00220294d385364a654833daddde10494ecd5750378280d4b7d0338da6cf35b42cbbb012102a939313ba38614081aee2d413be801880a27d1b9d5083ef022b7a6ca932c68f573502700"
	_ = txToDefend

	var tx wire.MsgTx
	err := tx.Deserialize(hex.NewDecoder(strings.NewReader(txToDefend)))
	require.NoError(t, err)

	def := New()
	err = def.Defend(&tx)
	require.NoError(t, err)

	info, found := def.transactionsToIgnore[tx.TxHash()]
	require.True(t, found)
	tx2, err := info.Tx()
	require.NoError(t, err)
	t.Log(tx2.TxHash())
}

func TestDefender(t *testing.T) {
	node, err := lightnode.NewTestNet()
	require.NoError(t, err)

	// initialTX
	txToDefend := "020000000001080e8ee61ccaaaf3a2902bc63a931cb704737ddd2ecb025ae8a0b6c83cc85c72760000000000fdffffff0e8ee61ccaaaf3a2902bc63a931cb704737ddd2ecb025ae8a0b6c83cc85c72760100000000fdffffff1cd07c2685562a18e18cf2bc11bd1d953f29bceb59542959c288092f3b04ac990000000000fdffffff1cd07c2685562a18e18cf2bc11bd1d953f29bceb59542959c288092f3b04ac990100000000fdffffffd57aa2c8e8132ae56efd7779614e2e81330a3569f23d83f063d1d88d2e6573a70000000000fdffffffd57aa2c8e8132ae56efd7779614e2e81330a3569f23d83f063d1d88d2e6573a70100000000fdffffff3e5229aee8269948e4f6c2113a6eb3cb086a2863da0656367caa4b4d80c18ef10000000000fdffffff3e5229aee8269948e4f6c2113a6eb3cb086a2863da0656367caa4b4d80c18ef10100000000fdffffff012d7d0909000000001600146a4738bb27b5d8dc02b8cf207648f025a5217105024730440220678267ccd215617433f54e7eb6d37045c6a2be78925cf07b64e79264ff7a187702206a3809bf7e096d596894dc8c2749a132eb073507d140f9e26c36e22d1c6d06d40121036b7bbdb7064b1dcb1483b2634e5072f7fcf88448702aa4f421486f62ff953d780247304402200bda2874de7fd621652c70924e5b0369ae1881cf913ee020f517fd8f0119ae6c0220116df45bad3052174e93cc7906c727d9a4e9ce67dc08f12be46cea19dd5c5755012103e91e9dcb7949f1d81f960cbaa03084fd4dcb06c6e5df2d06967c6ad3473b67350247304402203880a7e2f07d08387fa150516db6580c83750aca1aa60ae3858d4c0bbe0c79da0220100ce0c6fe0f00c26c5fbbc8fd0b3dde33d5b63c8e9c1cd9364de63ccf9de7ef012102af3c60a952aea304e0e3c860bf4500deda6a386e0aaa1d8b1a17f8ecc0933860024730440220232981875612127fdf2932a81a1a9d6cc3062e78906b328619768c50a1e8b70d02203b7e639fd16e5b5250d5429a607bf7246e5811ad76315db0edf601431f0dda6e0121039fcf18cd610a8a4c0d4f4ebb5c2a707bb5ecf681febcdfed3b8babb6aa1cce0b0247304402202c4ea479f476dd09515f7275e68055349bf63d36ba58e8bf14a61ec3e7fffdff0220602206ccdb7c0da53b8252cf10688e70872a01c7607b5b076925fceabc1837d1012102a0612113e548097982c3c63bbb2b099f9a8b082e97182b74c73edb83a3f121b00247304402207f56b67be92155f72e851987ce05d2777b9cab514c676ff4b8d968fb3eb5b4fa02207fd1fa6dfc0b4757c8c17161429b6d10ecc510afdb2260102a39350345003f6501210277bbd748ba49e1d25ba927779671dc7196ee9b4f0bbc1bbb466dfb9997e6fca102473044022001f124e38109777f5701e9a14232c37d50fe67ef6b51c10dd9e13b02de56916f02202aa015e873d5362e927b253d658f47cb9436322e0c036ae62c29d0aa92faacc3012102a0612113e548097982c3c63bbb2b099f9a8b082e97182b74c73edb83a3f121b00247304402204b3d891d93a3ece7ac4733e5fec6b622586a78badc88228988d99664e15a386f02203b12fe304c33d0f2d7939296ccde3b2de114534c31d9ca5d6e315e16278ccd57012102a939313ba38614081aee2d413be801880a27d1b9d5083ef022b7a6ca932c68f575502700"
	_ = txToDefend

	var tx wire.MsgTx
	err = tx.Deserialize(hex.NewDecoder(strings.NewReader(txToDefend)))
	require.NoError(t, err)

	def := New()
	err = def.Defend(&tx)
	require.NoError(t, err)

	info, found := def.transactionsToIgnore[tx.TxHash()]
	require.True(t, found)
	tx2, err := info.Tx()
	require.NoError(t, err)
	t.Log(tx2.TxHash())

	t.Log("connecting to electrum nodes")

	initialBroadcaster := "testnet.hsmiths.com:53012"
	knownPeers := make(map[string]struct{})
	knownPeers[initialBroadcaster] = struct{}{}
	cli := broadcaster.NewElectrumClient()
	err = cli.Connect(initialBroadcaster)

	require.NoError(t, err)
	var clis []*broadcaster.ElectrumClient
	clis = append(clis, cli)

	peers, err := cli.GetPeers()

	clis = append(clis, addPeers(peers, knownPeers, 30)...)
	fmt.Println(len(clis), "electrum nodes connected to")

	go def.Start(node, clis)
	time.Sleep(time.Second)

	t.Log("connecting to bitcoin network")

	go node.ConnectV2()

	t.Cleanup(func() {
		for _, c := range clis {
			c.Disconnect()
		}
	})

	select {
	case <-node.Done():
	}

}

func addPeers(peers []broadcaster.PeerServer, knownPeers map[string]struct{}, max int) []*broadcaster.ElectrumClient {
	var clis []*broadcaster.ElectrumClient
	for _, p := range peers {
		if len(knownPeers) >= max {
			break
		}
		server := p.SSLServer()
		if server == "" {
			continue
		}
		if _, found := knownPeers[server]; found {
			continue
		}
		conn, err := p.Connect()
		if err != nil {
			continue
		}
		fmt.Println("connected to", server, "electrum")
		knownPeers[server] = struct{}{}
		clis = append(clis, conn)
	}

	for _, cli := range clis {
		if len(knownPeers) >= max {
			break
		}

		ps, err := cli.GetPeers()
		if err != nil {
			continue
		}
		clis = append(clis, addPeers(ps, knownPeers, max)...)
	}

	return clis
}
