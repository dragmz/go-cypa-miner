package gvm

import (
	"fmt"
	"os"
	"path"
)

func WriteRewardsToConfig(addr string) error {
	os.Mkdir(".cypa", os.ModePerm)
	fp := path.Join(".cypa", "rewards")

	err := os.WriteFile(fp, []byte(addr), 0600)
	if err != nil {
		return fmt.Errorf("failed to write secret file: %w", err)
	}
	return nil
}

func ReadRewardsFromConfig() (string, error) {
	os.Mkdir(".cypa", os.ModePerm)
	fp := path.Join(".cypa", "rewards")

	_, err := os.Stat(fp)
	if os.IsNotExist(err) {
		return "", nil
	}

	addr, err := os.ReadFile(fp)
	if err != nil {
		return "", fmt.Errorf("failed to read secret file: %w", err)
	}

	return string(addr), nil
}
