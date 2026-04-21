package cert_renew

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"

	"github.com/goccy/go-yaml"
)

const (
	TlsSecretKind       = "Secret"
	TlsSecretApiVersion = "v1"
	TlsSecretType       = "kubernetes.io/tls"
)

type TlsSecretMetaData struct {
	Name string `json:"name" yaml:"name"`
}

type KubeSecretManifest struct {
	ApiVersion string            `json:"apiVersion" yaml:"apiVersion"`
	Data       map[string]string `json:"data" yaml:"data"`
	Kind       string            `json:"kind" yaml:"kind"`
	Metadata   TlsSecretMetaData `json:"metadata" yaml:"metadata"`
	Type       string            `json:"type" yaml:"type"`
}

func NewKubeTlsSecretManifest(tlsCrt string, tlsKey string, secretName string) *KubeSecretManifest {
	base64EncodedTlsCrt := base64.StdEncoding.EncodeToString([]byte(tlsCrt))
	base64EncodedTlsKey := base64.StdEncoding.EncodeToString([]byte(tlsKey))
	data := map[string]string{
		"tls.crt": base64EncodedTlsCrt,
		"tls.key": base64EncodedTlsKey,
	}

	return &KubeSecretManifest{
		ApiVersion: TlsSecretApiVersion,
		Data:       data,
		Kind:       TlsSecretKind,
		Metadata:   TlsSecretMetaData{Name: secretName},
		Type:       TlsSecretType,
	}
}

func (k *KubeSecretManifest) ToYaml() ([]byte, error) {
	out, err := yaml.Marshal(k)
	if err != nil {
		slog.Error("error marshaling to yaml", slog.String("error", err.Error()))
		return out, err
	}

	fmt.Println(string(out))
	return out, err
}

func (k *KubeSecretManifest) ExportYaml(path string) (int, error) {
	out, err := k.ToYaml()
	if err != nil {
		slog.Error("error marshaling to yaml", slog.String("error", err.Error()))
		return 0, err
	}

	f, err := os.Create(path)
	if err != nil {
		slog.Error("error creating file", slog.String("error", err.Error()))
		return 0, err
	}
	defer f.Close()

	l, err := f.Write(out)
	if err != nil {
		slog.Error("error writing file", slog.String("error", err.Error()))
	}
	return l, err
}
