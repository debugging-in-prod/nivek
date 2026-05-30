package overseer

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// SubmitFunc is the executor-side dispatch function for incoming Commands.
type SubmitFunc func(action wire.Action) error

// Server handles incoming WebSocket connections, verifies envelopes, and
// dispatches Commands to the provided SubmitFunc.
type Server struct {
	hmacKey []byte
	submit  SubmitFunc
}

func NewServer(hmacKey []byte, submit SubmitFunc) *Server {
	return &Server{hmacKey: hmacKey, submit: submit}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // local LAN; HMAC provides authn
	})
	if err != nil {
		log.Printf("[overseer-server] accept failed: %v", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "server closing")

	log.Printf("[overseer-server] connection from %s", r.RemoteAddr)

	var (
		lastReqSeq uint64
		ackSeq     uint64
	)

	for {
		readCtx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		_, buf, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("[overseer-server] read err from %s: %v", r.RemoteAddr, err)
			}
			return
		}

		env, err := UnmarshalEnvelope(buf, s.hmacKey)
		if err != nil {
			log.Printf("[overseer-server] envelope error from %s: %v", r.RemoteAddr, err)
			conn.Close(websocket.StatusPolicyViolation, "envelope verification failed")
			return
		}
		if env.Seq <= lastReqSeq {
			log.Printf("[overseer-server] replay: seq %d <= last %d", env.Seq, lastReqSeq)
			conn.Close(websocket.StatusPolicyViolation, "replay detected")
			return
		}
		lastReqSeq = env.Seq

		if env.Type != wire.EnvelopeTypeCommand {
			log.Printf("[overseer-server] ignoring envelope type %s", env.Type)
			continue
		}

		var cmd wire.Command
		if err := json.Unmarshal(env.Data, &cmd); err != nil {
			log.Printf("[overseer-server] bad Command JSON: %v", err)
			continue
		}

		execErr := s.submit(cmd.Action)
		executed := wire.ExecutedCmd{
			CommandID:    cmd.ID,
			RawText:      cmd.RawText,
			Action:       cmd.Action,
			FromUsername: cmd.From.Username,
			ExecutedAt:   time.Now().UTC(),
			ChatToExecMs: time.Since(cmd.ReceivedAt).Milliseconds(),
			Result:       wire.ExecResultOK,
		}
		if execErr != nil {
			executed.Result = wire.ExecResultError
			executed.ErrorMessage = execErr.Error()
		}

		ackSeq++
		ackEnv, err := MarshalEnvelope(wire.EnvelopeTypeExecutedCmd, ackSeq, executed, s.hmacKey)
		if err != nil {
			log.Printf("[overseer-server] marshal ack: %v", err)
			continue
		}

		writeCtx, writeCancel := context.WithTimeout(r.Context(), 10*time.Second)
		err = conn.Write(writeCtx, websocket.MessageText, ackEnv)
		writeCancel()
		if err != nil {
			log.Printf("[overseer-server] write ack failed: %v", err)
			return
		}
	}
}
