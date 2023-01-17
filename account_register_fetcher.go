package main

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/onflow/cadence"
	"github.com/onflow/cadence/encoding/json"
	"github.com/onflow/flow-archive/api/archive"
	"github.com/onflow/flow-go/engine/execution/state"
	"github.com/onflow/flow-go/fvm"
	"github.com/onflow/flow-go/ledger/common/pathfinder"
	"github.com/onflow/flow-go/ledger/complete"
	"github.com/onflow/flow-go/model/flow"
)

type AccountRegisterFetcher struct {
	chain  flow.Chain
	client archive.APIClient
	logger zerolog.Logger
}

func NewAccountRegisterFetcher(
	chain flow.Chain,
	client archive.APIClient,
	logger zerolog.Logger,
) *AccountRegisterFetcher {
	return &AccountRegisterFetcher{
		chain:  chain,
		client: client,
		logger: logger,
	}
}

func (r *AccountRegisterFetcher) Fetch(ctx context.Context, address flow.Address) <-chan FetcherResult {
	resultChan := make(chan FetcherResult)
	go r.fetch(ctx, address, resultChan)
	return resultChan
}

func (r *AccountRegisterFetcher) fetch(ctx context.Context, address flow.Address, resultChan chan<- FetcherResult) {
	defer close(resultChan)

	resp, err := r.client.GetLast(ctx, &archive.GetLastRequest{})
	if err != nil {
		resultChan <- ErrorResult{
			Error: fmt.Errorf("could not fetch a recent blockheight: %w", err),
		}
		return
	}
	blockHeight := resp.GetHeight()
	resultChan <- BlockHeightResult{
		BlockHeight: blockHeight,
	}

	sumUsed := uint64(0)
	cache := make(map[flow.RegisterID]flow.RegisterValue)
	readFunc := func(owner string, key string) (flow.RegisterValue, error) {
		regID := flow.RegisterID{Key: key, Owner: owner}
		if value, ok := cache[regID]; ok {
			return value, nil
		}

		ledgerKey := state.RegisterIDToKey(flow.RegisterID{Key: key, Owner: owner})
		ledgerPath, err := pathfinder.KeyToPath(ledgerKey, complete.DefaultPathFinderVersion)
		if err != nil {
			return nil, err
		}

		resp, err := r.client.GetRegisterValues(ctx, &archive.GetRegisterValuesRequest{
			Height: blockHeight,
			Paths:  [][]byte{ledgerPath[:]},
		})
		if err != nil {
			return nil, err
		}
		cache[regID] = resp.Values[0]

		if flow.BytesToAddress([]byte(owner)) == address && len(resp.Values[0]) > 0 {
			sumUsed += uint64(len(resp.Values[0])) + uint64(len(key)) + uint64(len(owner)) + 4

			resultChan <- AccountRegisterResult{
				Register: AccountRegister{
					Address: address,
					Key:     key,
					Value:   resp.Values[0],
				},
			}
		}

		return resp.Values[0], nil
	}

	view := NewRemoteView(readFunc)
	scriptExecutor := NewRemoteDebugger(
		view,
		r.chain,
		r.logger,
	)

	script := fvm.Script([]byte(`
	pub fun main(address: Address): UInt64 {
		let account = getAuthAccount(address)
        let storage = account.storageUsed
		let paths: {StoragePath:Bool} = {}
		account.forEachStored(fun (path: StoragePath, type: Type): Bool {
			paths[path] = type.isSubtype(of: Type<@AnyResource>())
			return true
		})
		paths.forEachKey(fun (key: StoragePath): Bool {
			let value = paths[key]??false
			if value {
				destroy account.load<@AnyResource>(from: key)
			} else {
				account.copy<AnyStruct>(from: key)
			}
			return true
		})
		let capabilityPaths: [CapabilityPath] = []
		account.forEachPublic(fun (path: PublicPath, type: Type): Bool {
			capabilityPaths.append(path)
			return true
		})
		account.forEachPrivate(fun (path: PrivatePath, type: Type): Bool {
			capabilityPaths.append(path)
			return true
		})
		for capabilityPath in capabilityPaths {
			account.unlink(capabilityPath)
		}
		account.keys.forEach(fun( key: AccountKey): Bool{
			return true
		})
        for name in account.contracts.names {
			account.contracts.get(name: name)
        }
		return storage
	  }
	`)).WithArguments(json.MustEncode(cadence.NewAddress(address)))

	value, scriptError, processError := scriptExecutor.RunScript(script)
	if scriptError != nil {
		resultChan <- ErrorResult{
			Error: fmt.Errorf("could not run script, script error: %w", scriptError),
		}
		return
	}
	if processError != nil {
		resultChan <- ErrorResult{
			Error: fmt.Errorf("could not run script, script process error: %w", processError),
		}
		return
	}
	resultChan <- StorageUsedResult{
		StorageUsed:         value.(cadence.UInt64).ToGoValue().(uint64),
		ComputedStorageUsed: sumUsed,
	}
}

type FetcherResult interface {
	isFetcherResult()
}

type fetcherResult struct{}

func (r fetcherResult) isFetcherResult() {}

var _ FetcherResult = fetcherResult{}

type AccountRegister struct {
	Address flow.Address
	Key     string
	Value   []byte
}

type AccountRegisterResult struct {
	fetcherResult
	Register AccountRegister
}

type ErrorResult struct {
	fetcherResult
	Error error
}

type StorageUsedResult struct {
	fetcherResult
	StorageUsed         uint64
	ComputedStorageUsed uint64
}

type BlockHeightResult struct {
	fetcherResult
	BlockHeight uint64
}
