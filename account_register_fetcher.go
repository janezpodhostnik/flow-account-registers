package main

import (
	"context"
	"fmt"
	"strings"

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
	pub struct AccountInfo {
		pub(set) var storageUsed: UInt64
		pub(set) var contracts: [String]
	
		init() {
			self.storageUsed = 0
			self.contracts = []
		}
	}

	pub fun main(address: Address): AccountInfo {
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
				let res <- account.load<@AnyResource>(from: key)
				account.save(<-res, to: key)
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

		let info = AccountInfo()
		info.storageUsed = storage
		info.contracts = account.contracts.names

		return info
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
	storageUsed := value.(cadence.Struct).Fields[0].(cadence.UInt64).ToGoValue().(uint64)
	contracts := make(map[string]struct{})
	for _, contract := range value.(cadence.Struct).Fields[1].(cadence.Array).Values {
		contracts[contract.(cadence.String).ToGoValue().(string)] = struct{}{}
	}

	sb := strings.Builder{}
	for contract := range contracts {
		sb.WriteString("import ")
		sb.WriteString(contract)
		sb.WriteString(" from ")
		sb.WriteString(address.HexWithPrefix())
		sb.WriteString("\n")
	}

	sb.WriteString(`
				transaction {
					prepare(account: AuthAccount) {
						let contracts = account.contracts.names
						for name in contracts {
							account.contracts.get(name: name)
							account.contracts.remove(name: name)
						}
					}
				}
			`)

	tx := flow.NewTransactionBody().
		SetScript([]byte(sb.String())).
		AddAuthorizer(address)

	scriptError, processError = scriptExecutor.RunTransaction(fvm.Transaction(tx, 0))
	if scriptError != nil {
		resultChan <- ErrorResult{
			Error: fmt.Errorf("could not run fake tx, script error: %w", scriptError),
		}
		return
	}
	if processError != nil {
		resultChan <- ErrorResult{
			Error: fmt.Errorf("could not run fake tx, script process error: %w", processError),
		}
		return
	}

	resultChan <- StorageUsedResult{
		StorageUsed:         storageUsed,
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
