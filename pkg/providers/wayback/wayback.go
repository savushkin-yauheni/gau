package wayback

import (
	"context"
	"fmt"
	"math"
	jsoniter "github.com/json-iterator/go"
	"github.com/lc/gau/v2/pkg/httpclient"
	"github.com/lc/gau/v2/pkg/providers"
	"github.com/sirupsen/logrus"
)

const (
	Name = "wayback"
)

// verify interface compliance
var _ providers.Provider = (*Client)(nil)

// Client is the structure that holds the WaybackFilters and the Client's configuration
type Client struct {
	filters providers.Filters
	config  *providers.Config
}

func New(config *providers.Config, filters providers.Filters) *Client {
	return &Client{filters, config}
}

func (c *Client) Name() string {
	return Name
}

// waybackResult holds the response from the wayback API
type waybackResult [][]string

// Fetch fetches all urls for a given domain and sends them to a channel.
// It returns an error should one occur.
func (c *Client) Fetch(ctx context.Context, domain string, results chan string) error {
	pages, err := c.getPagination(domain)
	if err != nil {
		return fmt.Errorf("failed to fetch wayback pagination: %s", err)
	}

    max_pages := c.config.Pages
	result_pages := uint(pages)
	if max_pages != 0 {
	    result_pages = uint(math.Min(float64(max_pages), float64(result_pages)))
	}
	for page := uint(0); page < result_pages; page++ {
		select {
		case <-ctx.Done():
			return nil
		default:
			logrus.WithFields(logrus.Fields{"provider": Name, "page": page}).Infof("fetching %s", domain)
			apiURL := c.formatURL(domain, page)
			// make HTTP request
			resp, err := httpclient.MakeRequest(c.config.Client, apiURL, c.config.MaxRetries, c.config.Timeout)
			if err != nil {
				return fmt.Errorf("failed to fetch wayback results page %d: %s", page, err)
			}

			var result waybackResult
			if err = jsoniter.Unmarshal(resp, &result); err != nil {
				return fmt.Errorf("failed to decode wayback results for page %d: %s", page, err)
			}

			// check if there's results, wayback's pagination response
			// is not always correct when using a filter
			if len(result) == 0 {
				break
			}

			// output results
			// Slicing as [1:] to skip first result by default
			for _, entry := range result[1:] {
				results <- entry[0]
			}
		}
	}
	return nil
}

// formatUrl returns a formatted URL for the Wayback API
func (c *Client) formatURL(domain string, page uint) string {
	if c.config.IncludeSubdomains {
		domain = "*." + domain
	}
	filterParams := c.filters.GetParameters(true)
	return fmt.Sprintf(
		"https://web.archive.org/cdx/search/cdx?url=%s/*&output=json&collapse=urlkey&fl=original&page=%d",
		domain, page,
	) + filterParams
}

// getPagination returns the number of pages for Wayback
func (c *Client) getPagination(domain string) (uint, error) {
	url := fmt.Sprintf("%s&showNumPages=true", c.formatURL(domain, 0))
	resp, err := httpclient.MakeRequest(c.config.Client, url, c.config.MaxRetries, c.config.Timeout)

	if err != nil {
		return 0, err
	}

	var paginationResult uint

	if err = jsoniter.Unmarshal(resp, &paginationResult); err != nil {
		return 0, err
	}

	return paginationResult, nil
}
