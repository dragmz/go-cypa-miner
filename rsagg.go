package gvm

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/mnemonic"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
)

type Generator interface {
	Generate(prefix string, validityCb func() bool) (ed25519.PrivateKey, error)
}

type RsaggGenerator struct {
}

func Download(url string, to string) error {
	client := &http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		return errors.Wrap(err, "failed to download bacon")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download bacon, status code: %d", resp.StatusCode)
	}

	out, err := os.Create(BaconExe)
	if err != nil {
		return errors.Wrap(err, "failed to create bacon file")
	}

	defer out.Close()

	if _, err := out.ReadFrom(resp.Body); err != nil {
		return errors.Wrap(err, "failed to write bacon file")
	}

	if err := out.Chmod(0755); err != nil {
		return errors.Wrap(err, "failed to set permissions on bacon file")
	}

	fmt.Println("bacon downloaded successfully to:", BaconExe)

	return nil
}

func NewRsaggGenerator() Generator {
	{
		_, err := os.Stat(BaconExe)

		if os.IsNotExist(err) {
			err := Download(BaconUrl, BaconExe)
			if err != nil {
				panic(fmt.Errorf("failed to download bacon: %w", err))
			}
		}
	}

	{
		_, err := os.Stat("bacon.json")
		if os.IsNotExist(err) {
			fmt.Println("bacon.json not found, running optimize")

			cmd := exec.Command("./bacon", "optimize", "--time", "60000")
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout

			if err := cmd.Run(); err != nil {
				panic(fmt.Errorf("failed to run optimize command: %w", err))
			}
		}
	}

	return &RsaggGenerator{}
}

func (g *RsaggGenerator) Generate(prefix string, validityCb func() bool) (ed25519.PrivateKey, error) {
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

	waitCh := make(chan error)
	go func() {
		if err := cmd.Wait(); err != nil {
			waitCh <- err
		} else {
			close(waitCh)
		}
	}()

	ticker := time.NewTicker(1 * time.Second)

loop:
	for {
		select {
		case err := <-waitCh:
			if err != nil {
				return nil, fmt.Errorf("command failed: %w", err)
			}
			break loop
		case <-ticker.C:
			if validityCb != nil && !validityCb() {
				cmd.Process.Kill()
				return nil, fmt.Errorf("order is not valid anymore - skipping generation")
			}
		}
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
