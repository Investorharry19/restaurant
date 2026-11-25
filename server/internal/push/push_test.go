package push

import (
	"context"
	"net"
	"testing"
)

type staticResolver []net.IP

func (resolver staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	addresses := make([]net.IPAddr, 0, len(resolver))
	for _, ip := range resolver {
		addresses = append(addresses, net.IPAddr{IP: ip})
	}
	return addresses, nil
}

func TestValidateEndpointRejectsNonPublicAndNonHTTPSAddresses(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		resolver staticResolver
	}{
		{name: "http", endpoint: "http://push.example.test/send"},
		{name: "credentials", endpoint: "https://user:pass@push.example.test/send"},
		{name: "custom port", endpoint: "https://push.example.test:8443/send"},
		{name: "loopback IP", endpoint: "https://127.0.0.1/send"},
		{name: "private DNS result", endpoint: "https://push.example.test/send", resolver: staticResolver{net.ParseIP("10.2.3.4")}},
		{name: "mixed public and private DNS results", endpoint: "https://push.example.test/send", resolver: staticResolver{net.ParseIP("8.8.8.8"), net.ParseIP("192.168.1.1")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateEndpoint(context.Background(), test.endpoint, test.resolver)
			if err == nil {
				t.Fatal("ValidateEndpoint() accepted an unsafe endpoint")
			}
		})
	}
}

func TestValidateEndpointAcceptsPublicHTTPSHost(t *testing.T) {
	endpoint, err := ValidateEndpoint(
		context.Background(),
		"https://push.example.test/send/token",
		staticResolver{net.ParseIP("8.8.8.8")},
	)
	if err != nil {
		t.Fatalf("ValidateEndpoint() error = %v", err)
	}
	if endpoint.Hostname() != "push.example.test" {
		t.Fatalf("endpoint host = %q", endpoint.Hostname())
	}
}

func TestTicketTokenEncryptionAuthenticatesCiphertext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	encrypted, err := EncryptTicketToken(key, "private-ticket-token")
	if err != nil {
		t.Fatalf("EncryptTicketToken() error = %v", err)
	}
	decrypted, err := DecryptTicketToken(key, encrypted)
	if err != nil {
		t.Fatalf("DecryptTicketToken() error = %v", err)
	}
	if decrypted != "private-ticket-token" {
		t.Fatalf("decrypted token = %q", decrypted)
	}

	encrypted[len(encrypted)-1] ^= 0xff
	if _, err := DecryptTicketToken(key, encrypted); err == nil {
		t.Fatal("DecryptTicketToken() accepted modified ciphertext")
	}
}
