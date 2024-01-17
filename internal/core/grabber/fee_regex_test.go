package grabber

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFeeRegex(t *testing.T) {
	t.Run("scenario 1", func(t *testing.T) {
		msg := "TX rejected: replacement transaction 22968ac90519ebc2e98288266e36849cda6409260d6d1edefba9c55df9c75498 has an insufficient absolute fee: needs 1438, has 1218"
		results := feeRegex.FindStringSubmatch(msg)
		require.Len(t, results, 2)
		require.Equal(t, results[1], "1438")
	})

	t.Run("scenario 2", func(t *testing.T) {
		msg := "TX rejected: replacement transaction 7db370d8a48b53476bdd55d47bf22e6266dfeeecf99b867e2a3bcd0033e71813 has an insufficient fee rate: needs more than 2972, has 2972"
		results := feeRegex.FindStringSubmatch(msg)
		require.Len(t, results, 2)
		require.Equal(t, results[1], "2972")
	})

}

func TestInputRegex(t *testing.T) {
	t.Run("scenario 1", func(t *testing.T) {
		msg := "TX rejected: replacement transaction spends new unconfirmed input 5579bbe5a13510b41565c1c0c71864c1daa703299dd654d2ea4d6ddb55340a72:1 not found in conflicting transactions"
		results := unconfirmedInputRegex.FindStringSubmatch(msg)
		require.Len(t, results, 2)
		require.Equal(t, results[1], "5579bbe5a13510b41565c1c0c71864c1daa703299dd654d2ea4d6ddb55340a72:1")
	})
}
