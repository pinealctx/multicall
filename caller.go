package multicall

import (
	"context"
	"fmt"
	"github.com/pinealctx/multicall/contract"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// DefaultAddress is the same for all chains (Multicall3).
// Taken from https://github.com/mds1/multicall
const DefaultAddress = "0xcA11bde05977b3631167028862bE2a173976CA11"

type Options struct {
	ctx             context.Context
	rpcURL          string
	client          bind.ContractCaller
	contractAddress string
}

type Option func(*Options)

func WithRPCURL(url string) Option {
	return func(o *Options) {
		o.rpcURL = url
	}
}

func WithClient(client bind.ContractCaller) Option {
	return func(o *Options) {
		o.client = client
	}
}

func WithContractAddress(address string) Option {
	return func(o *Options) {
		o.contractAddress = address
	}
}

// Caller makes multicalls.
type Caller struct {
	contract contract.Interface
}

func New(fns ...Option) (*Caller, error) {
	opts := &Options{
		contractAddress: DefaultAddress,
	}

	for _, fn := range fns {
		fn(opts)
	}

	var err error
	if opts.client == nil {
		if opts.rpcURL == "" {
			return nil, fmt.Errorf("rpcURL is required")
		}
		if opts.ctx == nil {
			opts.client, err = ethclient.Dial(opts.rpcURL)
		} else {
			opts.client, err = ethclient.DialContext(opts.ctx, opts.rpcURL)
		}
		if err != nil {
			return nil, err
		}
	}

	c, err := contract.NewMulticallCaller(common.HexToAddress(opts.contractAddress), opts.client)
	if err != nil {
		return nil, err
	}
	return &Caller{contract: c}, nil

}

// Call makes multicalls.
func (caller *Caller) Call(opts *bind.CallOpts, calls ...*Call) ([]*Call, error) {
	var multiCalls []contract.Multicall3Call3

	for i, call := range calls {
		b, err := call.Pack()
		if err != nil {
			return calls, fmt.Errorf("failed to pack call inputs at index [%d]: %v", i, err)
		}
		multiCalls = append(multiCalls, contract.Multicall3Call3{
			Target:       call.Contract.address,
			AllowFailure: call.CanFail,
			CallData:     b,
		})
	}

	results, err := caller.contract.Aggregate3(opts, multiCalls)
	if err != nil {
		return calls, fmt.Errorf("multicall failed: %v", err)
	}

	for i, result := range results {
		call := calls[i] // index always matches
		call.Failed = !result.Success
		if call.Failed {
			continue
		}
		if err := call.Unpack(result.ReturnData); err != nil {
			return calls, fmt.Errorf("failed to unpack call outputs at index [%d]: %v", i, err)
		}
	}

	return calls, nil
}

// CallChunked makes multiple multicalls by chunking given calls.
// Cooldown is helpful for sleeping between chunks and avoiding rate limits.
func (caller *Caller) CallChunked(opts *bind.CallOpts, chunkSize int, cooldown time.Duration, calls ...*Call) ([]*Call, error) {
	var allCalls []*Call
	for i, chunk := range chunkInputs(chunkSize, calls) {
		if i > 0 && cooldown > 0 {
			time.Sleep(cooldown)
		}

		ck, err := caller.Call(opts, chunk...)
		if err != nil {
			return calls, fmt.Errorf("call chunk [%d] failed: %v", i, err)
		}
		allCalls = append(allCalls, ck...)
	}
	return allCalls, nil
}

func chunkInputs[T any](chunkSize int, inputs []T) (chunks [][]T) {
	if len(inputs) == 0 {
		return
	}

	if chunkSize <= 0 || len(inputs) < 2 || chunkSize > len(inputs) {
		return [][]T{inputs}
	}

	lastChunkSize := len(inputs) % chunkSize

	chunkCount := len(inputs) / chunkSize

	for i := 0; i < chunkCount; i++ {
		start := i * chunkSize
		end := start + chunkSize
		chunks = append(chunks, inputs[start:end])
	}

	if lastChunkSize > 0 {
		start := chunkCount * chunkSize
		end := start + lastChunkSize
		chunks = append(chunks, inputs[start:end])
	}

	return
}
