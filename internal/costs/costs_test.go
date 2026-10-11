package costs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type fakeCred struct{}

func (fakeCred) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "t", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestAzureQueryAndParse(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(raw)
		body = string(raw)
		if r.Header.Get("Authorization") != "Bearer t" || !strings.Contains(r.URL.Path, "/subscriptions/sub/providers/Microsoft.CostManagement/query") {
			t.Errorf("request = %s %s", r.Header.Get("Authorization"), r.URL)
		}
		_, _ = w.Write([]byte(`{"properties":{"columns":[{"name":"PreTaxCost"},{"name":"UsageDate"},{"name":"ResourceGroupName"},{"name":"Tag"},{"name":"Currency"}],
			"rows":[[1.5,20261001,"RG-KUBETRE-WORKSPACES","\"kubetre-workspace\":\"study1\"","GBP"],[2,20261001,"mc_rg","","GBP"]]}}`))
	}))
	defer srv.Close()
	a := &Azure{Credential: fakeCred{}, SubscriptionID: "sub", ResourceGroups: []string{"rg-a"}, Endpoint: srv.URL}
	rows, err := a.Costs(context.Background(), Query{Daily: true, ByTag: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `{"type":"Tag"}`) || !strings.Contains(body, `"timeframe":"MonthToDate"`) || !strings.Contains(body, `"granularity":"Daily"`) || !strings.Contains(body, `"rg-a"`) {
		t.Fatalf("body = %s", body)
	}
	if len(rows) != 2 || rows[0] != (Row{Cost: 1.5, Currency: "GBP", Date: "2026-10-01", ResourceGroup: "rg-kubetre-workspaces", TagKey: "kubetre-workspace", TagValue: "study1"}) ||
		rows[1].TagKey != "" || rows[1].ResourceGroup != "mc_rg" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestThrottlingAndUnsupported(t *testing.T) {
	status, payload := 429, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ms-ratelimit-microsoft.consumption-retry-after", "42")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()
	a := &Azure{Credential: fakeCred{}, SubscriptionID: "sub", Endpoint: srv.URL}
	_, err := a.Costs(context.Background(), Query{})
	if te, ok := err.(*ThrottledError); !ok || te.RetryAfter != 42 {
		t.Fatalf("err = %v", err)
	}
	status, payload = 404, `{"error":{"message":"Given subscription x doesn't have valid WebDirect/AIRS offer type"}}`
	if _, err := a.Costs(context.Background(), Query{}); err != ErrNotSupported {
		t.Fatalf("err = %v", err)
	}
}

type countingSource struct{ n int }

func (c *countingSource) Costs(context.Context, Query) ([]Row, error) {
	c.n++
	return []Row{{Cost: 1}}, nil
}

func TestCachedKeepsResultsForTTL(t *testing.T) {
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	src := &countingSource{}
	c := &Cached{Source: src, TTL: 2 * time.Hour, Now: func() time.Time { return now }}
	_, _ = c.Costs(context.Background(), Query{})
	_, _ = c.Costs(context.Background(), Query{})
	_, _ = c.Costs(context.Background(), Query{Daily: true})
	now = now.Add(3 * time.Hour)
	_, _ = c.Costs(context.Background(), Query{})
	if src.n != 3 {
		t.Fatalf("queries = %d, want 3", src.n)
	}
}
