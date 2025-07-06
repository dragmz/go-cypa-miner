package gvm

import (
	"fmt"
	"gvm/rac"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
)

type Generator interface {
	Generate(prefix string, validityCb func() bool) (ed25519.PrivateKey, error)
}

type RsaggGenerator struct {
	batch uint64
}

func NewRsaggGenerator(batch uint64) (Generator, error) {
	return &RsaggGenerator{
		batch: batch,
	}, nil
}

func (g *RsaggGenerator) Generate(prefix string, validityCb func() bool) (ed25519.PrivateKey, error) {
	r, err := rac.New()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create RAC instance")
	}

	defer r.Free()

	s, err := r.NewSession(prefix, g.batch)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create RAC session")
	}

	defer s.Free()

	for {
		acc, err := s.Poll()
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate account")
		}

		if acc != nil {
			fmt.Printf("RAC generated account: %s\n", acc.Address)
			return acc.PrivateKey, nil
		}

		if validityCb != nil && !validityCb() {
			return nil, fmt.Errorf("order is not valid anymore - skipping generation")
		}
	}
}
