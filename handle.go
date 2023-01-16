package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/onflow/flow-archive/api/archive"
	"github.com/onflow/flow-go/model/flow"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
)

type Message struct {
	Type string `json:"type"`
}

type RegisterMessage struct {
	Message
	RegisterHeader string `json:"register_header"`
	Register       string `json:"register"`
}

func NewRegisterMessage(
	registerHeader string,
	register string,
) RegisterMessage {
	return RegisterMessage{
		Message: Message{
			Type: "register",
		},
		RegisterHeader: registerHeader,
		Register:       register,
	}
}

type StorageMessage struct {
	Message
	StorageMessage string `json:"storage_message"`
}

func NewStorageMessage(
	storageMessage string,
) StorageMessage {
	return StorageMessage{
		Message: Message{
			Type: "storage",
		},
		StorageMessage: storageMessage,
	}
}

type BlockHeightMessage struct {
	Message
	BlockHeight string `json:"block_height"`
}

func NewBlockHeightMessage(
	blockHeight string,
) BlockHeightMessage {
	return BlockHeightMessage{
		Message: Message{
			Type: "block",
		},
		BlockHeight: blockHeight,
	}
}

type ErrorMessage struct {
	Message
	Error string `json:"error"`
}

func NewErrorMessage(
	err error,
) ErrorMessage {
	return ErrorMessage{
		Message: Message{
			Type: "error",
		},
		Error: err.Error(),
	}
}

func handle(
	ctx context.Context,
	fetcher *AccountRegisterFetcher,
	writer io.Writer,
	address flow.Address) error {

	for result := range fetcher.Fetch(ctx, address) {
		var encoded []byte
		var err error

		switch result := result.(type) {
		case AccountRegisterResult:
			register := hex.Dump(result.Register.Value)
			decodedKey := ""
			if len(result.Register.Key) > 0 && result.Register.Key[0] == '$' {
				decodedKey = "|$" + hex.EncodeToString([]byte(result.Register.Key[1:])) + "|"
			} else {
				decodedKey = "|" + result.Register.Key + "|"
			}
			registerHeader := fmt.Sprintf("%x  %s", result.Register.Key, decodedKey)

			message := NewRegisterMessage(
				registerHeader,
				register,
			)

			encoded, err = json.Marshal(message)
			if err != nil {
				return err
			}
		case StorageUsedResult:
			s := ""
			if result.StorageUsed < result.ComputedStorageUsed {
				s = fmt.Sprintf(
					"Some bytes are extra!? Expected %d bytes, but got %d bytes of registers.\n"+
						"Strange, better report this on the flow discord, if you are consistently getting this result for an account.",
					result.StorageUsed,
					result.ComputedStorageUsed)
			} else if result.StorageUsed > result.ComputedStorageUsed {
				s = fmt.Sprintf(
					"Some bytes are missing. Expected %d bytes, but only %d bytes of registers are available.\n"+
						"The script use to fetch registers was not thorough enough, sorry.",
					result.StorageUsed,
					result.ComputedStorageUsed)
			} else {
				s = fmt.Sprintf(
					"That is everything (%d bytes)",
					result.StorageUsed)
			}

			message := NewStorageMessage(s)
			encoded, err = json.Marshal(message)
			if err != nil {
				return err
			}
		case BlockHeightResult:
			s := fmt.Sprintf("@ block height: %d", result.BlockHeight)
			message := NewBlockHeightMessage(s)
			encoded, err = json.Marshal(message)
			if err != nil {
				return err
			}
		case ErrorResult:
			return result.Error
		default:
			return fmt.Errorf("unexpected result type: %T", result)
		}

		_, err = writer.Write(encoded)
		if err != nil {
			return err
		}
	}
	return nil
}

type clientWithConnection struct {
	archive.APIClient
	*grpc.ClientConn
}

// getClient returns a client to the Archive API.
func getClient(archiveHost string, log zerolog.Logger) (clientWithConnection, error) {
	conn, err := grpc.Dial(
		archiveHost,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Error().
			Err(err).
			Str("host", archiveHost).
			Msg("Could not connect to server.")
		return clientWithConnection{}, err
	}
	client := archive.NewAPIClient(conn)

	return clientWithConnection{
		APIClient:  client,
		ClientConn: conn,
	}, nil
}
