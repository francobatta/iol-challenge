// Package providers names the services that deliver notifications.
//
// To add a provider:
//
//  1. Add its name here, to the constants and to Names.
//  2. Say which channel it delivers on, in the api module's audience package.
//  3. Teach the worker to call it, in the worker module's provider package. A test
//     there fails until every name here has a client.
//  4. Give it a route in the mock, worker/cmd/mockprovider.
//  5. Run a worker pool for it: a service in compose.yaml, and a Deployment and a
//     ScaledObject in deploy/k8s.
package providers

const (
	Twilio    = "twilio"    // SMS
	Mailchimp = "mailchimp" // email
	APNs      = "apns"      // push
	FCM       = "fcm"       // push
)

func Names() []string {
	return []string{Twilio, Mailchimp, APNs, FCM}
}
