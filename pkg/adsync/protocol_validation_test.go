// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func validTestSignature() Signature {
	return Signature{
		KeyID: "test-key",
		Alg:   "Ed25519",
		Value: base64.StdEncoding.EncodeToString(make([]byte, 64)),
	}
}

func validTestSnapshotResponse() SnapshotResponse {
	snapshot := Snapshot{Name: "books", Policies: []Policy{}, RolePolicies: []Policy{}}
	hash := StateHash(snapshot)
	return SnapshotResponse{
		ServiceID:  testServiceID1,
		SourceID:   testServiceID1,
		Epoch:      testServiceID1,
		Revision:   1,
		StateHash:  hash,
		CommitHash: hash,
		Signature:  validTestSignature(),
		Snapshot:   snapshot,
	}
}

func validTestService() Service {
	return Service{
		ID:            testServiceID1,
		Name:          "books",
		Epoch:         testServiceID1,
		Revision:      1,
		HeadStateHash: StateHash(Snapshot{Name: "books"}),
	}
}

func TestServiceValidation(t *testing.T) {
	valid := validTestService()
	cases := []struct {
		name   string
		mutate func(*Service)
	}{
		{"invalid id", func(service *Service) { service.ID = "bad" }},
		{"invalid name", func(service *Service) { service.Name = "" }},
		{"revision out of range", func(service *Service) { service.Revision = maxSafeInteger + 1 }},
		{"invalid epoch", func(service *Service) { service.Epoch = "bad" }},
		{"missing head hash", func(service *Service) { service.HeadStateHash = Hash{} }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service := valid
			testCase.mutate(&service)
			if err := service.validate(); !errors.Is(err, ErrProtocol) {
				t.Fatalf("validate error = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestServicesResponseValidation(t *testing.T) {
	valid := ServicesResponse{ServicesVersion: 1, Services: []Service{validTestService()}}
	cases := []struct {
		name   string
		mutate func(*ServicesResponse)
	}{
		{"zero version", func(response *ServicesResponse) { response.ServicesVersion = 0 }},
		{"version out of range", func(response *ServicesResponse) { response.ServicesVersion = maxSafeInteger + 1 }},
		{"too many services", func(response *ServicesResponse) {
			response.Services = make([]Service, maxServicesPerSource+1)
		}},
		{"invalid service", func(response *ServicesResponse) { response.Services[0].ID = "bad" }},
		{"duplicate service", func(response *ServicesResponse) {
			response.Services = append(response.Services, response.Services[0])
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := ServicesResponse{ServicesVersion: valid.ServicesVersion}
			response.Services = append([]Service(nil), valid.Services...)
			testCase.mutate(&response)
			if err := response.validate(); !errors.Is(err, ErrProtocol) {
				t.Fatalf("validate error = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestChangesPageEnvelopeValidation(t *testing.T) {
	valid := ChangesResponse{
		ServiceID:     testServiceID1,
		SourceID:      testServiceID1,
		Epoch:         testServiceID1,
		From:          2,
		To:            1,
		Head:          1,
		NextSince:     1,
		HeadStateHash: StateHash(Snapshot{Name: "books"}),
	}
	cases := []struct {
		name   string
		mutate func(*ChangesResponse)
	}{
		{"wrong service", func(response *ChangesResponse) { response.ServiceID = testServiceID2 }},
		{"wrong source", func(response *ChangesResponse) { response.SourceID = testServiceID2 }},
		{"wrong epoch", func(response *ChangesResponse) { response.Epoch = testServiceID2 }},
		{"revision out of range", func(response *ChangesResponse) { response.From = maxSafeInteger + 1 }},
		{"wrong start", func(response *ChangesResponse) { response.From = 3 }},
		{"ends after head", func(response *ChangesResponse) { response.To = 2 }},
		{"wrong next cursor", func(response *ChangesResponse) { response.NextSince = 2 }},
		{"missing head hash", func(response *ChangesResponse) { response.HeadStateHash = Hash{} }},
		{"too many commits", func(response *ChangesResponse) {
			response.Commits = make([]Commit, maxCommitsPerPage+1)
		}},
		{"wrong empty end", func(response *ChangesResponse) {
			response.To = 0
			response.NextSince = 0
		}},
		{"empty page hides head", func(response *ChangesResponse) { response.Head = 2 }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := valid
			testCase.mutate(&response)
			err := response.validatePage(testServiceID1, testServiceID1, testServiceID1, 1, valid.HeadStateHash, 100)
			if !errors.Is(err, ErrProtocol) {
				t.Fatalf("validatePage error = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestSnapshotResponseValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SnapshotResponse)
	}{
		{"invalid service id", func(response *SnapshotResponse) { response.ServiceID = "bad" }},
		{"invalid source id", func(response *SnapshotResponse) { response.SourceID = "bad" }},
		{"invalid epoch", func(response *SnapshotResponse) { response.Epoch = "bad" }},
		{"revision out of range", func(response *SnapshotResponse) { response.Revision = maxSafeInteger + 1 }},
		{"missing state hash", func(response *SnapshotResponse) { response.StateHash = Hash{} }},
		{"missing commit hash", func(response *SnapshotResponse) { response.CommitHash = Hash{} }},
		{"invalid signature", func(response *SnapshotResponse) { response.Signature.Alg = "none" }},
		{"invalid snapshot", func(response *SnapshotResponse) { response.Snapshot.Name = "" }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := validTestSnapshotResponse()
			testCase.mutate(&response)
			if err := response.validate(); !errors.Is(err, ErrProtocol) {
				t.Fatalf("validate error = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestChangeValidation(t *testing.T) {
	valid := Change{
		Op:   "upsert",
		Kind: "authz",
		ID:   "policy-1",
		Name: stringPointer("read-books"),
		SPDL: stringPointer("grant read"),
	}
	cases := []struct {
		name   string
		mutate func(*Change)
	}{
		{"unknown op", func(change *Change) { change.Op = "replace" }},
		{"delete carries name", func(change *Change) { change.Op = "delete" }},
		{"empty id", func(change *Change) { change.ID = "" }},
		{"unknown kind", func(change *Change) { change.Kind = "unknown" }},
		{"missing name", func(change *Change) { change.Name = nil }},
		{"long name", func(change *Change) { change.Name = stringPointer(strings.Repeat("n", maxPolicyNameChars+1)) }},
		{"invalid service name", func(change *Change) {
			change.Kind = "service"
			change.Name = stringPointer("")
			change.SPDL = nil
		}},
		{"service carries SPDL", func(change *Change) { change.Kind = "service" }},
		{"missing SPDL", func(change *Change) { change.SPDL = nil }},
		{"empty SPDL", func(change *Change) { change.SPDL = stringPointer("") }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			change := valid
			testCase.mutate(&change)
			if err := change.validate(); !errors.Is(err, ErrProtocol) {
				t.Fatalf("validate error = %v, want ErrProtocol", err)
			}
		})
	}
}

func TestErrorFormatting(t *testing.T) {
	if got := (&SnapshotRequiredError{}).Error(); got != ErrSnapshotRequired.Error() {
		t.Errorf("empty snapshot-required error = %q", got)
	}
	if got := (&SnapshotRequiredError{Reason: "epoch_mismatch"}).Error(); !strings.Contains(got, "epoch_mismatch") {
		t.Errorf("snapshot-required error = %q", got)
	}
	status := &statusError{method: "GET", path: "/services", status: 503}
	if got := status.Error(); !strings.Contains(got, "503") || !strings.Contains(got, "Service Unavailable") {
		t.Errorf("status error = %q", got)
	}
	status.code = "source_unavailable"
	status.reason = "maintenance"
	if got := status.Error(); !strings.Contains(got, "source_unavailable (maintenance)") {
		t.Errorf("detailed status error = %q", got)
	}
}

func TestSyncerRequiresClient(t *testing.T) {
	if _, err := NewSyncer(nil, ""); err == nil {
		t.Fatal("NewSyncer accepted a nil client")
	}
}
