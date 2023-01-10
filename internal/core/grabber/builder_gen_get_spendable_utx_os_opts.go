// Code generated by builder-gen. DO NOT EDIT.
package grabber

import (
	"github.com/btcsuite/btcd/btcutil"
)

func (o GetSpendableUTXOsOpts) HasSpend() bool {
	return o.Spend != nil
}

func (o GetSpendableUTXOsOpts) HasAddressMap() bool {
	return o.AddressMap != nil
}

func (o GetSpendableUTXOsOpts) HasIgnoreUnknownSpenders() bool {
	return o.IgnoreUnknownSpenders != nil
}

type GetSpendableUTXOsOptsFunc func(*GetSpendableUTXOsOpts)

func ToGetSpendableUTXOsOpts(opts ...GetSpendableUTXOsOptsFunc) GetSpendableUTXOsOpts {
	var info GetSpendableUTXOsOpts
	ToGetSpendableUTXOsOptsWithDefault(&info, opts...)

	return info
}

func ToGetSpendableUTXOsOptsWithDefault(info *GetSpendableUTXOsOpts, opts ...GetSpendableUTXOsOptsFunc) {
	for _, o := range opts {
		o(info)
	}
}

func WithSpend(spendParam btcutil.Amount) GetSpendableUTXOsOptsFunc {
	return func(opts *GetSpendableUTXOsOpts) {
		opts.Spend = &spendParam
	}
}

func WithAddressMap(addressMapParam AddressMap) GetSpendableUTXOsOptsFunc {
	return func(opts *GetSpendableUTXOsOpts) {
		opts.AddressMap = &addressMapParam
	}
}

func WithIgnoreUnknownSpenders(ignoreUnknownSpendersParam bool) GetSpendableUTXOsOptsFunc {
	return func(opts *GetSpendableUTXOsOpts) {
		opts.IgnoreUnknownSpenders = &ignoreUnknownSpendersParam
	}
}
