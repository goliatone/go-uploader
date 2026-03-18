package uploader

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestNewProviderBuildsFSProvider(t *testing.T) {
	provider, err := NewProvider(context.Background(), ProviderConfig{
		Backend: BackendFS,
		FS: FSConfig{
			BasePath:  t.TempDir(),
			URLPrefix: "/assets",
		},
	})
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	fsProvider, ok := provider.(*FSProvider)
	if !ok {
		t.Fatalf("expected *FSProvider, got %T", provider)
	}
	if fsProvider.base == "" {
		t.Fatalf("expected fs base path to be configured")
	}
	if fsProvider.urlPrefix != "/assets/" {
		t.Fatalf("expected url prefix /assets/, got %q", fsProvider.urlPrefix)
	}
}

func TestNewProviderBuildsS3Provider(t *testing.T) {
	provider, err := NewProvider(context.Background(), ProviderConfig{
		Backend: BackendS3,
		S3: S3Config{
			Bucket:               "bucket-1",
			Region:               "us-west-2",
			BasePath:             "tenant/uploads",
			EndpointURL:          "http://localhost:4566",
			AccessKeyID:          "test",
			SecretAccessKey:      "test",
			UsePathStyle:         true,
			ServerSideEncryption: "aws:kms",
			KMSKeyID:             "kms-key-1",
		},
	})
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	s3Provider, ok := provider.(*AWSProvider)
	if !ok {
		t.Fatalf("expected *AWSProvider, got %T", provider)
	}
	if s3Provider.bucket != "bucket-1" {
		t.Fatalf("expected bucket bucket-1, got %q", s3Provider.bucket)
	}
	if s3Provider.basePath != "tenant/uploads" {
		t.Fatalf("expected base path tenant/uploads, got %q", s3Provider.basePath)
	}
	if s3Provider.serverSideEncryption != "aws:kms" {
		t.Fatalf("expected aws:kms encryption, got %q", s3Provider.serverSideEncryption)
	}
	if s3Provider.kmsKeyID != "kms-key-1" {
		t.Fatalf("expected kms key id kms-key-1, got %q", s3Provider.kmsKeyID)
	}

	options := s3Provider.client.Options()
	if options.Region != "us-west-2" {
		t.Fatalf("expected region us-west-2, got %q", options.Region)
	}
}

func TestNewProviderBuildsMultiProvider(t *testing.T) {
	provider, err := NewProvider(context.Background(), ProviderConfig{
		Backend: BackendMulti,
		FS: FSConfig{
			BasePath: t.TempDir(),
		},
		S3: S3Config{
			Bucket:          "bucket-1",
			Region:          "us-east-1",
			EndpointURL:     "http://localhost:4566",
			AccessKeyID:     "test",
			SecretAccessKey: "test",
			UsePathStyle:    true,
		},
	})
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	multiProvider, ok := provider.(*MultiProvider)
	if !ok {
		t.Fatalf("expected *MultiProvider, got %T", provider)
	}
	if multiProvider.local == nil {
		t.Fatalf("expected local provider to be configured")
	}
	if multiProvider.objectStore == nil {
		t.Fatalf("expected object store to be configured")
	}
}

func TestBuildS3EndpointURL(t *testing.T) {
	cfg := S3Config{EndpointURL: "localhost:4566", DisableSSL: true}
	if got := buildS3EndpointURL(cfg); got != "http://localhost:4566" {
		t.Fatalf("expected http endpoint, got %q", got)
	}
	cfg = S3Config{EndpointURL: "https://localhost:4566"}
	if got := buildS3EndpointURL(cfg); got != "https://localhost:4566" {
		t.Fatalf("expected unchanged endpoint, got %q", got)
	}
}

func TestNewProviderRequiresS3SecretWithStaticAccessKey(t *testing.T) {
	_, err := NewProvider(context.Background(), ProviderConfig{
		Backend: BackendS3,
		S3: S3Config{
			Bucket:      "bucket-1",
			Region:      "us-east-1",
			AccessKeyID: "test",
		},
	})
	if err == nil {
		t.Fatal("expected missing secret access key error")
	}
}

func TestNewProviderRejectsUnsupportedBackend(t *testing.T) {
	_, err := NewProvider(context.Background(), ProviderConfig{
		Backend: Backend("bogus"),
		FS: FSConfig{
			BasePath: t.TempDir(),
		},
	})
	if err == nil {
		t.Fatal("expected unsupported backend error")
	}
}

func TestAWSProviderAppliesConfiguredEncryptionDefaults(t *testing.T) {
	client := &fakeS3Client{
		createMultipartOutput: &s3.CreateMultipartUploadOutput{
			UploadId: aws.String("upload-123"),
		},
	}
	provider := (&AWSProvider{
		client: client,
		bucket: "bucket-1",
	}).
		WithServerSideEncryption("aws:kms", "kms-key-1")

	if _, err := provider.UploadFile(context.Background(), "test.pdf", []byte("pdf")); err != nil {
		t.Fatalf("UploadFile returned error: %v", err)
	}
	if client.lastPutObjectInput == nil {
		t.Fatalf("expected put object input to be captured")
	}
	if client.lastPutObjectInput.ServerSideEncryption != "aws:kms" {
		t.Fatalf("expected aws:kms SSE, got %q", client.lastPutObjectInput.ServerSideEncryption)
	}
	if got := aws.ToString(client.lastPutObjectInput.SSEKMSKeyId); got != "kms-key-1" {
		t.Fatalf("expected kms key id kms-key-1, got %q", got)
	}

	session := &ChunkSession{ID: "1", Key: "path/test.pdf", ProviderData: map[string]any{}}
	if _, err := provider.InitiateChunked(context.Background(), session); err != nil {
		t.Fatalf("InitiateChunked returned error: %v", err)
	}
	if client.lastCreateInput == nil {
		t.Fatalf("expected create multipart input to be captured")
	}
	if client.lastCreateInput.ServerSideEncryption != "aws:kms" {
		t.Fatalf("expected aws:kms SSE on multipart init, got %q", client.lastCreateInput.ServerSideEncryption)
	}
	if got := aws.ToString(client.lastCreateInput.SSEKMSKeyId); got != "kms-key-1" {
		t.Fatalf("expected kms key id kms-key-1 on multipart init, got %q", got)
	}
}

func TestNormalizeServerSideEncryptionAcceptsKMSAlias(t *testing.T) {
	got, err := NormalizeServerSideEncryption("kms")
	if err != nil {
		t.Fatalf("NormalizeServerSideEncryption returned error: %v", err)
	}
	if got != "aws:kms" {
		t.Fatalf("expected aws:kms, got %q", got)
	}
}
