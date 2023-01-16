package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/onflow/flow-go/model/flow"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
)

type TemplateData struct {
	Address string
}

func main() {
	log.Logger = log.
		Output(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(zerolog.InfoLevel)

	tmpl := template.Must(template.ParseFiles("index.html"))
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		currentGorillaConn, err := upgrader.Upgrade(w, r, w.Header())
		if err != nil {
			http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
		}

		addressString := r.URL.Query().Get("address")
		address := flow.HexToAddress(addressString)

		writer := &SocketWriter{conn: currentGorillaConn}
		defer func() {
			err := currentGorillaConn.Close()
			if err != nil {
				log.Error().Err(err).Msg("failed to close websocket connection")
			}
		}()

		err = func(w io.Writer, address flow.Address) error {
			client, err := getClient("dps-001.mainnet20.nodes.onflow.org:9000", log.Logger)
			if err != nil {
				return err
			}

			fetcher := NewAccountRegisterFetcher(
				flow.Mainnet.Chain(),
				client,
				log.Logger,
			)

			err = handle(r.Context(), fetcher, w, address)
			return err
		}(writer, address)
		if err != nil {
			log.Error().Err(err).Msg("error handling request")
			msg := NewErrorMessage(err)
			enc, err := json.Marshal(msg)
			if err != nil {
				_, _ = writer.Write(enc)
			}
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.Split(r.URL.Path, "/")
		if len(p) != 2 {
			http.Redirect(w, r, "/static/help.html", http.StatusSeeOther)
			return
		}

		// todo validate address

		address := flow.HexToAddress(p[1])

		tmpl.Execute(w, TemplateData{Address: address.HexWithPrefix()})
	})
	fmt.Println("Server starting at :8080")
	http.ListenAndServe(":8080", nil)
}

type SocketWriter struct {
	conn *websocket.Conn
}

func (s *SocketWriter) Write(p []byte) (int, error) {
	err := s.conn.WriteMessage(websocket.TextMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

var _ io.Writer = (*SocketWriter)(nil)
