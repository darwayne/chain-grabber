package indexer

import (
	"bytes"
	"encoding/gob"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/syndtr/goleveldb/leveldb"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sync"
)

type RawUTXO struct {
}

func (r *RawUTXO) getDBPath(wipeFirst ...bool) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	loc := filepath.Join(home, "btc", "mainnet", "utxo")
	if len(wipeFirst) > 0 && wipeFirst[0] {
		os.RemoveAll(loc)
		os.MkdirAll(loc, os.ModePerm)
	}

	return loc, nil
}

func (r *RawUTXO) getDB(wipeFirst ...bool) (*leveldb.DB, error) {
	loc, err := r.getDBPath(wipeFirst...)
	if err != nil {
		return nil, err
	}

	return leveldb.OpenFile(loc, nil)
}

func (r *RawUTXO) getSourcePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	loc := filepath.Join(home, "btc", "mainnet", "rawcoreutxo", "chainstate")

	return loc, nil
}

func (r *RawUTXO) getSourceDB() (*leveldb.DB, error) {
	loc, err := r.getSourcePath()
	if err != nil {
		return nil, err
	}

	return leveldb.OpenFile(loc, nil)
}

var rawUTXOMu sync.Mutex

func (r *RawUTXO) Iter() error {
	rawUTXOMu.Lock()
	defer rawUTXOMu.Unlock()
	utxoDB, err := r.getDB()
	if err != nil {
		return err
	}

	iter := utxoDB.NewIterator(nil, nil)

	for iter.Next() {
		key := iter.Key()
		val := iter.Value()

		_, _ = key, val
	}

	return nil
}

func (r *RawUTXO) Get(key wire.OutPoint) (UTXOValue, error) {
	rawUTXOMu.Lock()
	defer rawUTXOMu.Unlock()
	utxoDB, err := r.getDB()
	if err != nil {
		return UTXOValue{}, err
	}

	dataKey, err := toGob(key)
	if err != nil {
		return UTXOValue{}, err
	}

	val, err := utxoDB.Get(dataKey, nil)
	if err != nil {
		return UTXOValue{}, err
	}

	var value UTXOValue
	reader := bytes.NewBuffer(val)
	if err := gob.NewDecoder(reader).Decode(&value); err != nil {
		return UTXOValue{}, err
	}
	return value, nil
}

