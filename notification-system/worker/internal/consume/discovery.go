package consume

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/francobatta/iol-challenge/notification-system/commons/topology"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// discoveryPageSize is how many queue names are asked of the management API at a time.
const discoveryPageSize = 500

// A Discoverer finds the send queues of a provider by asking RabbitMQ's management
// API, since AMQP itself has no way to list queues.
type Discoverer struct {
	apiURL   string // without credentials
	username string
	password string
	prefix   string
	client   *http.Client
}

// NewDiscoverer returns a Discoverer of the provider's send queues. apiURL is the base
// URL of the management API, such as http://user:password@rabbitmq:15672; its
// credentials are sent as HTTP basic authentication.
func NewDiscoverer(apiURL, provider string, client *http.Client) (*Discoverer, error) {
	u, err := url.Parse(apiURL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%q is not a URL of the RabbitMQ management API", apiURL)
	}
	d := &Discoverer{prefix: topology.SendQueuePrefix(provider), client: client}
	if u.User != nil {
		d.username = u.User.Username()
		d.password, _ = u.User.Password()
		u.User = nil
	}
	d.apiURL = strings.TrimSuffix(u.String(), "/")
	return d, nil
}

// Queues returns the names of the provider's send queues that exist now.
func (d *Discoverer) Queues(ctx context.Context) ([]string, error) {
	var names []string
	for page := 1; ; page++ {
		query := url.Values{
			"name":      {"^" + regexp.QuoteMeta(d.prefix)},
			"use_regex": {"true"},
			"columns":   {"name"},
			"page":      {strconv.Itoa(page)},
			"page_size": {strconv.Itoa(discoveryPageSize)},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.apiURL+"/api/queues?"+query.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("building the queue listing request: %v", err)
		}
		req.SetBasicAuth(d.username, d.password)
		resp, err := d.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("listing queues: %v", err)
		}
		var body struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			PageCount int `json:"page_count"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("listing queues: the management API answered %d", resp.StatusCode)
		}
		if err != nil {
			return nil, fmt.Errorf("listing queues: decoding the answer: %v", err)
		}
		for _, item := range body.Items {
			// The API has already filtered by name; checking again costs nothing and
			// keeps a worker off queues that are not its own whatever the API does.
			if strings.HasPrefix(item.Name, d.prefix) {
				names = append(names, item.Name)
			}
		}
		if page >= body.PageCount {
			return names, nil
		}
	}
}
