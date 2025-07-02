package gvm

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/mnemonic"
	"golang.org/x/crypto/ed25519"
)

type Generator interface {
	Generate(prefix string) (ed25519.PrivateKey, error)
}

type RsaggGenerator struct {
}

func NewRsaggGenerator() Generator {
	// check ig bacon exists
	_, err := os.Stat(BaconExe)
	if os.IsNotExist(err) {
		// if not download it
		url := BaconUrl
		fmt.Println("bacon not found, please download it from", url)

		client := &http.Client{}
		resp, err := client.Get(url)
		if err != nil {
			panic(fmt.Errorf("failed to download bacon: %w", err))
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			panic(fmt.Errorf("failed to download bacon, status code: %d", resp.StatusCode))
		}

		out, err := os.Create(BaconExe)
		if err != nil {
			panic(fmt.Errorf("failed to create bacon file: %w", err))
		}

		defer out.Close()

		if _, err := out.ReadFrom(resp.Body); err != nil {
			panic(fmt.Errorf("failed to write bacon file: %w", err))
		}

		if err := out.Chmod(0755); err != nil {
			panic(fmt.Errorf("failed to set permissions on bacon file: %w", err))
		}

		fmt.Println("bacon downloaded successfully, please run it again")
	}

	return &RsaggGenerator{}
}

func (g *RsaggGenerator) Generate(prefix string) (ed25519.PrivateKey, error) {
	f, err := os.CreateTemp("", "cypa")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	f.Close()

	name := f.Name()

	cmd := exec.Command("./bacon", "generate", "--benchmark", "--output", name, prefix)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("command failed: %w", err)
	}

	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	for line := range strings.Lines(string(data)) {
		parts := strings.SplitN(line, ",", 2)
		mnemonics := strings.TrimSpace(parts[1])

		sk, err := mnemonic.ToPrivateKey(mnemonics)
		if err != nil {
			return nil, fmt.Errorf("failed to convert mnemonic to private key: %w", err)
		}

		return sk, nil
	}

	return nil, fmt.Errorf("no valid mnemonic found in output")
}
