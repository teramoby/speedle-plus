// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
)

// Pin the wire key sets independently of the Go structs in channel_messages.go.
// In particular, transport belongs only to the display-only host block.
func TestChannelFrameJSONKeys(t *testing.T) {
	f := newFixture(t)
	machine := newChannelMachine(t)
	mustAck(t, machine, ackWith(testAckService(testServiceID1, "books", 2)))
	channel := newTestChannel(t, f.newClient(nil), machine, ChannelOptions{
		HostInfo: func(time.Time) HostInfo { return HostInfo{Hostname: "edge-1", IPs: []string{}} },
	})
	hello := channel.helloMessage(TransportWebSocket)
	websocket := channel.heartbeatMessage(TransportWebSocket)
	poll := channel.heartbeatMessage(TransportLongPoll)
	channel.downstreamCount = func() uint64 { return 0 }
	relay := channel.heartbeatMessage(TransportLongPoll)
	ack := validChannelAck()
	ack.Commands = []channelCommand{{ID: "01HF7YAT000000000000000301", Action: commandActionResync,
		ServiceID: testServiceID1, ExpiresAt: 1762500060000}}

	const heartbeatKeys = "agentVersion fallback instanceId liveness seq services type"
	const hostKeys = "arch container goVersion hostname ips os osVersion pid startedAt transport"
	const serviceKeys = "id name revision state"
	for _, tc := range []struct {
		name    string
		frame   any
		keys    string
		service string
	}{
		{"hello", hello, "agentVersion host instanceId services type", serviceKeys},
		{"heartbeat websocket", websocket, heartbeatKeys, serviceKeys + " lagSeconds"},
		{"heartbeat longpoll", poll, heartbeatKeys + " host", serviceKeys + " lagSeconds"},
		{"heartbeat relay", relay, heartbeatKeys + " host downstreamCount", serviceKeys + " lagSeconds"},
		{"ack", ack, "commands heartbeatInterval serverTime services servicesVersion staleAfter stopAfter type",
			"epoch head headStateHash id name originHead"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(tc.frame)
			if err != nil {
				t.Fatal(err)
			}
			var frame map[string]json.RawMessage
			if err := json.Unmarshal(payload, &frame); err != nil {
				t.Fatal(err)
			}
			assertFrameJSONKeys(t, payload, tc.keys)
			var services []json.RawMessage
			if err := json.Unmarshal(frame["services"], &services); err != nil || len(services) != 1 {
				t.Fatalf("services = %s, error = %v", frame["services"], err)
			}
			assertFrameJSONKeys(t, services[0], tc.service)
			if host, ok := frame["host"]; ok {
				assertFrameJSONKeys(t, host, hostKeys)
				var sent HostInfo
				if err := json.Unmarshal(host, &sent); err != nil {
					t.Fatal(err)
				}
				want := string(TransportLongPoll)
				if tc.name == "hello" {
					want = string(TransportWebSocket)
				}
				if sent.Transport != want {
					t.Errorf("host.transport = %q, want %q", sent.Transport, want)
				}
			}
			if fallback, ok := frame["fallback"]; ok {
				assertFrameJSONKeys(t, fallback, "active routedCalls services since")
			}
			if commands, ok := frame["commands"]; ok {
				var entries []json.RawMessage
				if err := json.Unmarshal(commands, &entries); err != nil || len(entries) != 1 {
					t.Fatalf("commands = %s, error = %v", commands, err)
				}
				assertFrameJSONKeys(t, entries[0], "action expiresAt id serviceId")
				if _, err := decodeServerMessage(payload); err != nil {
					t.Fatalf("decode golden ack: %v", err)
				}
			}
		})
	}
}

func assertFrameJSONKeys(t *testing.T, payload []byte, golden string) {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatal(err)
	}
	got, want := slices.Sorted(maps.Keys(object)), strings.Fields(golden)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("JSON keys = %v, want %v; frame: %s", got, want, payload)
	}
}
