package cf_acme

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cloudflare/cloudflare-go"
)

type RecordQueryResult struct {
	RecordName string                 `json:"recordName"`
	Exists     bool                   `json:"exists"`
	Records    []cloudflare.DNSRecord `json:"records"`
	Count      int                    `json:"count"`
}

func GetCloudflareZoneIdFromDomainName(token string, domainName string) (string, error) {
	api, err := cloudflare.NewWithAPIToken(token)
	if err != nil {
		slog.Error("Error initializing cf api client. Verify token.")
		return "", err
	}

	zoneID, err := api.ZoneIDByName(getRootDomain(domainName))
	if err != nil {
		msg := fmt.Sprintf("Error retrieving ZoneId for Domain: %s error: %s", domainName, err.Error())
		slog.Error(msg)
		return zoneID, err
	}

	return zoneID, err
}

func CheckRecordNameExists(token string, zoneId string, domainName string) (RecordQueryResult, error) {
	api, err := cloudflare.NewWithAPIToken(token)
	if err != nil {
		slog.Error("Error initializing cf api client. Verify token.")
		return RecordQueryResult{}, err
	}

	records, _, err := api.ListDNSRecords(context.Background(), cloudflare.ZoneIdentifier(zoneId), cloudflare.ListDNSRecordsParams{Type: "TXT", Name: domainName})
	if err != nil {
		return RecordQueryResult{}, err
	}
	qryResult := RecordQueryResult{
		Records: records,
		Count:   len(records),
		Exists:  len(records) > 0,
	}

	return qryResult, nil
}

func UpdateCloudflareDnsRecord(token string, zoneId string, recordUpdateParams cloudflare.UpdateDNSRecordParams) (cloudflare.DNSRecord, error) {
	record := cloudflare.DNSRecord{}
	api, err := cloudflare.NewWithAPIToken(token)
	if err != nil {
		slog.Error("error initializing cf api client. Verify token")
		return record, err
	}

	record, err = api.UpdateDNSRecord(context.Background(), cloudflare.ZoneIdentifier(zoneId), recordUpdateParams)
	if err != nil {
		slog.Error("error updating dns record", slog.String("error", err.Error()), slog.String("RecordID", record.ID))
		return record, err
	}
	return record, err
}

func CreateCloudflareDnsRecord(token string, zoneId string, recordCreateParams cloudflare.CreateDNSRecordParams) (cloudflare.DNSRecord, error) {
	record := cloudflare.DNSRecord{}
	api, err := cloudflare.NewWithAPIToken(token)
	if err != nil {
		slog.Error("error initializing cf api client. Verify token")
		return record, err
	}

	record, err = api.CreateDNSRecord(context.Background(), cloudflare.ZoneIdentifier(zoneId), recordCreateParams)
	if err != nil {
		slog.Error("error creatomg dns record", slog.String("error", err.Error()), slog.String("RecordID", record.ID))
		return record, err
	}
	return record, err
}

func DeleteCloudFlareDnsRecord(token string, zoneId string, recordId string) error {
	api, err := cloudflare.NewWithAPIToken(token)
	if err != nil {
		slog.Error("error initializing cf api client. Verify token", slog.String("error", err.Error()))
		return err
	}
	slog.Info("Deleting Cloudflare DNS Record", slog.String("ZoneID", zoneId), slog.String("RecordID", recordId))
	err = api.DeleteDNSRecord(context.Background(), cloudflare.ZoneIdentifier(zoneId), recordId)
	return err
}
