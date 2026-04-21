package cf_acme

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/babbage88/infra-core/internal/acmecli/internal/pretty"
	"github.com/babbage88/infra-core/internal/acmecli/storage/goinfra_minio"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/joho/godotenv"
)

type InfraDnsChallengeSolver interface{}

func (u *AcmeUser) GetEmail() string {
	return u.Email
}

func (u *AcmeUser) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *AcmeUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func (c *CertificateRenewalRequest) InitialzeClientandPovider(token string, recursiveNameServers []string, timeout time.Duration) (*lego.Client, *AcmeUser, error) {
	// Create a user. New accounts need an email and private key to start.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		slog.Error("error creating private key", slog.String("error", err.Error()))
		return &lego.Client{}, &AcmeUser{}, err
	}

	acmeUser := AcmeUser{
		Email: c.AcmeEmail,
		key:   privateKey,
	}

	config := lego.NewConfig(&acmeUser)

	// This CA URL is configured for a local dev instance of Boulder running in Docker in a VM.
	config.CADirURL = c.AcmeUrl
	config.Certificate.KeyType = certcrypto.RSA2048

	// A client facilitates communication with the CA server.
	client, err := lego.NewClient(config)
	if err != nil {
		slog.Error("error creating client", slog.String("error", err.Error()))
		return &lego.Client{}, &acmeUser, err
	}
	/*
		provider, err := lego_cloudflare.NewDNSProviderConfig(&lego_cloudflare.Config{
			AuthToken:          token,
			ZoneToken:          token,
			TTL:                int(120),
			PropagationTimeout: timeout,
		})
	*/
	provider, err := NewInfraCfCustomDNSProvider(token, token, recursiveNameServers, timeout)
	if err != nil {
		slog.Error("error initializing cloudflare DNS challenge provider", slog.String("error", err.Error()))
		return &lego.Client{}, &acmeUser, err
	}
	recursiveServersOption := dns01.AddRecursiveNameservers(recursiveNameServers)
	timeoutOption := dns01.AddDNSTimeout(timeout)
	err = client.Challenge.SetDNS01Provider(provider, recursiveServersOption, timeoutOption)
	if err != nil {
		slog.Error("Failed to set DNS challenge.", slog.String("error", err.Error()))
		return &lego.Client{}, &acmeUser, err
	}
	return client, &acmeUser, err
}

func (c *CertificateRenewalRequest) RenewCertWithDnsFromEnv() (CertificateData, error) {
	certdata := &CertificateData{DomainNames: c.DomainNames}
	token := os.Getenv("CLOUDFLARE_DNS_API_TOKEN")
	nameServers := []string{"1.1.1.1", "1.0.0.1"}
	timeout := 60 * time.Second
	client, acmeUser, err := c.InitialzeClientandPovider(token, nameServers, timeout)
	if err != nil {
		slog.Error("Error initializing client", slog.String("error", err.Error()))
		return *certdata, err
	}
	// New users will need to register
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		slog.Error("Error creating registration", slog.String("error", err.Error()))
		return *certdata, err
	}

	acmeUser.Registration = reg

	request := certificate.ObtainRequest{
		Domains: c.DomainNames,
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatal(err)
	}

	cert := string(certificates.Certificate)
	privKey := string(certificates.PrivateKey)
	issuerCA := string(certificates.IssuerCertificate)
	fullChain := fmt.Sprint(cert, issuerCA)
	certdata.CertPEM = cert
	certdata.PrivKey = privKey
	certdata.Fullchain = fullChain
	certdata.FullchainAndKey = fmt.Sprint(fullChain, privKey)
	certdata.ZipDir = c.ZipDir

	return *certdata, err
}

func (c *CertificateRenewalRequest) CliRenewal() (CertificateData, error) {
	err := godotenv.Load(c.EnvFile)
	if err != nil {
		slog.Error("Error loading .env file", slog.String("error", err.Error()))
		return CertificateData{DomainNames: c.DomainNames}, err
	}

	certData, err := c.Renew(c.Token, c.RecursiveNameServers, c.Timeout)
	if err != nil {
		slog.Error("error renewing certificate", slog.String("error", err.Error()))
		return CertificateData{DomainNames: c.DomainNames}, err
	}

	if c.SaveZip {
		err := saveToZip(c.ZipDir, []byte(certData.CertPEM), []byte(certData.PrivKey), []byte(certData.Fullchain))
		if err != nil {
			slog.Error("error saving zip", slog.String("error", err.Error()))
			return certData, err
		}
	}

	if c.PushS3 {
		certData.PushZipDirToS3(c.ZipDir)
	}
	printJson(certData)

	if c.SaveZip {
		pretty.Printf("Zip File location: %s", c.ZipDir)
	}
	return certData, err
}

