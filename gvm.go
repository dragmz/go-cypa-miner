package gvm

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"iter"
	"strings"
	"time"

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

type OrderHeader struct {
	Key    []byte
	Length uint64
}

type Order struct {
	Key []byte

	Id     uint64
	Owner  types.Address
	Prefix []byte
	Mask   []byte
	Text   string

	Asset    uint64
	Amount   uint64
	Validity uint64
}

var orderType abi.Type

func init() {
	var err error
	orderType, err = abi.TypeOf("(uint64,address,byte[],byte[],byte[],uint64,uint64,uint64)")
	if err != nil {
		panic(errors.Wrap(err, "failed to create order type"))
	}
}

func convertToByteSlice(slice []interface{}) ([]byte, error) {
	result := make([]byte, len(slice))
	for i, v := range slice {
		if b, ok := v.(byte); ok {
			result[i] = b
		} else {
			return nil, errors.New("failed to convert value to byte")
		}
	}
	return result, nil
}

func (m *Miner) List(maxLength int) iter.Seq[OrderHeader] {
	return func(yield func(OrderHeader) bool) {
		response, err := m.ac.GetApplicationBoxes(m.appid).Do(context.Background())
		if err != nil {
			return
		}

		for _, item := range response.Boxes {
			if len(item.Name) == 0 || item.Name[0] != 'o' {
				continue // Skip boxes that are not orders
			}

			extras_index := 1 + 32 + 8
			length_index := extras_index

			length := binary.BigEndian.Uint64(item.Name[length_index:(length_index + 8)])
			if maxLength > 0 && length > uint64(maxLength) {
				continue // Skip orders that exceed the max length
			}

			validity_index := length_index + 8
			validity := binary.BigEndian.Uint64(item.Name[validity_index:(validity_index + 8)])

			now := time.Now().Unix()
			if validity > 0 && validity < uint64(now) {
				continue // Skip orders that are no longer valid
			}

			key := item.Name[1:]
			header := OrderHeader{
				Key:    key,
				Length: length,
			}

			if !yield(header) {
				return // Stop iteration if yield returns false
			}
		}
	}
}

func (m *Miner) Read(key []byte) (*Order, error) {
	box_name := append([]byte("o"), key...)

	box, err := m.ac.GetApplicationBoxByName(m.appid, box_name).Do(context.Background())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get application box by name %s", box_name)
	}

	values, err := orderType.Decode(box.Value)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode order box value for %s", box_name)
	}

	valuesSlice, ok := values.([]interface{})
	if !ok {
		return nil, errors.New("failed to cast order values to []interface{}")
	}

	if len(valuesSlice) != 8 {
		return nil, errors.Errorf("expected 8 values in order, got %d", len(valuesSlice))
	}

	var order Order

	order.Key = key
	order.Id = valuesSlice[0].(uint64)
	order.Owner = types.Address(valuesSlice[1].([]byte))

	order.Prefix, err = convertToByteSlice(valuesSlice[2].([]interface{}))
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert prefixSlice to byte slice")
	}

	order.Mask, err = convertToByteSlice(valuesSlice[3].([]interface{}))
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert maskSlice to byte slice")
	}

	text, err := convertToByteSlice(valuesSlice[4].([]interface{}))
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert textSlice to byte slice")
	}
	order.Text = string(text)

	order.Asset = valuesSlice[5].(uint64)
	order.Amount = valuesSlice[6].(uint64)
	order.Validity = valuesSlice[7].(uint64)

	return &order, nil
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

func (m *Miner) Fulfill(sk ed25519.PrivateKey, key []byte, owner types.Address, rewards types.Address) error {
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
			rewards,
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
			rewards.String(),
		},
		RekeyTo: owner,
	})

	_, err = atc.Execute(m.ac, context.Background(), 4)
	if err != nil {
		return errors.Wrap(err, "failed to execute atomic transaction composer")
	}

	return nil
}
