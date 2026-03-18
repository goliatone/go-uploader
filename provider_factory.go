package uploader

import (
	"context"
	"fmt"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Backend string

const (
	BackendFS    Backend = "fs"
	BackendS3    Backend = "s3"
	BackendMulti Backend = "multi"
)

type FSConfig struct {
	BasePath  string `json:"base_path" koanf:"base_path"`
	URLPrefix string `json:"url_prefix" koanf:"url_prefix"`
}

type S3Config struct {
	Bucket               string `json:"bucket" koanf:"bucket"`
	Region               string `json:"region" koanf:"region"`
	BasePath             string `json:"base_path" koanf:"base_path"`
	EndpointURL          string `json:"endpoint_url" koanf:"endpoint_url"`
	Profile              string `json:"profile" koanf:"profile"`
	AccessKeyID          string `json:"access_key_id" koanf:"access_key_id"`
	SecretAccessKey      string `json:"secret_access_key" koanf:"secret_access_key"`
	SessionToken         string `json:"session_token" koanf:"session_token"`
	UsePathStyle         bool   `json:"use_path_style" koanf:"use_path_style"`
	DisableSSL           bool   `json:"disable_ssl" koanf:"disable_ssl"`
	ServerSideEncryption string `json:"server_side_encryption" koanf:"server_side_encryption"`
	KMSKeyID             string `json:"kms_key_id" koanf:"kms_key_id"`
}

type MultiConfig struct{}

type ProviderConfig struct {
	Backend Backend     `json:"backend" koanf:"backend"`
	FS      FSConfig    `json:"fs" koanf:"fs"`
	S3      S3Config    `json:"s3" koanf:"s3"`
	Multi   MultiConfig `json:"multi" koanf:"multi"`
}

type ProviderFactoryOption func(*providerFactoryOptions)

type providerFactoryOptions struct {
	logger Logger
}

func WithProviderFactoryLogger(logger Logger) ProviderFactoryOption {
	return func(opts *providerFactoryOptions) {
		if opts == nil || logger == nil {
			return
		}
		opts.logger = logger
	}
}

func NewProvider(ctx context.Context, cfg ProviderConfig, opts ...ProviderFactoryOption) (Uploader, error) {
	options := providerFactoryOptions{logger: &DefaultLogger{}}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&options)
	}

	backend, err := ParseBackend(cfg.Backend)
	if err != nil {
		return nil, err
	}

	switch backend {
	case BackendS3:
		return newS3ProviderFromConfig(ctx, cfg.S3, options)
	case BackendMulti:
		local, err := newFSProviderFromConfig(cfg.FS, options)
		if err != nil {
			return nil, err
		}
		remote, err := newS3ProviderFromConfig(ctx, cfg.S3, options)
		if err != nil {
			return nil, err
		}
		return NewMultiProvider(local, remote).WithLogger(options.logger), nil
	case BackendFS:
		fallthrough
	default:
		return newFSProviderFromConfig(cfg.FS, options)
	}
}

func ParseBackend(raw Backend) (Backend, error) {
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case "", string(BackendFS):
		return BackendFS, nil
	case string(BackendS3):
		return BackendS3, nil
	case string(BackendMulti):
		return BackendMulti, nil
	default:
		return "", fmt.Errorf("uploader provider: unsupported backend %q", strings.TrimSpace(string(raw)))
	}
}

func newFSProviderFromConfig(cfg FSConfig, options providerFactoryOptions) (*FSProvider, error) {
	basePath := strings.TrimSpace(cfg.BasePath)
	if basePath == "" {
		return nil, fmt.Errorf("uploader provider: fs.base_path is required")
	}
	provider := NewFSProvider(basePath).WithLogger(options.logger)
	if prefix := strings.TrimSpace(cfg.URLPrefix); prefix != "" {
		provider.WithURLPrefix(prefix)
	}
	return provider, nil
}

func newS3ProviderFromConfig(ctx context.Context, cfg S3Config, options providerFactoryOptions) (*AWSProvider, error) {
	client, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("uploader provider: s3.bucket is required")
	}
	serverSideEncryption, err := NormalizeServerSideEncryption(cfg.ServerSideEncryption)
	if err != nil {
		return nil, err
	}
	provider := NewAWSProvider(client, bucket).WithLogger(options.logger)
	if basePath := strings.TrimSpace(cfg.BasePath); basePath != "" {
		provider.WithBasePath(basePath)
	}
	provider.WithServerSideEncryption(serverSideEncryption, strings.TrimSpace(cfg.KMSKeyID))
	return provider, nil
}

func NormalizeServerSideEncryption(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", nil
	case "aes256", "aes-256", "s3", "sse-s3":
		return string(types.ServerSideEncryptionAes256), nil
	case "aws:kms", "kms", "sse-kms":
		return string(types.ServerSideEncryptionAwsKms), nil
	default:
		return "", fmt.Errorf("uploader provider: unsupported s3.server_side_encryption %q", strings.TrimSpace(raw))
	}
}

func newS3Client(ctx context.Context, cfg S3Config) (*s3.Client, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{}
	if region := strings.TrimSpace(cfg.Region); region != "" {
		loadOptions = append(loadOptions, awsconfig.WithRegion(region))
	}
	if profile := strings.TrimSpace(cfg.Profile); profile != "" {
		loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(profile))
	}
	if accessKeyID := strings.TrimSpace(cfg.AccessKeyID); accessKeyID != "" {
		secretAccessKey := strings.TrimSpace(cfg.SecretAccessKey)
		if secretAccessKey == "" {
			return nil, fmt.Errorf("uploader provider: s3.secret_access_key is required when s3.access_key_id is set")
		}
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID,
			secretAccessKey,
			strings.TrimSpace(cfg.SessionToken),
		)))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("uploader provider: load aws config: %w", err)
	}
	clientOptions := []func(*s3.Options){}
	if endpoint := buildS3EndpointURL(cfg); endpoint != "" {
		clientOptions = append(clientOptions, func(o *s3.Options) {
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = cfg.UsePathStyle
		})
	} else if cfg.UsePathStyle {
		clientOptions = append(clientOptions, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}
	return s3.NewFromConfig(awsCfg, clientOptions...), nil
}

func buildS3EndpointURL(cfg S3Config) string {
	endpoint := strings.TrimSpace(cfg.EndpointURL)
	if endpoint == "" {
		return ""
	}
	if strings.Contains(endpoint, "://") {
		return endpoint
	}
	scheme := "https"
	if cfg.DisableSSL {
		scheme = "http"
	}
	return scheme + "://" + endpoint
}
