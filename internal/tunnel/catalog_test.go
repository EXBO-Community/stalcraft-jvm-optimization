package tunnel

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFetchCatalogFallsBackAndAddsLogin(t *testing.T) {
	t.Parallel()

	var requests []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if got := r.URL.Query().Get("login"); got != Login {
			http.Error(w, "missing login", http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/first" {
			_, _ = fmt.Fprint(w, `{"mode":"unsupported","pools":[]}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"mode":"roxy",
			"clientToTunnelRttWeight":1.25,
			"pools":[
				{"name":"MSK2","tunnels":[
					{"name":"MSK2-1","address":"192.0.2.10:29450"},
					{"name":"duplicate","address":"192.0.2.10:29450"}
				]}
			]
		}`)
	}))
	t.Cleanup(server.Close)

	got, err := fetchCatalog(
		context.Background(),
		server.Client(),
		[]string{server.URL + "/first", server.URL + "/second"},
		Login,
	)
	if err != nil {
		t.Fatalf("fetchCatalog(): %v", err)
	}
	if !reflect.DeepEqual(requests, []string{"/first", "/second"}) {
		t.Fatalf("requests = %#v, want fallback order", requests)
	}
	if len(got.Pools) != 1 || len(got.Pools[0].Tunnels) != 1 {
		t.Fatalf("catalog was not normalized: %#v", got)
	}
	if got.ClientToTunnelRTTWeight != 1.25 {
		t.Fatalf("weight = %v, want 1.25", got.ClientToTunnelRTTWeight)
	}
}

func TestFetchCatalogRejectsInsecureSource(t *testing.T) {
	t.Parallel()

	_, err := fetchCatalog(
		context.Background(),
		http.DefaultClient,
		[]string{"http://backend.example/address_list"},
		Login,
	)
	if err == nil {
		t.Fatal("fetchCatalog() accepted an HTTP source")
	}
}

func TestValidateCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		catalog Catalog
	}{
		{
			name: "unsupported mode",
			catalog: Catalog{
				Mode:  "direct",
				Pools: []Pool{{Name: "MSK", Tunnels: []Endpoint{{Name: "node", Address: "127.0.0.1:1"}}}},
			},
		},
		{
			name: "invalid address",
			catalog: Catalog{
				Mode:  "roxy",
				Pools: []Pool{{Name: "MSK", Tunnels: []Endpoint{{Name: "node", Address: "invalid"}}}},
			},
		},
		{
			name: "duplicate pool",
			catalog: Catalog{
				Mode: "roxy",
				Pools: []Pool{
					{Name: "MSK", Tunnels: []Endpoint{{Name: "one", Address: "127.0.0.1:1000"}}},
					{Name: "MSK", Tunnels: []Endpoint{{Name: "two", Address: "127.0.0.2:1000"}}},
				},
			},
		},
		{
			name: "excessive client weight",
			catalog: Catalog{
				Mode:                    "roxy",
				ClientToTunnelRTTWeight: maxClientRTTWeight + 1,
				Pools: []Pool{{
					Name: "MSK",
					Tunnels: []Endpoint{{
						Name:    "node",
						Address: "127.0.0.1:1000",
					}},
				}},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := validateCatalog(tt.catalog); err == nil {
				t.Fatalf("validateCatalog(%s) returned no error", tt.name)
			}
		})
	}
}
