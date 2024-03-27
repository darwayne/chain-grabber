package memspender

import (
	"github.com/darwayne/chain-grabber/internal/test/testhelpers"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
)

func TestNewParsedScript(t *testing.T) {
	t.Run("should detect multisig segwit", func(t *testing.T) {
		t.Run("with 2 of 3", func(t *testing.T) {
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

		t.Run("with 3 of 4", func(t *testing.T) {
			tx := testhelpers.TxFromHex(t, "010000000001022b47f72e07addb9d2cd8dabae7209e328cbe9936f50c5fc889e52db07d8f5f9d0000000023220020b2a5eebd500c9f082370f568ea42dda922c212f20c44ab8a42bc6ffafc059ee2ffffffff3fad453aff9fc5b49409f5f8c1d614a95a5bcfd75634f3e721693f97596344440100000023220020b2a5eebd500c9f082370f568ea42dda922c212f20c44ab8a42bc6ffafc059ee2ffffffff01d43c0100000000001976a9142b4891b9aaba7e065d31c49588700073cac5176088ac0500483045022100a505ea5c7bd3473175b187b94199185bef1ac06d353dd7ecd384d82e1964520b02200ccc8478e0cc791ec6fcebf6e59452b05fc519cbe0a779cdd5128d7cb2a672db01473044022033642df7b96f30416c1a5d429cdaaba1df529e2470f4e7b450aad5a0ffb102200220792710ac35e8393096859cd35a7e3fca9db5b30b3fec00aa6c01c1f984dfed130147304402207022bac2a7ec5f437db9a780a37c9c08c37147b98ac8ff975148dbb62e49176802205aa655d4ff824616bb617ff2dcb880b159d46db8715dfeebd9deaeb9560e9756018b532102a744e998361204cbe1d7f2739d71cfb91784fd2a5543204835d73e6938e759f52102c75f27ced77825cab5b589b69cb97cb900ed2e29a2181017b6f400cd212c337421034116032003f531797ea62172d04673cc48b9ef7c93777155b74d21a75392872421039340145b71c845766b81ce2631ee2f7b46d28343370a4fe2b1077a197cdcb83254ae0500483045022100a06b00573602b7be97e11e860a151b273231cfdf6e7431299d32ed3954f8b310022026c811a4101f4b00920d4efa4690ee10daa13f2ed20e8cdd7dff7a67c77345b00147304402201b45a7d8893fb453200fdee1b49e2673d60bd6883dd405f0fec4719fc7f10b88022059079c13a61f052c1edd8a11349cb49fc3f7cfd1b9525bbfec7436df19d2f2c90147304402204d343878d9230ee8ddc9b88e00577331ada261393f869c8aa72b555b6ea6fbce022037ab0eca42566a94944f373afe43c59f3ce0055ffc2184e144c9998063f4446b018b532102a744e998361204cbe1d7f2739d71cfb91784fd2a5543204835d73e6938e759f52102c75f27ced77825cab5b589b69cb97cb900ed2e29a2181017b6f400cd212c337421034116032003f531797ea62172d04673cc48b9ef7c93777155b74d21a75392872421039340145b71c845766b81ce2631ee2f7b46d28343370a4fe2b1077a197cdcb83254ae00000000")
			t.Logf("view online: https://mempool.space/tx/%s", tx.TxHash().String())
			in := tx.TxIn[0]
			info := NewParsedScript(in.SignatureScript, in.Witness...)

			require.True(t, isSegwitMultiSig(in.Witness...))

			require.True(t, info.IsSegwitMultiSig())

			keys, minKeys, err := info.MultiSigKeys()
			require.NoError(t, err)
			require.Len(t, keys, 4)
			require.Equal(t, 3, minKeys)
			for _, k := range keys {
				require.True(t, k.IsOnCurve())
			}
		})

		t.Run("with 1 of 1", func(t *testing.T) {
			tx := testhelpers.TxFromHex(t, "010000000001028caac84aa0d8c1e5e814b0bdd053061c505fbecc9a27e3b5500b36744275c94e0100000000ffffffff22ec2b0a71acf48a1713608153dfd1a044e53e0a37a98c4c022778bba5782a7e00000000232200207cc127d8f5baed77b15cdcc7772d73e88fe4a5055a1d10cc17baa87579b5399affffffff32cab53700000000002200202d69d539c3f01a41c79910c03902c812f09a8895c9ffac9bf57a8048128347f950ae420000000000220020cabbb56da08eac587612bb5456256889ec78c26ef31d8bd4a31cb6cfc6b30b41f034440000000000220020459258d11716464ff9783088ea8be7f441b851c83e44e53b716b454deb202343108344000000000022002007fc977713bad70da61e76f97efd41f60706ac4142f6741f053c196fa8104d9e8094450000000000220020bf911cc9d276eded8858d1df7e5aa61a8280e7369f53caea2177560d4b951c5b60b747000000000022002022a6a33a27ecacd58862a47cc01de72fba6934a8612e8e448304456df70d5ae73cbc470000000000220020b7cb9c4f9a4cfa3c214c13e83027e96a9d08b8f62834adde30472056f15f2fd1c0a1480000000000220020ce4523a35626d898be2954436c92fba3ada508312a7a4ecd975aaa91bc7159d110d64b000000000022002078d4ba0d5c3b5d4a6c802501de9b16458a314e51f697df92e9108090b0473f2f20fd4b000000000022002053cc3546faeef75ebc0f5037b99d37906a357266bc4bfd2588b92d75b9b8495b206e4e00000000002200205a8ff8f077518ca70a380dde821f28f168554c70f36154994f7fb952cf55808740bc4e000000000022002063d0d7c063f1f2fcc1c28239c10f6d6856325aa5c42822aa21e69f261ce8c186d01b5000000000002200206c92810f76a9fe3e7d06df18fd86b85f1d6eea7811191d815daf41982727de2d10b850000000000022002051986c744fce681d2e9c8d09b8140ce922ffaed8426abbd8262f1c377e834a439061540000000000220020393af09c71d720d22faada80230510681150d0e34bd0b5250f538178578a8e2eb0af54000000000022002075b3e360cdc6cba1478e6159619e184864ff06024f2899e8eeda6aa92905be59503656000000000022002088a8f8c3c1f91e4bcda9405e6ab1d16d9913675fa237934f45b3eb4b00651b15605d5600000000002200202dedfcc96c642725b629748d3a04cbc52199521dbb60b294a1f56307b00b62e37084560000000000220020a4b01e38cc6c5efad516ead8ace7cb3e697ac5196e0883fa07ffcc5934ea90ee00e4570000000000220020c2e92383fd4004e82bdc7aa4138cc3d4438a06b86bd433464bb551a3b406f3cff02d5a0000000000220020b4e1386f8128ecd5562585c29dacb9e4767328a674e5bcb5d8b2bb0b1db4ef24107c5a000000000022002092de45780895dce3c5c8bc6af6edf51154c66daa740170689ff972a2f7c836a5c0295c00000000002200203a361828411bb0922ec82ec459a45b651bd9e8a64571bb9bd97677e1f079582610ed5c00000000002200207917d278f5dbaf5c4390aab875dc9f126f5e6ab9bac744b4cf2b6f24432d3092b0e4600000000000220020cd2d8a4ed042700b4347c8941f6714f1c94023306a585ed12b7311cc086ffd79d032610000000000220020ffb085f71eb96153f9965e4226d786c8ce404fa7713cc234b29d79942a64533cd014660000000000220020235766f590cde263b405adcbe8b22267b6fc1c39d83986c918dec234edffa94a10b1660000000000220020142c0034fa681a65bfb7d1a9c6570db6c17fdd81707e2e5c48909009fa6692f6c05e6800000000002200205dfd0ad8a5407a791028cd38d3288a6dd2994f870deb78a78998b562f0b67d362049690000000000220020bfab713b868ec27809a18f1fe6fc536a5fb71f9ba6f5d2f2724852d8fecff44c40976900000000002200204e6768738dbd5dcfa48814b5bf454ceef97696a0946b8a7b91d833ee9be4368bb0196d0000000000220020eed023a41aab2f977262b5ef40dddda723c82e6bcbd0fbfbb0d13720c6f9a509e08e6d0000000000220020fef8c379ee9309b1e20bdf623c929ccc7ef8ffdba09e44c3bd426e1333afe6e010046e000000000022002061f98aeb5955d1e72c987b13df4323d2c7aefa3bb1f445335f4e7ffe6164cdde004e7000000000002200207ecbbee4c1339fcedcdb5413e108493a08ae717e1b920d838ef1dee72438c10160387100000000002200207146335b7c4713493c49cc3ba8446f9c4953db346ff8324215382e94aaafdc72f09772000000000022002007659bd06324bf799e8408ba767d7dcc53aadadfcf02421cbdf1fc59e8e4525b0030750000000000220020782ac4ea9d2cf7e6ea25c03a7332f1fd76661a6b1c87c9edcb824de6d94cfd4200a17700000000002200207541dc79b5eeaa08f81763189208d760745d655ddcd0b53c1d0a5c2d359d0fb6b04e79000000000022002010a3383f596f11e923f4aafff7870401b8e0686209972ccafb2e17a663cd91aae0c3790000000000220020ed60025e0626ae08f27d292161e6755175bbd284b6b14ffc800cb3cf7b53c913f0ea790000000000220020a2d6bcef8f3027034eb9826466ac0f4f2a7055b4741d269695a78f3f00694c3e804a7b0000000000220020b7c7857e5c5397c140db112a4b133f542498017f4783d92d8826b906cdb72efeb0bf7b0000000000220020e50349ea4104a297f0d67a8fce0434110018b4ebdf6a4c05fefdbe70dcf64fba00837c00000000002200206c245b7a87229cec750ae896e6b544212e3419d8a65dd178418f62530b9eb2dcc0577e0000000000220020618339a593ccd8863c24c348c774cbdc1234e90d2736de8a10c361db591a934520427f0000000000220020a2ee7c79713a96dcbd9481de60f7ba474e64efa10dda449fdf34f3b2294cd71140907f00000000002200200148130a68eeb5427138c1775750be7569ac9e36ba09fa7e0c1e8277cd50d4df50b77f000000000022002082d8981b46fe3ece540de172fae40db2aa67d48f7bbd90e6fb554a561a2a0fada07a800000000000220020cf872c6c53b765539b4be64dca0ba584db2925881d51c8c6ed89245fb1380e0b0300483045022100bd98c6416712dff8e2853241762b987f7dac2b98a243c5de365d1e037f169d4e0220446aa250e3f8b07a0bcc65252ef5bbbedd53de110660be6d9e792f89b8e4ff0301255121033f1237cf1d8a5ff8743f513859314d90025ec8309f01d43c2184dff91ed99abe51ae0300473044022077bc8c11506ae0d11f67631309b3f90e5503fc7399d68f7440c2295a6eebc611022069dedd7d0e5a4a1f4474b7fd3456686b15b23cb4edfe0e1457271d494e21bcc601255121039134672ee6f1255d1026557340d8822fc1a18637208e5cc67d5e17d0a96b0a1151ae00000000")
			t.Logf("view online: https://mempool.space/tx/%s", tx.TxHash().String())
			in := tx.TxIn[0]
			info := NewParsedScript(in.SignatureScript, in.Witness...)

			require.True(t, isSegwitMultiSig(in.Witness...))

			require.True(t, info.IsSegwitMultiSig())

			keys, minKeys, err := info.MultiSigKeys()
			require.NoError(t, err)
			require.Len(t, keys, 1)
			require.Equal(t, 1, minKeys)
			for _, k := range keys {
				require.True(t, k.IsOnCurve())
			}
		})

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

	t.Run("should NOT panic", func(t *testing.T) {
		transactions := []struct {
			str string
		}{
			{"0200000000010842ced294e88382f095235542f28c60c46ed45035d0d705006921936b0b8a5c0d0000000000fdffffffa914371aa29d3bd87e3f51900df9050c216fa3bf4bb7be3d0ba74c1e97e657950000000000fdffffffff75bcb9e33ed11751e0faeaac0f29791b51e9bb6e37fbffe05600be9b195b570000000000fdffffff72e0a77b3e635a68f4da8ce9d28587d3e29ce7378af664e2486a2bc8d988db510000000000fdffffff04f124757171daa4377f1741833bf0ab1f475a636d42451509f631d361cc007b0000000000fdffffff1f3eec57473978db784a5257f7295bc85da819886995754f47492d51e84b27d20000000000fdffffff8bc4d298f289d618adfd9e2fc523aec51b26548abbe026fde26c621ab5fccda90000000000fdffffff2ce26bcff1483f17fccf6cc7377a962654f0e22dc0877982ef9f80bee00d17b80000000000fdffffff01da09000000000000225120b19743dd5ededecb6d29ebc5fa990b45ecb00db01c74999486d65989a1a3ff0101401e3e75d8c49b0bf17ae978b3c7ab5d524560f7ac638cb2453b4b563073c9a42bca118d991136b0db0b897c97338cbc967804164a96cfdb1bcb802e447b44c4ae014051ed2ea0e6ed046d13f0df6318be01d5ff77e983adc4c0ef4c2fa1feec98fead4440478ac9022bbb4d6f16e88fcac7e79a5ecf385898657ef361b13282cccf5c0140ae44550184143bac29ca3b0278865e8f10252dedeeae062eb1e254b5190be644a3136833b0ca9e350bac8b0f8ad623aa8a917a932fe272eeeb1d610e3d59030f01409a9f12545c64d72013804833d051789c2e46f36aa15430e3fa178e6e361033550b0ad2d5d28020666f8da98c7012d73cc53a53789b1f0a2817907ecd39605da701401342855bc168f18426300b00865f96c4596ff469e6fde1029de7203898bf65d5bd6643bed99c5f3f17907b540f0ec0c38213c35a906d85b1a301805222cdb7c401400c9a449cb58de12bc30242d8d6f7b384fd4217cf1419bf76629afbe4b31a2867740c8b9e2faf5b40a69b6332c28d1911492c525caaba00216b03cccd4c758bd30140ac01f5cede9b654686f1f1a151446a1c83e08960bfd5811ef5b49f9ededd430faed2f0f65b64882ef3405f6be3ea9dfb297c1de44600d8320d840b7727dbae030140e98db85a98243648a09adc982b7551e5b46625ffad10fda3019887b004e6b8574dae2daee40cc22e9318dd1a0c14b44071eac3dc91075eb74811375030059baf00000000"},
			{"02000000000104c7a8dd02f44df08ac31e7ab8b641c611c5ae539c107fb2fba789ecb1db88d08c0300000000ffffffffc7a8dd02f44df08ac31e7ab8b641c611c5ae539c107fb2fba789ecb1db88d08c0400000000ffffffff40b4d36d9ef4356ad378e7bfc2a247f0f4affc242a7f65fb1038d6e06f88f9d20000000000ffffffffcdba50108a28165f4abfe4647738f67090bdca7278f37d0d88ac0fb3411cb8730000000000ffffffff06b004000000000000225120b92e470341c81b2df750f89c1501ac401dba9c189408f7014a46cec798a8aa202202000000000000225120b92e470341c81b2df750f89c1501ac401dba9c189408f7014a46cec798a8aa202023090000000000225120f821234a307f0a50c159c099b600f3cbfa1b615f1986b71cdfed2a482e2dd9ce5802000000000000225120b92e470341c81b2df750f89c1501ac401dba9c189408f7014a46cec798a8aa205802000000000000225120b92e470341c81b2df750f89c1501ac401dba9c189408f7014a46cec798a8aa208890000000000000225120b92e470341c81b2df750f89c1501ac401dba9c189408f7014a46cec798a8aa200140bc70ef38aff8d01121999dc290d3594bea594e658909b2aaeadebed433da50e80fa1bf049c2444fce9a2a8666e67325870513c70e9af1da06ef0d37b6e24fe3f0140ae3fd2ff31ded604ca1eada4b0c5cca83e75e33cc1ffbe4318dac96ac85c77c4cfc060ae9a44dd9c1bf4d5ef1adb3f2837c562507604813df5a344c500541fa4014174ae7029fa178e9a511026e8ad1a54eb08d3ab788223d8438c3edbea9e1ebee8474c7788792b227aa5846bb94ffde64c9fbb96b369a4172d7bbec330c4728cf583014055235a6c2b724a5f07c6cffe44e37f3af1e2e4d9bf5b8e529fb9c81572ff4cc188b3f2683289a106a5b743b92c061bcc963d412d55a8ba075957f3788b9cbac600000000"},
		}

		for idx, tt := range transactions {
			t.Run(strconv.Itoa(idx), func(t *testing.T) {
				tx := testhelpers.TxFromHex(t, tt.str)
				for _, in := range tx.TxIn {
					info := NewParsedScript(in.SignatureScript, in.Witness...)
					require.NotPanics(t, func() {
						info.IsSegwitMultiSig()
						info.IsP2WPKH()
						info.IsP2PKH()
						info.PublicKey()
						info.PublicKeyRaw()
						info.MultiSigKeysRaw()
						info.MultiSigKeys()
					})
				}
			})
		}
	})
}