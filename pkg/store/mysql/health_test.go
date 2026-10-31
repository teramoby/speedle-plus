// Licensed under the Universal Permissive License (UPL) Version 1.0.
package mysql

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/teramoby/speedle-plus/api/pms"
)

func TestHealthChecksReadableRevisionAndRedactsErrors(t *testing.T) {
	s, m := mockStore(t)
	m.ExpectQuery("SELECT revision").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(42))
	if err := s.CheckHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.ExpectQuery("SELECT revision").WillReturnError(stderrors.New("offline secret-password"))
	if err := s.CheckHealth(context.Background()); err == nil || strings.Contains(err.Error(), "secret-password") {
		t.Fatalf("unredacted/missing health error: %v", err)
	}
}

func TestWatchRepublishesUnchangedRevisionAfterRecovery(t *testing.T) {
	s, m := mockStore(t)
	expectSnapshot(m, 1, true)
	m.ExpectBegin()
	m.ExpectQuery("SELECT revision").WillReturnError(stderrors.New("offline"))
	m.ExpectRollback()
	expectSnapshot(m, 1, true)
	events, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []pms.EventType{pms.FULL_RELOAD, pms.STORE_UNAVAILABLE, pms.FULL_RELOAD} {
		select {
		case event := <-events:
			if event.Type != kind {
				t.Fatalf("event %v, want %v", event.Type, kind)
			}
		case <-time.After(time.Second):
			t.Fatal("no recovery snapshot for unchanged revision")
		}
	}
	s.StopWatch()
}