func (r *RawUTXO) Index() error {
	rawUTXOMu.Lock()
	defer rawUTXOMu.Unlock()
	utxoDB, err := r.getDB()
	if err != nil {
		return err
	}
	defer utxoDB.Close()
	db, err := r.getSourceDB()
	if err != nil {
		return err
	}
	defer db.Close()
	it := db.NewIterator(nil, nil)
	defer it.Release()

	var obfuscateKey []byte

	for it.Next() {
		//i++
		key := it.Key()
		val := it.Value()

		prefix := key[0]
		if prefix == 14 {
			obfuscateKey = val
		}

		if prefix == 67 {
			txidLE := key[1:33]
			h, err := chainhash.NewHash(txidLE)
			if err != nil {
				return err
			}

			idx := Varint128Decode(key[33:])

			// Copy the obfuscateKey ready to extend it
			obfuscateKeyExtended := obfuscateKey[1:] // ignore the first byte, as that just tells you the size of the obfuscateKey

			// Extend the obfuscateKey so it's the same length as the value
			for i, k := len(obfuscateKeyExtended), 0; len(obfuscateKeyExtended) < len(val); i, k = i+1, k+1 {
				// append each byte of obfuscateKey to the end until it's the same length as the val
				obfuscateKeyExtended = append(obfuscateKeyExtended, obfuscateKeyExtended[k])
				// Example
				//   [8 175 184 95 99 240 37 253 115 181 161 4 33 81 167 111 145 131 0 233 37 232 118 180 123 120 78]
				//   [8 177 45 206 253 143 135 37 54]                                                                  <- obfuscate key
				//   [8 177 45 206 253 143 135 37 54 8 177 45 206 253 143 135 37 54 8 177 45 206 253 143 135 37 54]    <- extended
			}

			// XOR the val with the obfuscateKey (xor each byte) to de-obfuscate the val
			var xor []byte // create a byte slice to hold the xor results
			for i := range val {
				result := val[i] ^ obfuscateKeyExtended[i]
				xor = append(xor, result)
			}

			offset := 0

			// First Varint
			// ------------
			// b98276a2ec7700cbc2986ff9aed6825920aece14aa6f5382ca5580
			// <---->
			varint, bytesRead := Varint128Read(xor, 0) // start reading at 0
			offset += bytesRead
			varintDecoded := Varint128Decode(varint)

			height := varintDecoded >> 1
			_ = height

			// Second Varint
			// -------------
			// b98276a2ec7700cbc2986ff9aed6825920aece14aa6f5382ca5580
			//       <---->
			varint, bytesRead = Varint128Read(xor, offset) // start after last varint
			offset += bytesRead
			varintDecoded = Varint128Decode(varint)

			amount := btcutil.Amount(DecompressValue(varintDecoded))

			point := wire.OutPoint{
				Hash:  *h,
				Index: uint32(idx),
			}

			pointKey, err := toGob(point)
			if err != nil {
				return err
			}
			pointValue, err := toGob(UTXOValue{Value: amount, Height: uint32(height)})
			if err != nil {
				return err
			}

			err = utxoDB.Put(pointKey, pointValue, nil)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func toGob(v any) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type UTXOValue struct {
	Value  btcutil.Amount
	Height uint32
}

func Varint128Decode(bytes []byte) int64 {
	var n int64

	for _, v := range bytes {
		// 1. shift n left 7 bits (add some extra bits to work with)
		//                             00000000
		n = n << 7

		// 2. set the last 7 bits of each byte in to the total value
		//    AND extracts 7 bits only 10111001  <- these are the bits of each byte
		//                              1111111
		//                              0111001  <- don't want the 8th bit (just indicated if there were more bytes in the varint)
		//    OR sets the 7 bits
		//                             00000000  <- the result
		//                              0111001  <- the bits we want to set
		//                             00111001
		n = n | int64(v&127)

		// 3. add 1 each time (only for the ones where the 8th bit is set)
		if v&128 != 0 { // 0b10000000 <- AND to check if the 8th bit is set
			// 1 << 7     <- could always bit shift to get 128
			n++
		}

	}

	return n
	// 11101000000111110110

}

func Varint128Read(bytes []byte, offset int) ([]byte, int) {

	// store bytes
	result := []byte{} // empty byte slice

	// loop through bytes
	for _, v := range bytes[offset:] { // start reading from an offset

		// store each byte as you go
		result = append(result, v)

		// Bitwise AND each of them with 128 (0b10000000) to check if the 8th bit has been set
		set := v & 128 // 0b10000000 is same as 1 << 7

		// When you get to one without the 8th bit set, return that byte slice
		if set == 0 {
			return result, len(result)
			// Also return the number of bytes read
		}
	}

	// Return zero bytes read if we haven't managed to read bytes properly
	return result, 0

}

func DecompressValue(x int64) int64 {

	var n int64 = 0 // decompressed value

	// Return value if it is zero (nothing to decompress)
	if x == 0 {
		return 0
	}

	// Decompress...
	x = x - 1   // subtract 1 first
	e := x % 10 // remainder mod 10
	x = x / 10  // quotient mod 10 (reduce x down by 10)

	// If the remainder is less than 9
	if e < 9 {
		d := x % 9       // remainder mod 9
		x = x / 9        // (reduce x down by 9)
		n = x*10 + d + 1 // work out n
	} else {
		n = x + 1
	}

	// Multiply n by 10 to the power of the first remainder
	result := float64(n) * math.Pow(10, float64(e)) // math.Pow takes a float and returns a float

	// manual exponentiation
	// multiplier := 1
	// for i := 0; i < e; i++ {
	//     multiplier *= 10
	// }
	// fmt.Println(multiplier)

	return int64(result)

}

func DecompressPublicKey(publickey []byte) []byte {
	// first byte (indicates whether y is even or odd)
	prefix := publickey[0:1]

	// remaining bytes (x coordinate)
	x := publickey[1:]

	// y^2 = x^3 + 7 mod p
	p, _ := new(big.Int).SetString("0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f", 0)
	x_int := new(big.Int).SetBytes(x)
	x_3 := new(big.Int).Exp(x_int, big.NewInt(3), p)
	y_sq := new(big.Int).Add(x_3, big.NewInt(7))
	y_sq = new(big.Int).Mod(y_sq, p)

	// square root of y - secp256k1 is chosen so that the square root of y is y^((p+1)/4)
	y := new(big.Int).Exp(y_sq, new(big.Int).Div(new(big.Int).Add(p, big.NewInt(1)), big.NewInt(4)), p)

	// determine if the y we have caluclated is even or odd
	y_mod_2 := new(big.Int).Mod(y, big.NewInt(2))

	// if prefix is even (indicating an even y value) and y is odd, use other y value
	if (int(prefix[0])%2 == 0) && (y_mod_2.Cmp(big.NewInt(0)) != 0) { // Cmp returns 0 if equal
		y = new(big.Int).Mod(new(big.Int).Sub(p, y), p)
	}

	// if prefix is odd (indicating an odd y value) and y is even, use other y value
	if (int(prefix[0])%2 != 0) && (y_mod_2.Cmp(big.NewInt(0)) == 0) { // Cmp returns 0 if equal
		y = new(big.Int).Mod(new(big.Int).Sub(p, y), p)
	}

	// convert y to byte array
	y_bytes := y.Bytes()

	// make sure y value is 32 bytes in length
	if len(y_bytes) < 32 {
		y_bytes = make([]byte, 32)
		copy(y_bytes[32-len(y.Bytes()):], y.Bytes())
	}

	// return full x and y coordinates (with 0x04 prefix) as a byte array
	uncompressed := []byte{0x04}
	uncompressed = append(uncompressed, x...)
	uncompressed = append(uncompressed, y_bytes...)

	return uncompressed
}