func (c *CertificateRenewalRequest) Renew(token string, recursiveNameservers []string, timeout time.Duration) (CertificateData, error) {
	client, acmeUser, err := c.InitialzeClientandPovider(token, recursiveNameservers, timeout)
	if err != nil {
		slog.Error("error initializing ACME client", slog.String("error", err.Error()))
		return CertificateData{}, err
	}

	// New users will need to register
	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		slog.Error("Error creating registration", slog.String("error", err.Error()))
		return CertificateData{}, err
	}

	acmeUser.Registration = reg

	request := certificate.ObtainRequest{
		Domains: c.DomainNames,
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatal(err)
	}
	cert := string(certificates.Certificate)
	privKey := string(certificates.PrivateKey)
	issuerCA := string(certificates.IssuerCertificate)
	fullChain := fmt.Sprint(cert, issuerCA)

	certdata := CertificateData{
		DomainNames:     c.DomainNames,
		CertPEM:         cert,
		ChainPEM:        issuerCA,
		PrivKey:         privKey,
		Fullchain:       fullChain,
		FullchainAndKey: fmt.Sprint(fullChain, privKey),
		ZipDir:          c.ZipDir,
	}

	return certdata, err
}

func (c *CertificateData) SaveToZip(path string) error {
	err := saveToZip(path, []byte(c.CertPEM), []byte(c.PrivKey), []byte(c.ChainPEM))
	if err != nil {
		slog.Error("error saving zip", slog.String("error", err.Error()))
	}
	return err
}

func (c *CertificateData) SaveToZipBuffer(objectName string) (*bytes.Buffer, error) {
	buf, err := saveZipToBuffer(objectName, []byte(c.CertPEM), []byte(c.PrivKey), []byte(c.ChainPEM))
	if err != nil {
		slog.Error("error saving zip to buffer", slog.String("error", err.Error()))
	}
	return buf, err
}

func (c *CertificateData) SaveFilesWithCertsToZipBuffer(objectName string, files map[string][]byte) (*bytes.Buffer, error) {
	files["certificate.pem"] = []byte(c.CertPEM)
	files["privkey.pem"] = []byte(c.PrivKey)
	files["issuer.pem"] = []byte(c.ChainPEM)

	zipBuffer, err := saveFilesToZipBuffer(objectName, files)
	return zipBuffer, err
}

func (c *CertificateData) PushZipDirToS3(objName string) error {
	s3client, err := goinfra_minio.NewS3ClientFromEnv()
	if err != nil {
		slog.Error("error initializing client", slog.String("error", err.Error()))
		return err
	}
	_, pushErr := s3client.PushFileToDefaultBucket(objName, c.ZipDir)
	if pushErr != nil {
		err = pushErr
		slog.Error("error pushing file to s3", slog.String("error", err.Error()), slog.String("sourceFile", c.ZipDir))
	}
	expiry := 15 * time.Minute
	presignedUrl, err := s3client.Client.PresignedGetObject(context.Background(), s3client.DefaultBucketName, objName, expiry, nil)
	if err != nil {
		slog.Error("Error generating presigned download URL", slog.String("error", err.Error()))
	}
	c.S3DownloadUrl = presignedUrl.String()
	return err
}

func (c *CertificateData) PushCertBufferToS3WithFiles(objName string, files map[string][]byte) error {
	s3client, err := goinfra_minio.NewS3ClientFromEnv()
	if err != nil {
		slog.Error("error initializing client", slog.String("error", err.Error()))
		return err
	}

	buf, err := c.SaveFilesWithCertsToZipBuffer(objName, files)
	if err != nil {
		slog.Error("error saving zip to buffer", slog.String("error", err.Error()))
	}
	_, pushErr := s3client.PushBufferToDefaultBucket(objName, buf)
	if pushErr != nil {
		err = pushErr
		slog.Error("error pushing buffer to s3", slog.String("error", err.Error()), slog.String("sourceFile", c.ZipDir))
	}
	expiry := 15 * time.Minute
	presignedUrl, err := s3client.Client.PresignedGetObject(context.Background(), s3client.DefaultBucketName, objName, expiry, nil)
	if err != nil {
		slog.Error("Error generating presigned download URL", slog.String("error", err.Error()))
	}
	c.S3DownloadUrl = presignedUrl.String()
	return err
}

func (c *CertificateData) PushCertBufferToS3(objName string) error {
	s3client, err := goinfra_minio.NewS3ClientFromEnv()
	if err != nil {
		slog.Error("error initializing client", slog.String("error", err.Error()))
		return err
	}

	buf, err := saveZipToBuffer(c.ZipDir, []byte(c.CertPEM), []byte(c.PrivKey), []byte(c.ChainPEM))
	if err != nil {
		slog.Error("error saving zip to buffer", slog.String("error", err.Error()))
	}
	_, pushErr := s3client.PushBufferToDefaultBucket(objName, buf)
	if pushErr != nil {
		err = pushErr
		slog.Error("error pushing buffer to s3", slog.String("error", err.Error()), slog.String("sourceFile", c.ZipDir))
	}
	expiry := 15 * time.Minute
	presignedUrl, err := s3client.Client.PresignedGetObject(context.Background(), s3client.DefaultBucketName, objName, expiry, nil)
	if err != nil {
		slog.Error("Error generating presigned download URL", slog.String("error", err.Error()))
	}
	c.S3DownloadUrl = presignedUrl.String()
	return err
}
