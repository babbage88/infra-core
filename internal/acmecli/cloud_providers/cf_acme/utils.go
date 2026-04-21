package cf_acme

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

func printJson(data any) {
	response, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		slog.Error("error marshaling response.", slog.String("error", err.Error()))
	}
	fmt.Printf("%s\n", string(response))
	fmt.Println()
}

func saveToZip(filename string, certPEM []byte, keyPEM []byte, issuerCA []byte) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	files := map[string][]byte{
		"certificate.pem": certPEM,
		"private_key.pem": keyPEM,
		"issuer_ca.pem":   issuerCA,
	}

	for name, content := range files {
		writer, err := zipWriter.Create(name)

		if err != nil {
			return err
		}
		_, err = writer.Write(content)
		if err != nil {
			return err
		}
	}

	return nil
}

func saveZipToBuffer(objectName string, certPEM, keyPEM, issuerCA []byte) (*bytes.Buffer, error) {
	// Create an in-memory buffer
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Define files to be added to the ZIP archive
	files := map[string][]byte{
		"certificate.pem": certPEM,
		"private_key.pem": keyPEM,
		"issuer_ca.pem":   issuerCA,
	}

	// Add each file to the ZIP archive
	for name, content := range files {
		writer, err := zipWriter.Create(name)
		if err != nil {
			return buf, fmt.Errorf("failed to create zip entry %s: %w", name, err)
		}
		if _, err := writer.Write(content); err != nil {
			return buf, fmt.Errorf("failed to write zip entry %s: %w", name, err)
		}
	}

	// Close the ZIP writer to flush data to buffer
	if err := zipWriter.Close(); err != nil {
		return buf, fmt.Errorf("failed to close zip writer: %w", err)
	}
	return buf, nil
}

func saveFilesToZipBuffer(objectName string, files map[string][]byte) (*bytes.Buffer, error) {
	// Create an in-memory buffer
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Add each file to the ZIP archive
	for name, content := range files {
		writer, err := zipWriter.Create(name)
		if err != nil {
			return buf, fmt.Errorf("failed to create zip entry %s: %w", name, err)
		}
		if _, err := writer.Write(content); err != nil {
			return buf, fmt.Errorf("failed to write zip entry %s: %w", name, err)
		}
	}

	// Close the ZIP writer to flush data to buffer
	if err := zipWriter.Close(); err != nil {
		return buf, fmt.Errorf("failed to close zip writer: %w", err)
	}
	return buf, nil
}
