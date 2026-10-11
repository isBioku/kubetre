// Package costs reads Azure Cost Management for the AzureTRE cost API: actual costs grouped
// by resource group and tag, as AzureTRE's API queries them.
package costs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// Query selects the period and granularity of a cost query.
type Query struct {
	// From and To are both set for a custom period; otherwise the month to date is used.
	From, To time.Time
	Daily    bool
	// ByTag also groups by tag. A resource then appears once for each of its tags, so totals
	// per resource group come from a query without it.
	ByTag bool
}

// Row is one cost total for a resource group and one of its tags, on one day when daily.
type Row struct {
	Cost          float64
	Currency      string
	Date          string // YYYY-MM-DD, or empty without daily granularity
	ResourceGroup string
	TagKey        string // empty for untagged costs
	TagValue      string
}

// Source returns cost rows for the TRE's resource groups.
type Source interface {
	Costs(ctx context.Context, q Query) ([]Row, error)
}

// ErrNotSupported means the subscription's offer has no Cost Management data.
var ErrNotSupported = errors.New("cost management is not supported by this subscription")

// ThrottledError means Cost Management asked the caller to come back later.
type ThrottledError struct {
	Status     int
	RetryAfter int
}

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("cost management returned %d; retry after %d seconds", e.Status, e.RetryAfter)
}

// Azure queries Cost Management at subscription scope, filtered to the TRE's resource groups.
type Azure struct {
	Credential     azcore.TokenCredential
	SubscriptionID string
	ResourceGroups []string
	HTTP           *http.Client
	// Endpoint is the Resource Manager address; https://management.azure.com when empty.
	Endpoint string
}

func (a *Azure) Costs(ctx context.Context, q Query) ([]Row, error) {
	granularity := "None"
	if q.Daily {
		granularity = "Daily"
	}
	body := map[string]any{
		"type": "ActualCost",
		"dataset": map[string]any{
			"granularity": granularity,
			"aggregation": map[string]any{"totalCost": map[string]any{"name": "PreTaxCost", "function": "Sum"}},
			"grouping":    grouping(q.ByTag),
			"filter":      map[string]any{"dimensions": map[string]any{"name": "ResourceGroupName", "operator": "In", "values": a.ResourceGroups}},
		},
	}
	if q.From.IsZero() || q.To.IsZero() {
		body["timeframe"] = "MonthToDate"
	} else {
		body["timeframe"] = "Custom"
		body["timePeriod"] = map[string]any{"from": q.From.UTC().Format(time.RFC3339), "to": q.To.UTC().Format(time.RFC3339)}
	}
	endpoint := a.Endpoint
	if endpoint == "" {
		endpoint = "https://management.azure.com"
	}
	url := fmt.Sprintf("%s/subscriptions/%s/providers/Microsoft.CostManagement/query?api-version=2023-11-01", endpoint, a.SubscriptionID)
	var rows []Row
	for url != "" {
		page, next, err := a.page(ctx, url, body)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page...)
		url = next
	}
	return rows, nil
}

func grouping(byTag bool) []any {
	g := []any{map[string]any{"type": "Dimension", "name": "ResourceGroupName"}}
	if byTag {
		g = append(g, map[string]any{"type": "Tag"})
	}
	return g
}

func (a *Azure) page(ctx context.Context, url string, body any) ([]Row, string, error) {
	tok, err := a.Credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"https://management.azure.com/.default"}})
	if err != nil {
		return nil, "", fmt.Errorf("token: %w", err)
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	req.Header.Set("Content-Type", "application/json")
	client := a.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
		retry, _ := strconv.Atoi(resp.Header.Get("x-ms-ratelimit-microsoft.consumption-retry-after"))
		if retry == 0 {
			retry, _ = strconv.Atoi(resp.Header.Get("Retry-After"))
		}
		if retry == 0 {
			retry = 60
		}
		return nil, "", &ThrottledError{Status: resp.StatusCode, RetryAfter: retry}
	case resp.StatusCode == http.StatusNotFound && bytes.Contains(payload, []byte("doesn't have valid WebDirect/AIRS")):
		return nil, "", ErrNotSupported
	case resp.StatusCode != http.StatusOK:
		return nil, "", fmt.Errorf("cost management returned %d: %s", resp.StatusCode, truncate(string(payload), 300))
	}
	return ParseResult(payload)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ParseResult reads a Cost Management query result. Columns are named, so their order does not
// matter; tags arrive as "key":"value" strings.
func ParseResult(payload []byte) ([]Row, string, error) {
	var res struct {
		Properties struct {
			NextLink string `json:"nextLink"`
			Columns  []struct {
				Name string `json:"name"`
			} `json:"columns"`
			Rows [][]any `json:"rows"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(payload, &res); err != nil {
		return nil, "", fmt.Errorf("cost management result: %w", err)
	}
	col := map[string]int{}
	for i, c := range res.Properties.Columns {
		col[strings.ToLower(c.Name)] = i
	}
	get := func(r []any, names ...string) any {
		for _, n := range names {
			if i, ok := col[n]; ok && i < len(r) {
				return r[i]
			}
		}
		return nil
	}
	var rows []Row
	for _, r := range res.Properties.Rows {
		row := Row{}
		switch v := get(r, "pretaxcost", "cost", "totalcost").(type) {
		case float64:
			row.Cost = v
		}
		row.Currency, _ = get(r, "currency").(string)
		row.ResourceGroup, _ = get(r, "resourcegroupname", "resourcegroup").(string)
		row.ResourceGroup = strings.ToLower(row.ResourceGroup)
		if d, ok := get(r, "usagedate").(float64); ok {
			s := strconv.FormatInt(int64(d), 10)
			if len(s) == 8 {
				row.Date = s[:4] + "-" + s[4:6] + "-" + s[6:]
			}
		}
		if tag, _ := get(r, "tag").(string); tag != "" {
			row.TagKey, row.TagValue = splitTag(tag)
		}
		rows = append(rows, row)
	}
	return rows, res.Properties.NextLink, nil
}

// splitTag splits Cost Management's "key":"value" form.
func splitTag(s string) (string, string) {
	k, v, ok := strings.Cut(s, `":"`)
	if !ok {
		return strings.ToLower(strings.Trim(s, `"`)), ""
	}
	return strings.ToLower(strings.Trim(k, `"`)), strings.Trim(v, `"`)
}

// Cached wraps a source and keeps results for TTL, as AzureTRE does: Cost Management is slow
// and rate limited, and its data is refreshed only a few times a day.
type Cached struct {
	Source Source
	TTL    time.Duration
	Now    func() time.Time

	mu      sync.Mutex
	entries map[Query]cached
}

type cached struct {
	rows    []Row
	expires time.Time
}

func (c *Cached) Costs(ctx context.Context, q Query) ([]Row, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	c.mu.Lock()
	if e, ok := c.entries[q]; ok && now().Before(e.expires) {
		c.mu.Unlock()
		return e.rows, nil
	}
	c.mu.Unlock()
	rows, err := c.Source.Costs(ctx, q)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[Query]cached{}
	}
	c.entries[q] = cached{rows: rows, expires: now().Add(c.TTL)}
	return rows, nil
}
