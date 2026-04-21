package goinfra_minio

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3ClientWithBucket struct {
	Client            *minio.Client           `json:"s3Client"`
	Buckets           []minio.BucketInfo      `json:"buckets"`
	DefaultBucketName string                  `json:"defaultBucket"`
	UseSSL            bool                    `json:"useSSL"`
	DefaultGetOptions *minio.GetObjectOptions `json:"defaultGetObjectOptions"`
	DefaultPutOptions *minio.PutObjectOptions `json:"defaultPutObjectOptions"`
}

func NewS3ClientWithBucket(endpoint string, keyID string, accessSecret string, defaultBucketName string, useSSL bool) (*S3ClientWithBucket, error) {
	var err error
	contentTypeOptions := "application/octet-stream"
	s3client := &S3ClientWithBucket{
		UseSSL:            useSSL,
		DefaultPutOptions: &minio.PutObjectOptions{ContentType: contentTypeOptions},
	}

	s3client.Client, err = NewMinioClient(endpoint, keyID, accessSecret, useSSL)

	if err != nil {
		slog.Error("error initializing minio s3 client", slog.String("error", err.Error()))
		return s3client, err
	}

	s3client.Buckets, err = s3client.Client.ListBuckets(context.Background())
	if err != nil {
		slog.Error("error listing buckets", slog.String("error", err.Error()))
		return s3client, err
	}
	checkDefault, bucketErr := s3client.Client.BucketExists(context.Background(), defaultBucketName)
	if bucketErr != nil {
		slog.Error("error verifying default bucket", slog.String("error", bucketErr.Error()))
		err = bucketErr
	}
	if !checkDefault {
		slog.Error("The selected default bucket does not exist", slog.String("defaultBucketName", defaultBucketName))
	}
	s3client.DefaultBucketName = defaultBucketName
	return s3client, err
}

func (s *S3ClientWithBucket) PushFileToDefaultBucket(newObjectName string, sourceFilePath string) (minio.UploadInfo, error) {
	info, err := s.Client.FPutObject(context.Background(), s.DefaultBucketName, newObjectName, sourceFilePath, *s.DefaultPutOptions)
	if err != nil {
		slog.Error("error pushing file to default bucket", slog.String("defaultBucketName", s.DefaultBucketName), slog.String("error", err.Error()))
	}

	return info, err
}

func (s *S3ClientWithBucket) PushBytesToDefaultBucket(newObjectName string, sourceData []byte) (minio.UploadInfo, error) {
	info, err := s.Client.PutObject(context.Background(), s.DefaultBucketName, newObjectName, bytes.NewReader(sourceData), int64(len(sourceData)), minio.PutObjectOptions{})
	if err != nil {
		slog.Error("error pushing file to default bucket", slog.String("defaultBucketName", s.DefaultBucketName), slog.String("error", err.Error()))
	}

	return info, err
}

func (s *S3ClientWithBucket) PushBufferToDefaultBucket(newObjectName string, buf *bytes.Buffer) (minio.UploadInfo, error) {
	info, err := s.Client.PutObject(context.Background(), s.DefaultBucketName, newObjectName, bytes.NewReader(buf.Bytes()), int64(buf.Len()), minio.PutObjectOptions{})
	if err != nil {
		slog.Error("error pushing bytes buffer to default bucket", slog.String("defaultBucketName", s.DefaultBucketName), slog.String("error", err.Error()))
	}

	return info, err
}

func (s *S3ClientWithBucket) GetFromDefaultBucket(objectName string, filePath string) error {
	err := s.Client.FGetObject(context.Background(), s.DefaultBucketName, objectName, filePath, *s.DefaultGetOptions)
	if err != nil {
		slog.Error("error pushing file to default bucket", slog.String("defaultBucketName", s.DefaultBucketName), slog.String("error", err.Error()))
	}

	return err
}

func (s *S3ClientWithBucket) GetObjectFromDefaultBucket(objectName string) (*minio.Object, error) {
	var obj *minio.Object
	obj, err := s.Client.GetObject(context.Background(), s.DefaultBucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		slog.Error("error pushing file to default bucket", slog.String("defaultBucketName", s.DefaultBucketName), slog.String("error", err.Error()))
		return obj, err
	}

	return obj, err
}

func NewMinioClient(endpoint string, keyID string, accessSecret string, useSSL bool) (*minio.Client, error) {
	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(keyID, accessSecret, ""),
		Secure: useSSL,
	})
	if err != nil {
		slog.Error("error initializing minio client", slog.String("endpoint", endpoint))
	}
	slog.Info("minio client created", slog.String("endpoint", endpoint))
	return minioClient, err
}

func NewS3ClientFromEnv() (*S3ClientWithBucket, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	keyID := os.Getenv("S3_KEYID")
	accessSecret := os.Getenv("S3_SECRET")
	defaultBucket := os.Getenv("S3_DEFAULT_BUCKET")
	useSSL := GetenvBool("S3_USESSL", false)
	s3client, err := NewS3ClientWithBucket(endpoint, keyID, accessSecret, defaultBucket, useSSL)
	if err != nil {
		slog.Error("error creating s3 client from env", slog.String("error", err.Error()))
		return s3client, err
	}

	return s3client, err

}

func GetenvBool(name string, defaultVal bool) bool {
	val := os.Getenv(name)
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(val) {
	case "true", "1", "t":
		return true
	case "false", "0", "f":
		return false
	default:
		return defaultVal
	}
}
