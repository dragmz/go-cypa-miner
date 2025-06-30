package gvm

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	_ "embed"

	"github.com/algorand/go-algorand-sdk/v2/abi"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
)

//go:embed VanityProxy.teal
var lsigTemplateSource string

type Miner struct {
	ac      *algod.Client
	appid   uint64
	subsidy crypto.LogicSigAccount
}

func NewMiner(algodAddr string, algodToken string, appid uint64) (*Miner, error) {
	ac, err := algod.MakeClient(algodAddr, algodToken)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create algod client")
	}

	lsigSource := strings.Replace(string(lsigTemplateSource), "TMPL_VANITY_APP_ID", fmt.Sprintf("%d", appid), 1)

	res, err := ac.TealCompile([]byte(lsigSource)).Do(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "failed to compile LogicSig source code")
	}

	lsigbytes, err := base64.StdEncoding.DecodeString(res.Result)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode LogicSig result")
	}

	lsig, err := crypto.LogicSigAccountFromLogicSig(
		types.LogicSig{
			Logic: lsigbytes,
		}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create LogicSigAccount from LogicSig")
	}

	return &Miner{
		ac:      ac,
		appid:   appid,
		subsidy: lsig,
	}, nil
}

func (m *Miner) Fulfill(sk ed25519.PrivateKey, key []byte, ownerBytes []byte) error {
	owner := types.Address(ownerBytes)

	match_acc, err := crypto.AccountFromPrivateKey(sk)
	if err != nil {
		return errors.Wrap(err, "failed to create account from private key")
	}

	match_signer := transaction.BasicAccountTransactionSigner{
		Account: match_acc,
	}

	sp, err := m.ac.SuggestedParams().Do(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to get suggested params")
	}

	sp.FlatFee = true
	sp.Fee = 0

	atc := transaction.AtomicTransactionComposer{}

	extra_method, err := abi.MethodFromSignature("extra()void")
	if err != nil {
		return errors.Wrap(err, "failed to create extra method from signature")
	}

	for i := range 5 {
		atc.AddMethodCall(transaction.AddMethodCallParams{
			AppID:           m.appid,
			Method:          extra_method,
			Sender:          match_acc.Address,
			SuggestedParams: sp,
			Signer:          match_signer,
			Note:            []byte(fmt.Sprintf("extra opcodes %d", i)),
		})
	}

	subsidy_addr, err := m.subsidy.Address()
	if err != nil {
		return errors.Wrap(err, "failed to get subsidy address")
	}

	pay_tx, err := transaction.MakePaymentTxn(subsidy_addr.String(), match_acc.Address.String(), 100_000, nil, "", sp)
	if err != nil {
		return errors.Wrap(err, "failed to create payment transaction")
	}

	pay_tx.Fee = 10_000

	subsidy_signer := transaction.LogicSigAccountTransactionSigner{
		LogicSigAccount: m.subsidy,
	}

	atc.AddTransaction(transaction.TransactionWithSigner{
		Txn:    pay_tx,
		Signer: subsidy_signer,
	})

	order_box_name := append([]byte("o"), key...)
	fulfillment_box_name := append([]byte("f"), key...)

	bytes_to_sign := append([]byte("AVMV"), key...)

	proof, err := crypto.SignBytes(sk, bytes_to_sign)
	if err != nil {
		return errors.Wrap(err, "failed to sign bytes")
	}

	fulfull_method, err := abi.MethodFromSignature("fulfill(byte[],byte[64],address)void")
	if err != nil {
		return errors.Wrap(err, "failed to create fulfill method from signature")
	}

	atc.AddMethodCall(transaction.AddMethodCallParams{
		AppID:  m.appid,
		Method: fulfull_method,
		MethodArgs: []interface{}{
			key,
			proof,
			match_acc.Address,
		},
		Sender:          match_acc.Address,
		Signer:          match_signer,
		SuggestedParams: sp,
		BoxReferences: []types.AppBoxReference{
			{
				AppID: m.appid,
				Name:  order_box_name,
			},
			{
				AppID: m.appid,
				Name:  fulfillment_box_name,
			},
		},
		ForeignAccounts: []string{
			subsidy_addr.String(),
		},
		RekeyTo: owner,
	})

	_, err = atc.Execute(m.ac, context.Background(), 4)
	if err != nil {
		return errors.Wrap(err, "failed to execute atomic transaction composer")
	}

	return nil
}
