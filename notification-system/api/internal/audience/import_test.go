package audience_test

import (
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

// smsEndpoints returns n valid endpoints, each of a user of its own.
func smsEndpoints(n int) []audience.Endpoint {
	endpoints := make([]audience.Endpoint, n)
	for i := range endpoints {
		endpoints[i] = audience.Endpoint{UserID: fmt.Sprintf("u%d", i), Address: "+549110000", Channel: audience.ChannelSMS, Provider: "twilio"}
	}
	return endpoints
}

// rowsOf yields endpoints as the rows of an import.
func rowsOf(endpoints []audience.Endpoint) iter.Seq2[audience.Endpoint, error] {
	return func(yield func(audience.Endpoint, error) bool) {
		for _, e := range endpoints {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func TestImportStoresInBatches(t *testing.T) {
	svc, repo := newService(t)
	endpoints := smsEndpoints(2*audience.ImportBatchSize + 1)
	full := gomock.Len(audience.ImportBatchSize)
	gomock.InOrder(
		repo.List(gomock.Any(), appID, "list-1").Return(audience.List{ID: "list-1"}, nil),
		repo.ImportEndpoints(gomock.Any(), appID, "list-1", full).Return(1000, 1000, nil),
		repo.ImportEndpoints(gomock.Any(), appID, "list-1", full).Return(400, 500, nil),
		repo.ImportEndpoints(gomock.Any(), appID, "list-1", endpoints[2*audience.ImportBatchSize:]).Return(0, 1, nil),
	)

	got, err := svc.Import(t.Context(), appID, "list-1", rowsOf(endpoints))
	want := audience.ImportResult{Rows: len(endpoints), Users: 1400, Endpoints: 1501}
	if err != nil || got != want {
		t.Errorf("Import(%d rows) = %+v, %v, want %+v, nil", len(endpoints), got, err, want)
	}
}

func TestImportOfNothing(t *testing.T) {
	svc, _ := newService(t)
	got, err := svc.Import(t.Context(), appID, "", rowsOf(nil))
	if err != nil || got != (audience.ImportResult{}) {
		t.Errorf("Import(no rows) = %+v, %v, want the zero result, nil", got, err)
	}
}

func TestImportIntoUnknownList(t *testing.T) {
	svc, repo := newService(t)
	repo.List(gomock.Any(), appID, "missing").Return(audience.List{}, audience.ErrNotFound)

	_, err := svc.Import(t.Context(), appID, "missing", rowsOf(smsEndpoints(1)))
	if !errors.Is(err, audience.ErrNotFound) {
		t.Errorf("Import(unknown list) = _, %v, want ErrNotFound", err)
	}
}

func TestImportStopsAtBadRow(t *testing.T) {
	// The bad row is the third of the second batch, so the first batch is stored and
	// the second is not.
	const badRow = audience.ImportBatchSize + 3
	tests := []struct {
		name string
		bad  audience.Endpoint
	}{
		{name: "NoUserID", bad: audience.Endpoint{Address: "x", Channel: audience.ChannelSMS, Provider: "twilio"}},
		{name: "NoAddress", bad: audience.Endpoint{UserID: "ana", Channel: audience.ChannelSMS, Provider: "twilio"}},
		{name: "UnknownChannel", bad: audience.Endpoint{UserID: "ana", Address: "x", Channel: "fax", Provider: "twilio"}},
		{name: "ProviderOfAnotherChannel", bad: audience.Endpoint{UserID: "ana", Address: "x", Channel: audience.ChannelSMS, Provider: "fcm"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, repo := newService(t)
			endpoints := smsEndpoints(audience.ImportBatchSize + 10)
			endpoints[badRow-1] = test.bad
			repo.ImportEndpoints(gomock.Any(), appID, "", gomock.Len(audience.ImportBatchSize)).Return(1000, 1000, nil)

			got, err := svc.Import(t.Context(), appID, "", rowsOf(endpoints))
			if !errors.Is(err, audience.ErrInvalid) || !strings.Contains(err.Error(), fmt.Sprintf("row %d:", badRow)) {
				t.Errorf("Import(bad row %d) = _, %v, want ErrInvalid naming the row", badRow, err)
			}
			if got.Rows != audience.ImportBatchSize {
				t.Errorf("Import(bad row %d) stored %d rows, want %d", badRow, got.Rows, audience.ImportBatchSize)
			}
		})
	}
}

func TestImportStopsAtUnreadableRow(t *testing.T) {
	svc, _ := newService(t)
	rows := func(yield func(audience.Endpoint, error) bool) {
		if yield(smsEndpoints(1)[0], nil) {
			yield(audience.Endpoint{}, errors.New("wrong number of fields"))
		}
	}
	_, err := svc.Import(t.Context(), appID, "", rows)
	if !errors.Is(err, audience.ErrInvalid) || !strings.Contains(err.Error(), "row 2:") {
		t.Errorf("Import(unreadable second row) = _, %v, want ErrInvalid naming row 2", err)
	}
}
