package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"
)

const (
	maxCatalogBytes     = 1 << 20
	maxCatalogPools     = 100
	maxCatalogEndpoints = 1000
	maxClientRTTWeight  = 1000
	sourceTimeout       = 3 * time.Second
)

var catalogSources = map[Region][]string{
	RegionEU: {
		"https://backend-eu.stalzone.com/address_list",
		"https://backend-eu.stalzone.net/address_list",
	},
	RegionNA: {
		"https://backend-na.stalzone.com/address_list",
		"https://backend-na.stalzone.net/address_list",
	},
	RegionSEA: {
		"https://backend-sea.stalzone.com/address_list",
		"https://backend-sea.stalzone.net/address_list",
	},
	RegionNEA: {
		"https://backend-nea.stalzone.com/address_list",
		"https://backend-nea.stalzone.net/address_list",
	},
	RegionRU: {
		"https://backend.stalzone.net/address_list",
		"https://backend.stalzone.com/address_list",
		"https://backend.stalcraftx.ru/address_list",
	},
}

func Sources(region Region) []string {
	return append([]string(nil), catalogSources[region]...)
}

func Fetch(ctx context.Context, region Region) (Catalog, error) {
	client := &http.Client{
		Timeout: sourceTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return fetchCatalog(ctx, client, Sources(region), Login)
}

func fetchCatalog(
	ctx context.Context,
	client *http.Client,
	sources []string,
	login string,
) (Catalog, error) {
	if len(sources) == 0 {
		return Catalog{}, fmt.Errorf("no tunnel catalog sources")
	}
	if client == nil {
		return Catalog{}, fmt.Errorf("nil tunnel catalog http client")
	}

	failures := make([]error, 0, len(sources))
	for _, source := range sources {
		sourceCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
		catalog, err := fetchSource(sourceCtx, client, source, login)
		cancel()
		if err == nil {
			catalog, err = validateCatalog(catalog)
		}
		if err == nil {
			return catalog, nil
		}
		failures = append(failures, err)
		if ctx.Err() != nil {
			break
		}
	}
	if len(failures) == 0 {
		return Catalog{}, fmt.Errorf("no tunnel catalog sources")
	}
	return Catalog{}, fmt.Errorf("fetch tunnel catalog: %w", errors.Join(failures...))
}

func fetchSource(
	ctx context.Context,
	client *http.Client,
	source string,
	login string,
) (Catalog, error) {
	endpoint, err := url.Parse(source)
	if err != nil {
		return Catalog{}, fmt.Errorf("parse tunnel catalog url: %w", err)
	}
	if endpoint.Scheme != "https" || endpoint.Host == "" {
		return Catalog{}, fmt.Errorf("reject non-https tunnel catalog source %q", source)
	}
	query := endpoint.Query()
	query.Set("login", login)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("create %s catalog request: %w", endpoint.Host, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Catalog{}, fmt.Errorf("%s: %w", endpoint.Host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Catalog{}, fmt.Errorf("%s: unexpected http status %d", endpoint.Host, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogBytes+1))
	if err != nil {
		return Catalog{}, fmt.Errorf("%s: read response: %w", endpoint.Host, err)
	}
	if len(data) > maxCatalogBytes {
		return Catalog{}, fmt.Errorf("%s: response exceeds %d bytes", endpoint.Host, maxCatalogBytes)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("%s: parse response: %w", endpoint.Host, err)
	}
	return catalog, nil
}

func validateCatalog(catalog Catalog) (Catalog, error) {
	if catalog.Mode != "roxy" {
		return Catalog{}, fmt.Errorf("unsupported tunnel mode %q", catalog.Mode)
	}
	if math.IsNaN(catalog.ClientToTunnelRTTWeight) ||
		math.IsInf(catalog.ClientToTunnelRTTWeight, 0) ||
		catalog.ClientToTunnelRTTWeight < 0 ||
		catalog.ClientToTunnelRTTWeight > maxClientRTTWeight {
		return Catalog{}, fmt.Errorf(
			"invalid client-to-tunnel rtt weight %v",
			catalog.ClientToTunnelRTTWeight,
		)
	}
	if len(catalog.Pools) == 0 || len(catalog.Pools) > maxCatalogPools {
		return Catalog{}, fmt.Errorf("invalid tunnel pool count %d", len(catalog.Pools))
	}

	seenPools := make(map[string]struct{}, len(catalog.Pools))
	pools := make([]Pool, 0, len(catalog.Pools))
	total := 0

	for _, pool := range catalog.Pools {
		if !validLabel(pool.Name) {
			return Catalog{}, fmt.Errorf("invalid tunnel pool name %q", pool.Name)
		}
		if _, ok := seenPools[pool.Name]; ok {
			return Catalog{}, fmt.Errorf("duplicate tunnel pool %q", pool.Name)
		}
		seenPools[pool.Name] = struct{}{}

		endpoints := make([]Endpoint, 0, len(pool.Tunnels))
		seenAddresses := make(map[string]struct{}, len(pool.Tunnels))
		for _, endpoint := range pool.Tunnels {
			if !validLabel(endpoint.Name) {
				return Catalog{}, fmt.Errorf("invalid tunnel endpoint name %q", endpoint.Name)
			}
			if _, _, err := ParseAddress(endpoint.Address); err != nil {
				return Catalog{}, fmt.Errorf(
					"invalid tunnel endpoint %s: %w",
					endpoint.Name,
					err,
				)
			}
			if _, ok := seenAddresses[endpoint.Address]; ok {
				continue
			}
			seenAddresses[endpoint.Address] = struct{}{}
			endpoints = append(endpoints, endpoint)
			total++
			if total > maxCatalogEndpoints {
				return Catalog{}, fmt.Errorf(
					"tunnel endpoint count exceeds %d",
					maxCatalogEndpoints,
				)
			}
		}
		if len(endpoints) == 0 {
			continue
		}
		pools = append(pools, Pool{Name: pool.Name, Tunnels: endpoints})
	}
	if len(pools) == 0 {
		return Catalog{}, fmt.Errorf("tunnel catalog contains no endpoints")
	}

	catalog.Pools = pools
	return catalog, nil
}
