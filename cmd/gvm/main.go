package main

import (
	"flag"
	"fmt"
	"gvm"
	"math"
	"time"

	"gvm/log"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/pkg/errors"
)

type args struct {
	Algod          string
	AlgodToken     string
	AppID          uint64
	Addr           string
	MaxLength      int
	OrdersInterval uint64
	ExpiryInterval uint64
	NonInteractive bool
	Batch          uint64
}

func run(a args) error {
	if a.Addr == "" {
		addr, err := gvm.ReadRewardsFromConfig()
		if err != nil {
			return errors.Wrap(err, "failed to read address from config")
		}

		a.Addr = addr

		if a.Addr == "" {
			if !a.NonInteractive {
				fmt.Print("Enter rewards address: ")
				_, err := fmt.Scanln(&a.Addr)
				if err != nil {
					return fmt.Errorf("failed to read address: %w", err)
				}
			}
		}

		if a.Addr == "" {
			fmt.Println("Address is required, either provide it via command line or interactively.")
		}
	}

	log.Info("CYPA miner starting")

	rewards, err := types.DecodeAddress(a.Addr)
	if err != nil {
		return fmt.Errorf("failed to decode address: %w", err)
	}

	err = gvm.WriteRewardsToConfig(a.Addr)
	if err != nil {
		return fmt.Errorf("failed to write address to config: %w", err)
	}

	log.Info("Algod:", a.Algod)
	log.Info("Application ID:", a.AppID)
	log.Info("Rewards address:", rewards.String())

	if a.MaxLength > 0 {
		log.Info("Max length:", a.MaxLength)
	} else {
		log.Info("Max length: no limit")
	}

	miner, err := gvm.NewMiner(a.Algod, a.AlgodToken, a.AppID)
	if err != nil {
		return err
	}

	log.Info("Miner initialized successfully. Awaiting for orders..")

	for {
		err := func() error {
			var header gvm.OrderHeader

			length := uint64(math.MaxUint64)
			found := false

			for item := range miner.List(a.MaxLength) {
				if item.Length < length {
					length = item.Length
					header = item
					found = true
				}
			}

			if !found {
				delay := time.Duration(a.OrdersInterval) * time.Millisecond
				fmt.Printf("No orders found, sleeping for %v..\n", delay)
				time.Sleep(delay)
				return nil
			}

			order, err := miner.Read(header.Key)
			if err != nil {
				return fmt.Errorf("failed to read order: %w", err)
			}

			fmt.Printf("Order received - Id: %d, Prefix: %s\n", order.Id, order.Text)

			rsagg, err := gvm.NewRsaggGenerator(a.Batch)
			if err != nil {
				return fmt.Errorf("failed to create Rsagg generator: %w", err)
			}

			last := time.Now()

			sk, err := rsagg.Generate(order.Text, func() bool {
				if time.Since(last) < time.Duration(a.ExpiryInterval)*time.Millisecond {
					return true
				}

				fmt.Println("Checking order validity...")

				result := func() bool {
					_, err := miner.Read(header.Key)
					if _, ok := err.(common.NotFound); ok {
						return false
					}

					return true
				}()

				if result {
					fmt.Println("Order is still valid, continuing generation...")
				} else {
					fmt.Println("Order is no longer valid, stopping generation.")
				}

				last = time.Now()
				return result
			})

			if err != nil {
				return errors.Wrap(err, "failed to generate private key")
			}

			addr, err := crypto.GenerateAddressFromSK(sk)
			if err != nil {
				return fmt.Errorf("failed to generate address from private key: %w", err)
			}

			fmt.Printf("Fulfill order for address: %s\n", addr)

			err = miner.Fulfill(sk, order.Key, order.Owner, rewards)
			if err != nil {
				return fmt.Errorf("failed to fulfill order: %w", err)
			}

			fmt.Println("Order fulfilled successfully!")
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
	flag.Uint64Var(&a.OrdersInterval, "orders-interval", 10000, "Delay in milliseconds between checks for new orders")
	flag.Uint64Var(&a.ExpiryInterval, "expiry-interval", 30000, "Delay in milliseconds between checks for order expiry")
	flag.BoolVar(&a.NonInteractive, "non-interactive", false, "Run in non-interactive mode (no prompts)")
	flag.Uint64Var(&a.Batch, "batch", 1000, "Batch size for the Rsagg generator")

	flag.Parse()

	err := run(a)
	if err != nil {
		panic(err)
	}
}
