package erebus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Store struct {
	client     *s3.Client
	bucket     string
	localCache string
	uploader   *manager.Uploader
	downloader *manager.Downloader
}

func NewS3Store(ctx context.Context, endpoint, region, bucket, accessKey, secretKey, localCache string) (*S3Store, error) {
	// Create custom resolver for MinIO/S3 compatible endpoints
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if endpoint != "" {
			return aws.Endpoint{
				PartitionID:   "aws",
				URL:           endpoint,
				SigningRegion: region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.UsePathStyle = true // Required for MinIO usually
		}
	})

	// Ensure local cache directory exists
	if err := os.MkdirAll(localCache, 0755); err != nil {
		return nil, fmt.Errorf("failed to create local cache dir: %w", err)
	}

	return &S3Store{
		client:     client,
		bucket:     bucket,
		localCache: localCache,
		uploader:   manager.NewUploader(client),
		downloader: manager.NewDownloader(client),
	}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, r io.Reader) error {
	// Upload to S3
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("failed to upload to s3: %w", err)
	}
	return nil
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	localPath := filepath.Join(s.localCache, key)

	// Check if exists locally
	if _, err := os.Stat(localPath); err == nil {
		return os.Open(localPath)
	}

	// Not found locally, download from S3
	// Ensure parent dir exists
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent dir: %w", err)
	}

	// Download to temp file first
	tmpFile, err := os.CreateTemp(filepath.Dir(localPath), "download-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up if we fail before rename
	defer tmpFile.Close()

	_, err = s.downloader.Download(ctx, tmpFile, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to download from s3: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile.Name(), localPath); err != nil {
		return nil, fmt.Errorf("failed to rename temp file to local cache: %w", err)
	}

	// Open the local file
	return os.Open(localPath)
}

func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return false, nil
		}
		// Also check for 404 in other ways if needed, but NoSuchKey is standard
		return false, nil // Assume not found or error prevents finding
	}
	return true, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from s3: %w", err)
	}

	// Also try to delete from local cache if present
	localPath := filepath.Join(s.localCache, key)
	_ = os.Remove(localPath)

	return nil
}
