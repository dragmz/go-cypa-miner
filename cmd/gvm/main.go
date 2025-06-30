package main

import (
	"flag"
	"gvm"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/pkg/errors"
)

type args struct {
	Algod      string
	AlgodToken string
	AppID      uint64
	Addr       string
}

func run(a args) error {
	miner, err := gvm.NewMiner(a.Algod, a.AlgodToken, a.AppID)
	if err != nil {
		return err
	}

	acc := crypto.GenerateAccount()
	if err != nil {
		return errors.Wrap(err, "failed to generate account")
	}

	key := []byte("TODO")

	err = miner.Fulfill(acc.PrivateKey, key, acc.Address[:])
	if err != nil {
		return errors.Wrap(err, "failed to fulfill")
	}

	return nil
}

func main() {
	var a args
	flag.StringVar(&a.Algod, "algod", "https://testnet-api.4160.nodely.dev", "Algod address")
	flag.StringVar(&a.AlgodToken, "algod-token", "", "Algod token")
	flag.Uint64Var(&a.AppID, "appid", 742018771, "Application ID")
	flag.StringVar(&a.Addr, "addr", "", "Address to use for the miner")

	flag.Parse()

	err := run(a)
	if err != nil {
		panic(err)
	}
}
