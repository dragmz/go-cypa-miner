package gvm

import (
	"fmt"
	"os"
	"path"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/mnemonic"
)

func ReadAddressFromSecret() (string, error) {
	fmt.Println("No address provided..")
	os.Mkdir(".cypa", os.ModePerm)
	fp := path.Join(".cypa", "secret")

	_, err := os.Stat(fp)
	if os.IsNotExist(err) {
		fmt.Println("No secret file found, generating a new address..")

		acc := crypto.GenerateAccount()

		mnemonics, err := mnemonic.FromPrivateKey(acc.PrivateKey)
		if err != nil {
			return "", fmt.Errorf("failed to generate mnemonic: %w", err)
		}

		os.WriteFile(fp, []byte(mnemonics), os.ModePerm)
	}

	fmt.Println("Reading address from secret file:", fp)

	data, err := os.ReadFile(fp)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file: %w", err)
	}

	sk, err := mnemonic.ToPrivateKey(string(data))
	if err != nil {
		return "", fmt.Errorf("failed to generate address from secret key: %w", err)
	}

	addr, err := crypto.GenerateAddressFromSK(sk)
	if err != nil {
		return "", fmt.Errorf("failed to generate address from private key: %w", err)
	}

	return addr.String(), nil
}
