// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func validChannelAck() channelAck {
	return channelAck{
		Type:              channelTypeAck,
		ServicesVersion:   1,
		ServerTime:        1762500000000,
		HeartbeatInterval: "30s",
		StaleAfter:        "60s",
		StopAfter:         "120s",
		Services: []channelAckService{{
			ID:            testServiceID1,
			Name:          "books",
			Head:          2,
			OriginHead:    2,
			Epoch:         testServiceID1,
			HeadStateHash: StateHash(Snapshot{Name: "books", Policies: []Policy{}, RolePolicies: []Policy{}}),
		}},
		Commands: []channelCommand{},
	}
}

func TestDecodeServerAck(t *testing.T) {
	ack := validChannelAck()
	payload, err := json.Marshal(ack)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	message, err := decodeServerMessage(payload)
	if err != nil {
		t.Fatalf("decodeServerMessage: %v", err)
	}
	decoded, ok := message.(*channelAck)
	if !ok {
		t.Fatalf("message is %T, want *channelAck", message)
	}
	if len(decoded.Services) != 1 || decoded.Services[0].Head != 2 {
		t.Fatalf("decoded ack = %+v", decoded)
	}
}

func TestDecodeServerMessageRejects(t *testing.T) {
	cases := []struct {
		name    string
		payload func() []byte
	}{
		{
			name: "unknown message type",
			payload: func() []byte {
				return []byte(`{"type":"surprise"}`)
			},
		},
		{
			name: "unknown field",
			payload: func() []byte {
				ack := validChannelAck()
				payload, _ := json.Marshal(ack)
				var object map[string]any
				_ = json.Unmarshal(payload, &object)
				object["surprise"] = true
				payload, _ = json.Marshal(object)
				return payload
			},
		},
		{
			name: "bad duration",
			payload: func() []byte {
				ack := validChannelAck()
				ack.StaleAfter = "1h"
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "origin head ahead of head",
			payload: func() []byte {
				ack := validChannelAck()
				ack.Services[0].OriginHead = 3
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "duplicate service",
			payload: func() []byte {
				ack := validChannelAck()
				ack.Services = append(ack.Services, ack.Services[0])
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "zero hash",
			payload: func() []byte {
				ack := validChannelAck()
				ack.Services[0].HeadStateHash = Hash{}
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "unknown command action",
			payload: func() []byte {
				ack := validChannelAck()
				ack.Commands = []channelCommand{{
					ID:        "01HF7YAT000000000000000301",
					Action:    "reload",
					ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				}}
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "set_source is not a command",
			payload: func() []byte {
				ack := validChannelAck()
				ack.Commands = []channelCommand{{
					ID:        "01HF7YAT000000000000000301",
					Action:    "set_source",
					Params:    json.RawMessage(`{"url":"https://proxy.example"}`),
					ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				}}
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "notify without a valid service",
			payload: func() []byte {
				return []byte(`{"type":"notify","serviceId":"not-a-service","head":1}`)
			},
		},
		{
			name: "notify with an out-of-range head",
			payload: func() []byte {
				return []byte(`{"type":"notify","serviceId":"` + testServiceID1 + `","head":9007199254740992}`)
			},
		},
		{
			name: "notify with an unknown field",
			payload: func() []byte {
				return []byte(`{"type":"notify","serviceId":"` + testServiceID1 + `","head":1,"surprise":true}`)
			},
		},
		{
			name: "malformed json",
			payload: func() []byte {
				return []byte(`{`)
			},
		},
		{
			name: "zero services version",
			payload: func() []byte {
				ack := validChannelAck()
				ack.ServicesVersion = 0
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "duplicate services",
			payload: func() []byte {
				ack := validChannelAck()
				for index := 0; index < 10; index++ {
					ack.Services = append(ack.Services, ack.Services[0])
				}
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "server time out of range",
			payload: func() []byte {
				ack := validChannelAck()
				ack.ServerTime = maxSafeInteger + 1
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "command with an invalid id",
			payload: func() []byte {
				ack := validChannelAck()
				ack.Commands = []channelCommand{{
					ID:        "not-a-ulid",
					Action:    "resync",
					ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				}}
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "command with a negative expiry",
			payload: func() []byte {
				ack := validChannelAck()
				ack.Commands = []channelCommand{{
					ID:        "01HF7YAT000000000000000301",
					Action:    "resync",
					ExpiresAt: -1,
				}}
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
		{
			name: "resync with params",
			payload: func() []byte {
				ack := validChannelAck()
				ack.Commands = []channelCommand{{
					ID:        "01HF7YAT000000000000000301",
					Action:    "resync",
					Params:    json.RawMessage(`{"url":"https://proxy.example"}`),
					ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
				}}
				payload, _ := json.Marshal(ack)
				return payload
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := decodeServerMessage(testCase.payload()); !errors.Is(err, ErrProtocol) {
				t.Fatalf("decodeServerMessage = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestParseChannelDuration(t *testing.T) {
	for _, value := range []string{"500ms", "30s", "5m"} {
		if _, err := parseChannelDuration(value); err != nil {
			t.Errorf("parseChannelDuration(%q) = %v", value, err)
		}
	}
	for _, value := range []string{"", "0s", "1h", "-1s", "1.5s", " 30s", "30S"} {
		if _, err := parseChannelDuration(value); !errors.Is(err, ErrProtocol) {
			t.Errorf("parseChannelDuration(%q) = %v, want ErrProtocol", value, err)
		}
	}
}
