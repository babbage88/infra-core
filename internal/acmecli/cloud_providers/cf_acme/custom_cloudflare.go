package cf_acme

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-acme/lego/v4/challenge/dns01"
)

type ICertRenewalService interface {
	Renew(token string, recurseiveNameservers []string, timeout time.Duration) (CertificateData, error)
}

type InfraCfCustomDNSProvider struct {
	DnsToken                 string        `json:"dnsToken"`
	ZoneToken                string        `json:"zoneToken"`
	RecursiveNameServers     []string      `json:"recursiveNameServers"`
	NewTxtRecordId           string        `json:"newTxtRecordId"`
	CreatedChallengeTXT      bool          `json:"txtCreated"`
	TTL                      int           `json:"ttl"`
	ZoneID                   string        `json:"zoneID"`
	PropagationTimeout       time.Duration `json:"propagationTimout"`
	PropagationCheckInterval time.Duration `json:"checkInterval"`
}

func (d *InfraCfCustomDNSProvider) Timeout() (timeout, interval time.Duration) {
	return d.PropagationTimeout, d.PropagationCheckInterval
}

func NewInfraCfCustomDNSProvider(dnsApiToken string, zoneApiToken string, recursiveNameServers []string, timeout time.Duration) (*InfraCfCustomDNSProvider, error) {
	ttl := int(120)
	checkInterval := 2 * time.Second

	return &InfraCfCustomDNSProvider{DnsToken: dnsApiToken, ZoneToken: zoneApiToken, RecursiveNameServers: recursiveNameServers, TTL: ttl, PropagationTimeout: timeout, PropagationCheckInterval: checkInterval}, nil
}

func (d *InfraCfCustomDNSProvider) SetZoneId(domain string) error {
	zoneid, err := GetCloudflareZoneIdFromDomainName(d.ZoneToken, domain)
	if err != nil {
		slog.Error("error retrieving zone id from domain name", slog.String("domain", domain), slog.String("error", err.Error()))
		return err
	}
	d.ZoneID = zoneid
	return err
}

func (d *InfraCfCustomDNSProvider) Present(domain, token, keyAuth string) error {
	if len(d.ZoneID) < 1 {
		err := d.SetZoneId(domain)
		if err != nil {
			slog.Error("error during Present retrieving zone id from domain name", slog.String("domain", domain), slog.String("error", err.Error()))
			return err
		}
	}

	info := dns01.GetChallengeInfo(domain, keyAuth)

	qry, err := CheckRecordNameExists(d.DnsToken, d.ZoneID, info.FQDN)
	if err != nil {
		slog.Error("error in InfraCfCustomProvider checking of txt record name exists", slog.String("error", err.Error()), slog.String("infoFQDN", info.FQDN))
		return err
	}

	switch qry.Exists {
	case true:
		return fmt.Errorf("txt record already exists, please remove it")
	default:
		params := cloudflare.CreateDNSRecordParams{Name: info.FQDN, Content: info.Value, TTL: d.TTL, Type: "TXT"}
		record, err := CreateCloudflareDnsRecord(d.DnsToken, d.ZoneID, params)
		if err != nil {
			slog.Error("error creating dns record", slog.String("error", err.Error()), slog.String("infofqdn", info.FQDN))
			return err
		}
		d.NewTxtRecordId = record.ID
		d.CreatedChallengeTXT = true

	}
	return err
}

func (d *InfraCfCustomDNSProvider) CleanUp(domain, token, keyAuth string) error {
	if len(d.ZoneID) < 1 {
		err := d.SetZoneId(domain)
		if err != nil {
			slog.Error("error during Present retrieving zone id from domain name", slog.String("domain", domain), slog.String("error", err.Error()))
			return err
		}
	}

	if d.CreatedChallengeTXT {
		if len(d.NewTxtRecordId) < 1 {
			return fmt.Errorf("no txt record id specified for cleanup")
		}
		err := DeleteCloudFlareDnsRecord(d.DnsToken, d.ZoneID, d.NewTxtRecordId)
		if err != nil {
			slog.Error("error deleting txt record id during cleanup", slog.String("error", err.Error()), slog.String("id", d.NewTxtRecordId), slog.String("zoneId", d.ZoneID))
			return err
		}
		d.CreatedChallengeTXT = false
		d.NewTxtRecordId = ""
		return err
	}
	return fmt.Errorf("error during cleanup, CreatedChallengeTXT set to false")
}

// getRootDomain extracts the root domain from a given subdomain.
func getRootDomain(subdomain string) string {
	parts := strings.Split(subdomain, ".")
	if len(parts) < 2 {
		return subdomain // Return as is if it's not a valid domain format
	}

	// Handle wildcard prefixes like "*.example.com"
	if parts[0] == "*" {
		parts = parts[1:]
	}

	// Return the last two segments as the root domain
	return strings.Join(parts[len(parts)-2:], ".")
}
