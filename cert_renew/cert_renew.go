package cert_renew

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/babbage88/infra-core/internal/acmecli/cloud_providers/cf_acme"
)

var (
	certPrefix = "-----BEGIN CERTIFICATE-----\n"
	certSuffix = "\n-----END CERTIFICATE-----\n"
	keyPrefix  = "-----BEGIN PRIVATE KEY-----\n"
	keySuffix  = "\n-----END PRIVATE KEY-----\n"
)

type CertificateDataRenewResponse struct {
	Body CertificateData
}

type CertificateData struct {
	DomainNames   []string `json:"domainName"`
	CertPEM       string   `json:"cert_pem"`
	ChainPEM      string   `json:"chain_pem"`
	Fullchain     string   `json:"fullchain_pem"`
	PrivKey       string   `json:"priv_key"`
	ZipDir        string   `json:"zipDir"`
	S3DownloadUrl string   `json:"s3DownloadUrl"`
}

type CertDnsRenewReqWrapper struct {
	Body CertDnsRenewReq `json:"body"`
}

type CertDnsRenewReq struct {
	DomainNames          []string      `json:"domainName"`
	AcmeEmail            string        `json:"acmeEmail"`
	AcmeUrl              string        `json:"acmeUrl"`
	ZipDir               string        `json:"zipDir"`
	PushS3               bool          `json:"pushS3"`
	Token                string        `json:"token"`
	RecursiveNameServers []string      `json:"recurseServers"`
	Timeout              time.Duration `json:"timeout"`
}

func (c *CertDnsRenewReq) InitAcmeRenewRequest() *cf_acme.CertificateRenewalRequest {
	return &cf_acme.CertificateRenewalRequest{
		DomainNames:          c.DomainNames,
		AcmeEmail:            c.AcmeEmail,
		AcmeUrl:              c.AcmeUrl,
		PushS3:               c.PushS3,
		ZipDir:               c.ZipDir,
		Token:                c.Token,
		RecursiveNameServers: c.RecursiveNameServers,
		Timeout:              c.Timeout,
	}
}

func (c *CertificateData) ParseAcmeCertStruct(acmeCert *cf_acme.CertificateData) {
	c.DomainNames = acmeCert.DomainNames
	c.CertPEM = acmeCert.CertPEM
	c.ChainPEM = acmeCert.ChainPEM
	c.Fullchain = acmeCert.Fullchain
	c.PrivKey = acmeCert.PrivKey
	c.ZipDir = acmeCert.ZipDir
	c.S3DownloadUrl = acmeCert.S3DownloadUrl
}

func (c *CertificateData) TrimJsonCertificateData() {
	certTrimmed, err := readAndTrimCert(c.CertPEM, certPrefix, certSuffix)
	if err == nil {
		slog.Info("Trimming certificate prefix/suffix json")
		c.CertPEM = certTrimmed
	}
	priKeyStr, err := readAndTrimCert(c.PrivKey, keyPrefix, keySuffix)
	if err == nil {
		slog.Info("Trimming key prefix for json")
		c.PrivKey = priKeyStr
	}
	chainStrTrim, err := readAndTrimCert(c.ChainPEM, certPrefix, certSuffix)
	if err == nil {
		slog.Info("Trimming certificate prefix/suffix json")
		c.ChainPEM = chainStrTrim
	}
}

type Renewal interface {
	Renew(token string, recursiveNameservers []string, timeout time.Duration) cf_acme.CertificateData
}

type CertRenewReq interface {
	GetDomainName() string
}

func (c *CertDnsRenewReq) ZipFileName() string {
	var retVal strings.Builder
	retVal.WriteString("__certs__")
	if c.ZipDir == "" {
		retVal.WriteString(strings.TrimPrefix(c.DomainNames[0], "*"))
		retVal.WriteString(".zip")
		slog.Info("generated zipfile name", slog.String("zipfilename", retVal.String()))
		return retVal.String()
	}

	retVal.WriteString(c.ZipDir)
	slog.Info("generated zipfile name", slog.String("zipfilename", retVal.String()))
	return retVal.String()
}

func (c *CertDnsRenewReq) KubeSecretName() string {
	var retVal strings.Builder
	retVal.WriteString(strings.TrimPrefix(strings.ReplaceAll(c.DomainNames[0], ".", "-"), "*-"))
	retVal.WriteString("-cert")
	slog.Info("generated zipfile name", slog.String("zipfilename", retVal.String()))
	return retVal.String()
}

func (c *CertDnsRenewReq) Renew() (certData *CertificateData, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered in cf_acme call", slog.Any("panic", r))
			err = fmt.Errorf("panic in certificate renewal: %v", r)
		}
	}()

	certData = &CertificateData{}
	acmeRenewal := c.InitAcmeRenewRequest()
	certificates, err := acmeRenewal.Renew(c.Token, c.RecursiveNameServers, c.Timeout)
	if err != nil {
		slog.Error("error renewing certificate", slog.String("error", err.Error()))
		return certData, err
	}

	if c.PushS3 {
		manifest := NewKubeTlsSecretManifest(certificates.CertPEM, certificates.PrivKey, c.KubeSecretName())
		out, yamlErr := manifest.ToYaml()
		if yamlErr != nil {
			return certData, yamlErr
		}

		files := map[string][]byte{
			"kube_secret.yaml": out,
		}
		certificates.PushCertBufferToS3WithFiles(c.ZipFileName(), files)

		if certificates.ZipDir != "" {
			removeErr := os.Remove(certificates.ZipDir)
			if removeErr != nil {
				slog.Error("error removing zip file", slog.String("error", removeErr.Error()))
			}
		}
	}

	certData.ParseAcmeCertStruct(&certificates)

	return certData, err
}

func (c *CertDnsRenewReq) Validate() error {
	if len(c.DomainNames) == 0 {
		return fmt.Errorf("at least one domain name is required")
	}
	if strings.TrimSpace(c.AcmeEmail) == "" {
		return fmt.Errorf("acmeEmail is required")
	}
	if strings.TrimSpace(c.AcmeUrl) == "" {
		return fmt.Errorf("acmeUrl is required")
	}
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("token is required")
	}
	return nil
}

func (c *CertDnsRenewReq) NormalizeTimeout() {
	switch {
	case c.Timeout <= 0:
		c.Timeout = 120 * time.Second
	case c.Timeout < time.Second:
		c.Timeout *= time.Second
	}
}

func readAndTrimCert(s string, beginMarker string, endMarker string) (string, error) {
	s = strings.ReplaceAll(s, beginMarker, "")
	s = strings.ReplaceAll(s, endMarker, "")
	slog.Info("Parsing finished", slog.String("Content", s))

	return strings.TrimSpace(s), nil
}
