package cf_acme

import (
	"crypto"
	"time"

	"github.com/go-acme/lego/v4/registration"
)

type CertificateData struct {
	DomainNames     []string `json:"domainName"`
	CertPEM         string   `json:"cert_pem"`
	ChainPEM        string   `json:"chain_pem"`
	Fullchain       string   `json:"fullchain_pem"`
	FullchainAndKey string   `json:"fullchain_and_key"`
	PrivKey         string   `json:"priv_key"`
	ZipDir          string   `json:"zipDir"`
	S3DownloadUrl   string   `json:"s3DownloadUrl"`
}

type CertificateRenewalRequest struct {
	EnvFile              string        `json:"envFile"`
	DomainNames          []string      `json:"domainName"`
	AcmeEmail            string        `json:"acmeEmail"`
	AcmeUrl              string        `json:"acmeUrl"`
	SaveZip              bool          `json:"saveZip"`
	ZipDir               string        `json:"zipDir"`
	PushS3               bool          `json:"pushS3"`
	TTL                  int           `json:"ttl"`
	Token                string        `json:"token"`
	RecursiveNameServers []string      `json:"recurseServers"`
	Timeout              time.Duration `json:"timeout"`
}

type AcmeUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}
