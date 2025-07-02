package main

import (
	"flag"
	"fmt"
	"gvm"
	"os"
	"path"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/mnemonic"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

type args struct {
	Algod      string
	AlgodToken string
	AppID      uint64
	Addr       string
	MaxLength  int
	Delay      uint64
}

func run(a args) error {
	if a.Addr == "" {
		fmt.Println("No address provided..")
		os.Mkdir(".cypa", os.ModePerm)
		fp := path.Join(".cypa", "secret")

		_, err := os.Stat(fp)
		if os.IsNotExist(err) {
			fmt.Println("No secret file found, generating a new address..")

			acc := crypto.GenerateAccount()

			mnemonics, err := mnemonic.FromPrivateKey(acc.PrivateKey)
			if err != nil {
				return fmt.Errorf("failed to generate mnemonic: %w", err)
			}

			os.WriteFile(fp, []byte(mnemonics), os.ModePerm)
		}

		fmt.Println("Reading address from secret file..")

		data, err := os.ReadFile(fp)
		if err != nil {
			return fmt.Errorf("failed to read secret file: %w", err)
		}

		sk, err := mnemonic.ToPrivateKey(string(data))
		if err != nil {
			return fmt.Errorf("failed to generate address from secret key: %w", err)
		}

		addr, err := crypto.GenerateAddressFromSK(sk)
		if err != nil {
			return fmt.Errorf("failed to generate address from private key: %w", err)
		}

		a.Addr = addr.String()
	}

	rewards, err := types.DecodeAddress(a.Addr)
	if err != nil {
		return fmt.Errorf("failed to decode address: %w", err)
	}

	fmt.Println("Using address:", rewards.String())

	miner, err := gvm.NewMiner(a.Algod, a.AlgodToken, a.AppID)
	if err != nil {
		return err
	}

	for {
		err := func() error {
			for key := range miner.List(a.MaxLength) {
				order, err := miner.Read(key)
				if err != nil {
					continue
				}

				fmt.Printf("Order: %+v\n", order)

				rsagg := gvm.NewRsaggGenerator()

				sk, err := rsagg.Generate(order.Text)
				if err != nil {
					return err
				}

				addr, err := crypto.GenerateAddressFromSK(sk)
				if err != nil {
					return fmt.Errorf("failed to generate address from private key: %w", err)
				}

				fmt.Printf("Fulfill order for address: %s\n", addr)

				err = miner.Fulfill(sk, order.Key, rewards[:])
				if err != nil {
					return fmt.Errorf("failed to fulfill order: %w", err)
				}

				fmt.Println("Order fulfilled successfully!")
				return nil
			}

			delay := time.Duration(a.Delay) * time.Millisecond
			fmt.Printf("No orders found, sleeping for %v..\n", delay)
			time.Sleep(delay)

			return nil
		}()

		if err != nil {
			fmt.Println("Error:", err)
		}
	}
}

func main() {
	var a args
	flag.StringVar(&a.Algod, "algod", "https://testnet-api.4160.nodely.dev", "Algod address")
	flag.StringVar(&a.AlgodToken, "algod-token", "", "Algod token")
	flag.Uint64Var(&a.AppID, "appid", 742018771, "Application ID")
	flag.StringVar(&a.Addr, "addr", "", "Address to use for the miner")
	flag.IntVar(&a.MaxLength, "max-length", 0, "Maximum length of the prefix to search for")
	flag.Uint64Var(&a.Delay, "delay", 10000, "Delay in milliseconds between checks for new orders")

	flag.Parse()

	err := run(a)
	if err != nil {
		panic(err)
	}
}
