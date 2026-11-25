package push

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

var (
	ErrInvalidEndpoint = errors.New("invalid push endpoint")
	ErrUnsafeEndpoint  = errors.New("unsafe push endpoint")
)

var nonPublicNetworks = parseNetworks([]string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.88.99.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
	"2001:db8::/32",
})

func parseNetworks(networks []string) []*net.IPNet {
	result := make([]*net.IPNet, 0, len(networks))
	for _, network := range networks {
		_, parsed, err := net.ParseCIDR(network)
		if err != nil {
			panic(err)
		}
		result = append(result, parsed)
	}
	return result
}

func ValidateEndpoint(ctx context.Context, endpoint string, resolver Resolver) (*url.URL, error) {
	if len(endpoint) == 0 || len(endpoint) > 2048 {
		return nil, fmt.Errorf("%w: length is invalid", ErrInvalidEndpoint)
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: must be an HTTPS URL without credentials or fragment", ErrInvalidEndpoint)
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return nil, fmt.Errorf("%w: must use port 443", ErrInvalidEndpoint)
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if err := validateHostAddresses(ctx, parsed.Hostname(), resolver); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateHostAddresses(ctx context.Context, host string, resolver Resolver) error {
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicAddress(ip) {
			return fmt.Errorf("%w: host is not publicly routable", ErrUnsafeEndpoint)
		}
		return nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve push endpoint host")
	}
	if len(addresses) == 0 {
		return fmt.Errorf("%w: host has no addresses", ErrUnsafeEndpoint)
	}
	for _, address := range addresses {
		if !isPublicAddress(address.IP) {
			return fmt.Errorf("%w: host is not publicly routable", ErrUnsafeEndpoint)
		}
	}
	return nil
}

func isPublicAddress(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, network := range nonPublicNetworks {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

type PublicEndpointTransport struct {
	Resolver Resolver
	Dialer   net.Dialer
}

func (transport *PublicEndpointTransport) DialContext(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid push endpoint address")
	}
	if port != "443" {
		return nil, fmt.Errorf("%w: must use port 443", ErrInvalidEndpoint)
	}
	resolver := transport.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := transport.Dialer
	if dialer.Timeout == 0 {
		dialer.Timeout = 8 * time.Second
		dialer.KeepAlive = 30 * time.Second
	}

	if ip := net.ParseIP(host); ip != nil {
		if !isPublicAddress(ip) {
			return nil, fmt.Errorf("%w: host is not publicly routable", ErrUnsafeEndpoint)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}

	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve push endpoint host")
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("%w: host has no addresses", ErrUnsafeEndpoint)
	}
	for _, candidate := range addresses {
		if !isPublicAddress(candidate.IP) {
			return nil, fmt.Errorf("%w: host is not publicly routable", ErrUnsafeEndpoint)
		}
	}

	var lastErr error
	for _, candidate := range addresses {
		connection, err := dialer.DialContext(
			ctx,
			network,
			net.JoinHostPort(candidate.IP.String(), port),
		)
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		return nil, fmt.Errorf("no public push endpoint address is available")
	}
	return nil, lastErr
}

func NewPublicEndpointHTTPClientWithResolver(resolver Resolver) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: (&PublicEndpointTransport{
				Resolver: resolver,
				Dialer: net.Dialer{
					Timeout:   8 * time.Second,
					KeepAlive: 30 * time.Second,
				},
			}).DialContext,
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: 8 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			MaxIdleConns:          16,
			MaxIdleConnsPerHost:   2,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 15 * time.Second,
	}
}
