package weather

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/ipinfo/go/v2/ipinfo"
)

// IPInfoClient is the only code that knows the IPInfo API. It implements
// locator.
type IPInfoClient struct {
	client *ipinfo.Client
}

// NewIPInfoClient builds an IPInfo client with the API token injected.
func NewIPInfoClient(token string) *IPInfoClient {
	return &IPInfoClient{client: ipinfo.NewClient(nil, nil, token)}
}

// Locate looks up ip and returns its city and coordinates. A missing or
// malformed "lat,lon" in the response is an error, never 0,0.
func (c *IPInfoClient) Locate(ip string) (Location, error) {
	info, err := c.client.GetIPInfo(net.ParseIP(ip))
	if err != nil {
		return Location{}, err
	}

	lat, lon, err := parseLatLon(info.Location)
	if err != nil {
		return Location{}, err
	}

	return Location{City: info.City, Lat: lat, Lon: lon}, nil
}

// parseLatLon parses IPInfo's "lat,lon" location string.
func parseLatLon(s string) (float64, float64, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("error parsing lat/lon %q", s)
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("error parsing lat %q: %w", parts[0], err)
	}

	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("error parsing lon %q: %w", parts[1], err)
	}

	return lat, lon, nil
}
