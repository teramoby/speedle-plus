// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

// largeAck is a valid ack well beyond 64 KiB.
func largeAck(t *testing.T) []byte {
	t.Helper()
	ack := validChannelAck()
	hash := ack.Services[0].HeadStateHash
	ack.Services = nil
	for i := 0; i < 1500; i++ {
		id := fmt.Sprintf("01HF7YAT%018d", i+1)
		ack.Services = append(ack.Services, channelAckService{
			ID: id, Name: fmt.Sprintf("service-%04d", i), Head: 1, OriginHead: 1, Epoch: id, HeadStateHash: hash,
		})
	}
	payload, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) <= 64<<10 {
		t.Fatalf("ack is only %d bytes", len(payload))
	}
	return payload
}

func captureChannelLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	logger := log.StandardLogger()
	out := logger.Out
	logger.SetOutput(&buffer)
	t.Cleanup(func() { logger.SetOutput(out) })
	return &buffer
}

// TestChannelFramesHaveNoSizeLimit applies a long-poll ack beyond the old
// 64 KiB bound, then sends a heartbeat for all of its services, also beyond
// it.
func TestChannelFramesHaveNoSizeLimit(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")
	source := newChannelSource(t, books)
	server := startChannelFixture(t, source)
	machine := newChannelMachine(t)
	channel := newTestChannel(t, newChannelClient(t, server), machine, ChannelOptions{Transport: TransportLongPoll})

	source.configure(func(f *channelSource) { f.pollBody = largeAck(t) })
	if err := channel.pollOnce(context.Background()); err != nil {
		t.Fatalf("large ack: %v", err)
	}
	if got := len(machine.Services()); got != 1500 {
		t.Fatalf("services after a large ack = %d", got)
	}
	source.configure(func(f *channelSource) { f.pollBody = nil })
	if err := channel.pollOnce(context.Background()); err != nil {
		t.Fatalf("large heartbeat: %v", err)
	}
	if size := channel.sentFrameBytes.Load(); size <= 64<<10 {
		t.Fatalf("heartbeat is only %d bytes", size)
	}
}

// TestChannelReportsAFrameTheSourceRejects checks that a frame the source
// refuses as too large surfaces as ErrFrameTooLarge naming its size, on
// both transports, and is logged as an error.
func TestChannelReportsAFrameTheSourceRejects(t *testing.T) {
	books := newFixtureService(testServiceID1, "books")

	t.Run("long poll", func(t *testing.T) {
		logged := captureChannelLog(t)
		source := newChannelSource(t, books)
		source.configure(func(f *channelSource) { f.pollStatus = http.StatusRequestEntityTooLarge })
		server := startChannelFixture(t, source)
		channel := newTestChannel(t, newChannelClient(t, server), newChannelMachine(t), ChannelOptions{Transport: TransportLongPoll})
		err := channel.pollOnce(context.Background())
		size := fmt.Sprintf("%d", channel.sentFrameBytes.Load())
		if !errors.Is(err, ErrFrameTooLarge) || !strings.Contains(err.Error(), size) {
			t.Fatalf("pollOnce = %v", err)
		}
		if text := logged.String(); !strings.Contains(text, "level=error") || !strings.Contains(text, size+"-byte") {
			t.Fatalf("log = %q", text)
		}
	})

	t.Run("websocket", func(t *testing.T) {
		logged := captureChannelLog(t)
		upgrader := websocket.Upgrader{}
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_, _, _ = conn.ReadMessage()
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseMessageTooBig, "frame too large"))
			_, _, _ = conn.ReadMessage()
		}))
		server.Config.ErrorLog = nil
		server.StartTLS()
		t.Cleanup(server.Close)
		channel := newTestChannel(t, newChannelClient(t, server), newChannelMachine(t), ChannelOptions{Transport: TransportWebSocket})
		outcome := channel.runWebSocket(context.Background())
		size := fmt.Sprintf("%d", channel.sentFrameBytes.Load())
		if !errors.Is(outcome.err, ErrFrameTooLarge) || !strings.Contains(outcome.err.Error(), size) || size == "0" {
			t.Fatalf("outcome = %+v", outcome)
		}
		if text := logged.String(); !strings.Contains(text, size+"-byte") {
			t.Fatalf("log = %q", text)
		}
	})
}
