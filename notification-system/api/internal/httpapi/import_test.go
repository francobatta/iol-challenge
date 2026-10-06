package httpapi

import (
	"net/http"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/audience"
)

func TestImportUsers(t *testing.T) {
	c, repo := newTestAPI(t)
	const body = "ana,sms,twilio,+5491100000000\n" +
		"ana,email,mailchimp,ana@example.com\n" +
		"bob,push,fcm,\"token,with a comma\"\n"
	want := []audience.Endpoint{
		{UserID: "ana", Address: "+5491100000000", Channel: audience.ChannelSMS, Provider: "twilio"},
		{UserID: "ana", Address: "ana@example.com", Channel: audience.ChannelEmail, Provider: "mailchimp"},
		{UserID: "bob", Address: "token,with a comma", Channel: audience.ChannelPush, Provider: "fcm"},
	}
	repo.List(gomock.Any(), testAppID, "l1").Return(audience.List{ID: "l1"}, nil)
	repo.ImportEndpoints(gomock.Any(), testAppID, "l1", want).Return(2, 3, nil)

	var got audience.ImportResult
	status := c.call(t, "POST", "/v1/users/import?list_id=l1", body, &got)
	wantResult := audience.ImportResult{Rows: 3, Users: 2, Endpoints: 3}
	if status != http.StatusOK || got != wantResult {
		t.Errorf("POST /v1/users/import = %d %+v, want %d %+v", status, got, http.StatusOK, wantResult)
	}
}

func TestImportUsersRejectsBadCSV(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "TooFewFields", body: "ana,sms,twilio\n"},
		{name: "TooManyFields", body: "ana,sms,twilio,+549,extra\n"},
		{name: "UnterminatedQuote", body: "ana,sms,twilio,\"+549\n"},
		{name: "UnknownChannel", body: "ana,fax,twilio,+549\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := newTestAPI(t)
			if got := c.call(t, "POST", "/v1/users/import", test.body, nil); got != http.StatusBadRequest {
				t.Errorf("POST /v1/users/import with body %q = %d, want %d", test.body, got, http.StatusBadRequest)
			}
		})
	}
}
