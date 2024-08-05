package multicall

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type ContractOptions struct {
	abi     *abi.ABI
	address common.Address
	err     error
}

type ContractOption func(*ContractOptions)

func WithABI(abi *abi.ABI) ContractOption {
	return func(o *ContractOptions) {
		o.abi = abi
	}
}

func WithABIMeta(abiMeta *bind.MetaData) ContractOption {
	return func(o *ContractOptions) {
		o.abi, o.err = abiMeta.GetAbi()
	}
}

func WithABIJSON(abiJSON string) ContractOption {
	return func(o *ContractOptions) {
		o.abi, o.err = ParseABI(abiJSON)
	}
}

func WithAddress(address common.Address) ContractOption {
	return func(o *ContractOptions) {
		o.address = address
	}
}

// Contract wraps the parsed ABI and acts as a call factory.
type Contract struct {
	abi     *abi.ABI
	address common.Address
}

func NewContract(fns ...ContractOption) (*Contract, error) {
	opts := &ContractOptions{}
	for _, fn := range fns {
		fn(opts)
	}
	if opts.err != nil {
		return nil, opts.err
	}
	if opts.abi == nil {
		return nil, errors.New("abi is required")
	}
	return &Contract{
		abi:     opts.abi,
		address: opts.address,
	}, nil
}

func NewContract2(abiMeta *bind.MetaData, address common.Address) (*Contract, error) {
	return NewContract(WithABIMeta(abiMeta), WithAddress(address))
}

// ParseABI parses raw ABI JSON.
func ParseABI(rawJson string) (*abi.ABI, error) {
	parsed, err := abi.JSON(bytes.NewBufferString(rawJson))
	if err != nil {
		return nil, fmt.Errorf("failed to parse abi: %v", err)
	}
	return &parsed, nil
}

// Call wraps a multicall call.
type Call struct {
	CallName string
	Contract *Contract
	Method   string
	Inputs   []any
	Outputs  any
	CanFail  bool
	Failed   bool
}

// NewCall creates a new call using given inputs.
// Outputs type is the expected output struct to unpack and set values in.
func (contract *Contract) NewCall(outputs any, methodName string, inputs ...any) *Call {
	return &Call{
		Contract: contract,
		Method:   methodName,
		Inputs:   inputs,
		Outputs:  outputs,
	}
}

// Name sets a name for the call.
func (call *Call) Name(name string) *Call {
	call.CallName = name
	return call
}

// AllowFailure sets if the call is allowed to fail. This helps avoiding a revert
// when one of the calls in the array fails.
func (call *Call) AllowFailure() *Call {
	call.CanFail = true
	return call
}

// Unpack unpacks and converts EVM outputs and sets struct fields.
func (call *Call) Unpack(b []byte) error {
	t := reflect.ValueOf(call.Outputs)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return errors.New("outputs type is not a struct")
	}

	out, err := call.Contract.abi.Unpack(call.Method, b)
	if err != nil {
		return fmt.Errorf("failed to unpack '%s' outputs: %v", call.Method, err)
	}

	fieldCount := t.NumField()
	for i := 0; i < fieldCount; i++ {
		field := t.Field(i)
		converted := abi.ConvertType(out[i], reflect.New(field.Type()).Interface())
		field.Set(reflect.ValueOf(converted).Elem())
	}

	return nil
}

// Pack converts and packs EVM inputs.
func (call *Call) Pack() ([]byte, error) {
	b, err := call.Contract.abi.Pack(call.Method, call.Inputs...)
	if err != nil {
		return nil, fmt.Errorf("failed to pack '%s' inputs: %v", call.Method, err)
	}
	return b, nil
}
