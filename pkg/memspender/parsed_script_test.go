package memspender

import (
	"github.com/darwayne/chain-grabber/internal/test/testhelpers"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewParsedScript(t *testing.T) {
	t.Run("should detect multisig segwit", func(t *testing.T) {
		tx := testhelpers.TxFromHex(t, "010000000001015d4b529f841a4beac6f56abd91fe4b2002cd3f429064519d32dd9df10a70a87e0000000000ffffffff02203c670000000000160014a2e667bb74537ddea9aad6c1892afd320a6cba3b9009460200000000220020701a8d401c84fb13e6baf169d59684e17abd9fa216c8cc5b9fc63d622ff8c58d04004730440220115f01faaa6c0ab0e1efc94bf2b0b3953a4533de6a4070f28194d5622ea7c04a0220301224188c32f0672fc481db839d12f08459965d637138a8ace1f707d4eed91801473044022076d7b91b7f20f02d106afcd69218c580f647de7ecab7a8e055a6ca1a654241d70220071c03fb4fc88bd8ddfbb70eb869ddce969e8fb41608712a409201ad4f577fe401695221022b003d276bce58bef509bdcd9cf7e156f0eae18e1175815282e65e7da788bb5b21035c58f2f60ecf38c9c8b9d1316b662627ec672f5fd912b1a2cc28d0b9b00575fd2103c96d495bfdd5ba4145e3e046fee45e84a8a48ad05bd8dbb395c011a32cf9f88053ae00000000")
		t.Logf("view online: https://mempool.space/tx/%s", tx.TxHash().String())
		in := tx.TxIn[0]
		info := NewParsedScript(in.SignatureScript, in.Witness...)

		require.True(t, isSegwitMultiSig(in.Witness...))

		require.True(t, info.IsSegwitMultiSig())

		keys, minKeys, err := info.MultiSigKeys()
		require.NoError(t, err)
		require.Len(t, keys, 3)
		require.Equal(t, 2, minKeys)
		for _, k := range keys {
			require.True(t, k.IsOnCurve())
		}
	})

	t.Run("should detect P2PKH", func(t *testing.T) {
		tx := testhelpers.TxFromHex(t, "0100000001c2c26dd75a21ecedd4da168791c18a85afbb5ef4ad32a839090637415bee7400010000006b483045022100d82904b1b140fcb8b51ee8c5566c51edfecbb4ada366aa4839811383285a3106022043ffa01e14de87e7080fafdf2d112ef8fc638a5c39d213fbe36247296e5df53d0121028251724a83c93c093c2be109f65bf3f3b10f33ee6467cd84717c1186bd122470ffffffff020000000000000000306a2e9b25c2802fb601fb512294841a14f125b0fa4c4083d19141e235a88d39674ba6f17156ddfc5335e225380d30f35ce0493a00000000001976a914fd20e1f76a1245264683c67830b4163fbd9319b688ac00000000")
		t.Logf("view online: https://mempool.space/tx/%s", tx.TxHash().String())
		in := tx.TxIn[0]
		info := NewParsedScript(in.SignatureScript, in.Witness...)

		require.True(t, info.IsP2PKH())
		key, err := info.PublicKey()
		require.NoError(t, err)
		require.NotNil(t, key)
		require.True(t, key.IsOnCurve())
	})

	t.Run("should detect P2WPKH", func(t *testing.T) {
		tx := testhelpers.TxFromHex(t, "010000000001029535cc822cf304ab387529d42fc046f4b5008cf08792c2911a5e563399852ad30100000000ffffffffe4a9020553334c0cfeb0b487aa647a84feb5095b4d9a3fe65d11ad04b788a8f60100000000ffffffff02f09e5c00000000001976a9143cdb231544122b9d00d07243a9732e4eeadf16e488ac60230000000000001600148d310acec1cb3c14eca9084b07c2a6e13264a9550247304402200cd07173982a1a96794ed174c4265fcfba41a175fb461bd86c9558eddbc64f0c02204b732786f05ed807ae6cd0c9d47906a440844d1f41518fef1a47db1d8ebe707c012102bb6f0d424dfedc310291e5a237bc833bc73c4a2ed6fbd69b1ce73acd4fe2f3e102483045022100dfe2fa6b1b4b570039b91d50967a1df2a75bb0e558a32f5bf3914a5aa50a193802200c9cab49dd8a7e22d783f375de205c1716ccc138aa92d83d0a6b981a41cdf853012102bb6f0d424dfedc310291e5a237bc833bc73c4a2ed6fbd69b1ce73acd4fe2f3e100000000")
		t.Logf("view online: https://mempool.space/tx/%s", tx.TxHash().String())
		in := tx.TxIn[0]
		info := NewParsedScript(in.SignatureScript, in.Witness...)

		require.True(t, info.IsP2WPKH())
		key, err := info.PublicKey()
		require.NoError(t, err)
		require.NotNil(t, key)
		require.True(t, key.IsOnCurve())
	})
}
