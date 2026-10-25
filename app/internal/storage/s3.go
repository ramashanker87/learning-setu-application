package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"io"
	"learning-setu/app/internal/config"
	"strings"
	"time"
)

type Upload struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Expires int               `json:"expiresInSeconds"`
}
type Object struct {
	Version string `json:"version"`
	ETag    string `json:"etag"`
}
type Objects interface {
	EnsureBucket(context.Context, string, string) error
	Upload(context.Context, string, string, string, int64) (Upload, error)
	Inspect(context.Context, string, string, string, int64) (Object, error)
	Read(context.Context, string, string, string) ([]byte, error)
	Playback(context.Context, string, string, string) (string, error)
	DeletePrefix(context.Context, string, string) error
	Put(context.Context, string, string, string, []byte) (Object, error)
	DeleteTenantBucket(context.Context, string, string) error
}
type S3 struct {
	Client *s3.Client
	Public *s3.PresignClient
	Config config.Config
}

func New(ctx context.Context, c config.Config) (*S3, error) {
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(c.Region)}
	if c.S3Endpoint != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("local", "local", "")))
	}
	ac, e := awsconfig.LoadDefaultConfig(ctx, opts...)
	if e != nil {
		return nil, e
	}
	makeClient := func(endpoint string) *s3.Client {
		return s3.NewFromConfig(ac, func(o *s3.Options) {
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			if endpoint != "" {
				o.BaseEndpoint = &endpoint
				o.UsePathStyle = true
			}
		})
	}
	public := c.S3PublicEndpoint
	if public == "" {
		public = c.S3Endpoint
	}
	return &S3{Client: makeClient(c.S3Endpoint), Public: s3.NewPresignClient(makeClient(public)), Config: c}, nil
}
func missing(e error) bool {
	var api smithy.APIError
	return errors.As(e, &api) && (api.ErrorCode() == "NotFound" || api.ErrorCode() == "NoSuchBucket" || api.ErrorCode() == "404")
}
func (s *S3) EnsureBucket(ctx context.Context, bucket, org string) error {
	_, e := s.Client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &bucket})
	if e != nil && !missing(e) {
		return e
	}
	if e != nil {
		if !s.Config.Local() {
			n := 0
			pg := s3.NewListBucketsPaginator(s.Client, &s3.ListBucketsInput{})
			for pg.HasMorePages() {
				r, e := pg.NextPage(ctx)
				if e != nil {
					return e
				}
				n += len(r.Buckets)
			}
			if n >= s.Config.BucketQuota-5 {
				return fmt.Errorf("bucket quota headroom exhausted")
			}
		}
		input := &s3.CreateBucketInput{Bucket: &bucket}
		if s.Config.Region != "us-east-1" {
			input.CreateBucketConfiguration = &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(s.Config.Region)}
		}
		if _, e = s.Client.CreateBucket(ctx, input); e != nil {
			return e
		}
		if _, e = s.Client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{Bucket: &bucket, Tagging: &types.Tagging{TagSet: []types.Tag{{Key: aws.String("Organization"), Value: &org}}}}); e != nil {
			return e
		}
	} else {
		tags, e := s.Client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: &bucket})
		if e != nil {
			return e
		}
		owned := false
		for _, t := range tags.TagSet {
			if aws.ToString(t.Key) == "Organization" && aws.ToString(t.Value) == org {
				owned = true
			}
		}
		if !owned {
			return fmt.Errorf("bucket ownership collision")
		}
	}
	if _, e = s.Client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: &bucket, VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}}); e != nil {
		return e
	}
	if _, e = s.Client.PutBucketCors(ctx, &s3.PutBucketCorsInput{Bucket: &bucket, CORSConfiguration: &types.CORSConfiguration{CORSRules: []types.CORSRule{{AllowedOrigins: []string{s.Config.Origin}, AllowedMethods: []string{"PUT", "GET"}, AllowedHeaders: []string{"*"}, ExposeHeaders: []string{"ETag"}}}}}); e != nil {
		return e
	}
	if !s.Config.Local() {
		if _, e = s.Client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{Bucket: &bucket, PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{BlockPublicAcls: aws.Bool(true), IgnorePublicAcls: aws.Bool(true), BlockPublicPolicy: aws.Bool(true), RestrictPublicBuckets: aws.Bool(true)}}); e != nil {
			return e
		}
		if _, e = s.Client.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{Bucket: &bucket, ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{Rules: []types.ServerSideEncryptionRule{{ApplyServerSideEncryptionByDefault: &types.ServerSideEncryptionByDefault{SSEAlgorithm: types.ServerSideEncryptionAes256}}}}}); e != nil {
			return e
		}
		policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Deny", "Principal": "*", "Action": "s3:*", "Resource": []string{"arn:aws:s3:::" + bucket, "arn:aws:s3:::" + bucket + "/*"}, "Condition": map[string]any{"Bool": map[string]any{"aws:SecureTransport": "false"}}}}})
		if _, e = s.Client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: &bucket, Policy: aws.String(string(policy))}); e != nil {
			return e
		}
	}
	return nil
}
func (s *S3) Upload(ctx context.Context, b, k, t string, size int64) (Upload, error) {
	p, e := s.Public.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: &b, Key: &k, ContentType: &t, ContentLength: &size, IfNoneMatch: aws.String("*")}, func(o *s3.PresignOptions) { o.Expires = 2 * time.Minute })
	if e != nil {
		return Upload{}, e
	}
	return Upload{URL: p.URL, Method: "PUT", Headers: map[string]string{"Content-Type": t, "If-None-Match": "*"}, Expires: 120}, nil
}
func (s *S3) Inspect(ctx context.Context, b, k, t string, size int64) (Object, error) {
	h, e := s.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &b, Key: &k})
	if e != nil {
		return Object{}, e
	}
	if aws.ToInt64(h.ContentLength) != size || aws.ToString(h.ContentType) != t || aws.ToString(h.VersionId) == "" {
		return Object{}, fmt.Errorf("uploaded file does not match registered metadata")
	}
	return Object{aws.ToString(h.VersionId), aws.ToString(h.ETag)}, nil
}
func (s *S3) Read(ctx context.Context, b, k, v string) ([]byte, error) {
	i := &s3.GetObjectInput{Bucket: &b, Key: &k}
	if v != "" {
		i.VersionId = &v
	}
	r, e := s.Client.GetObject(ctx, i)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	data, e := io.ReadAll(io.LimitReader(r.Body, 26*1024*1024))
	if len(data) > 25*1024*1024 {
		return nil, fmt.Errorf("object exceeds limit")
	}
	return data, e
}
func (s *S3) Playback(ctx context.Context, b, k, v string) (string, error) {
	r, e := s.Public.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &b, Key: &k, VersionId: &v}, func(o *s3.PresignOptions) { o.Expires = time.Minute })
	if e != nil {
		return "", e
	}
	return r.URL, nil
}
func (s *S3) Put(ctx context.Context, b, k, t string, data []byte) (Object, error) {
	r, e := s.Client.PutObject(ctx, &s3.PutObjectInput{Bucket: &b, Key: &k, ContentType: &t, Body: bytes.NewReader(data), IfNoneMatch: aws.String("*")})
	if e != nil {
		return Object{}, e
	}
	return Object{aws.ToString(r.VersionId), aws.ToString(r.ETag)}, nil
}
func (s *S3) DeletePrefix(ctx context.Context, b, prefix string) error {
	// Re-read the first page after each deletion: no pagination marker points at
	// a version we just removed, and a retry resumes from the remaining objects.
	for {
		r, e := s.Client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: &b, Prefix: &prefix})
		if e != nil {
			return e
		}
		objects := []types.ObjectIdentifier{}
		for _, v := range r.Versions {
			objects = append(objects, types.ObjectIdentifier{Key: v.Key, VersionId: v.VersionId})
		}
		for _, v := range r.DeleteMarkers {
			objects = append(objects, types.ObjectIdentifier{Key: v.Key, VersionId: v.VersionId})
		}
		if len(objects) == 0 {
			return nil
		}
		if len(objects) > 0 {
			r, e := s.Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: &b, Delete: &types.Delete{Objects: objects}})
			if e != nil {
				return e
			}
			if len(r.Errors) > 0 {
				return fmt.Errorf("object version deletion incomplete")
			}
		}
	}
}

func (s *S3) DeleteTenantBucket(ctx context.Context, bucket, org string) error {
	if org == "" || bucket == s.Config.DocumentBucket || !strings.HasPrefix(bucket, s.Config.Prefix+"-") || !strings.Contains(bucket, "-"+strings.ReplaceAll(org, "-", "")+"-") {
		return fmt.Errorf("unsafe tenant bucket mapping")
	}
	tags, err := s.Client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: &bucket})
	if missing(err) {
		return nil
	}
	if err != nil {
		return err
	}
	owned := false
	for _, tag := range tags.TagSet {
		owned = owned || (aws.ToString(tag.Key) == "Organization" && aws.ToString(tag.Value) == org)
	}
	if !owned {
		return fmt.Errorf("bucket ownership collision")
	}
	// Stop incomplete uploads as well as removing every version/delete marker.
	for {
		page, err := s.Client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: &bucket})
		if err != nil {
			return err
		}
		if len(page.Uploads) == 0 {
			break
		}
		for _, upload := range page.Uploads {
			if _, err = s.Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: &bucket, Key: upload.Key, UploadId: upload.UploadId}); err != nil {
				return err
			}
		}
	}
	if err = s.DeletePrefix(ctx, bucket, ""); err != nil {
		return err
	}
	_, err = s.Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: &bucket})
	if missing(err) {
		return nil
	}
	return err
}
