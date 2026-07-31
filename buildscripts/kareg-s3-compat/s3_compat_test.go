package s3compat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

const (
	defaultEndpoint  = "127.0.0.1:9000"
	defaultAccessKey = "smokeadmin"
	defaultSecretKey = "smoke-test-password-123456"
)

func TestS3Compatibility(t *testing.T) {
	client := newClient(t)

	t.Run("versioning", func(t *testing.T) {
		testVersioning(t, client)
	})
	t.Run("multipart", func(t *testing.T) {
		testMultipart(t, client)
	})
	t.Run("presigned-get", func(t *testing.T) {
		testPresignedGet(t, client)
	})
	t.Run("bucket-policy", func(t *testing.T) {
		testBucketPolicy(t, client)
	})
	t.Run("lifecycle", func(t *testing.T) {
		testLifecycle(t, client)
	})
	t.Run("object-lock", func(t *testing.T) {
		testObjectLock(t, client)
	})
}

func newClient(t *testing.T) *minio.Client {
	t.Helper()

	client, err := minio.New(envOrDefault("S3_ENDPOINT", defaultEndpoint), &minio.Options{
		Creds: credentials.NewStaticV4(
			envOrDefault("S3_ACCESS_KEY", defaultAccessKey),
			envOrDefault("S3_SECRET_KEY", defaultSecretKey),
			"",
		),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("create S3 client: %v", err)
	}
	return client
}

func testVersioning(t *testing.T, client *minio.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	bucket := makeBucket(t, ctx, client, "versioning", false)
	if err := client.EnableVersioning(ctx, bucket); err != nil {
		t.Fatalf("enable versioning: %v", err)
	}

	v1 := putText(t, ctx, client, bucket, "document.txt", "version one")
	v2 := putText(t, ctx, client, bucket, "document.txt", "version two")
	if v1.VersionID == "" || v2.VersionID == "" || v1.VersionID == v2.VersionID {
		t.Fatalf("expected two distinct version IDs, got %q and %q", v1.VersionID, v2.VersionID)
	}

	assertObjectBody(t, ctx, client, bucket, "document.txt", v1.VersionID, "version one")

	versionCount := 0
	for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
		if object.Err != nil {
			t.Fatalf("list object versions: %v", object.Err)
		}
		if object.Key == "document.txt" && !object.IsDeleteMarker {
			versionCount++
		}
	}
	if versionCount < 2 {
		t.Fatalf("expected at least two object versions, got %d", versionCount)
	}

	if err := client.RemoveObject(ctx, bucket, "document.txt", minio.RemoveObjectOptions{}); err != nil {
		t.Fatalf("create delete marker: %v", err)
	}
	assertObjectUnavailable(t, ctx, client, bucket, "document.txt")
	assertObjectBody(t, ctx, client, bucket, "document.txt", v1.VersionID, "version one")
}

func testMultipart(t *testing.T, client *minio.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	bucket := makeBucket(t, ctx, client, "multipart", false)
	core := minio.Core{Client: client}
	uploadID, err := core.NewMultipartUpload(ctx, bucket, "large.bin", minio.PutObjectOptions{})
	if err != nil {
		t.Fatalf("start multipart upload: %v", err)
	}

	partsData := [][]byte{
		bytes.Repeat([]byte("a"), 5*1024*1024),
		bytes.Repeat([]byte("b"), 5*1024*1024),
		bytes.Repeat([]byte("c"), 1024*1024),
	}
	completed := make([]minio.CompletePart, 0, len(partsData))
	for index, data := range partsData {
		partNumber := index + 1
		part, err := core.PutObjectPart(ctx, bucket, "large.bin", uploadID, partNumber, bytes.NewReader(data), int64(len(data)), minio.PutObjectPartOptions{})
		if err != nil {
			t.Fatalf("upload part %d: %v", partNumber, err)
		}
		completed = append(completed, minio.CompletePart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	if _, err := core.CompleteMultipartUpload(ctx, bucket, "large.bin", uploadID, completed, minio.PutObjectOptions{}); err != nil {
		t.Fatalf("complete multipart upload: %v", err)
	}

	want := bytes.Join(partsData, nil)
	got := readObject(t, ctx, client, bucket, "large.bin", "")
	if digest(got) != digest(want) {
		t.Fatalf("multipart content digest mismatch: got %s, want %s", digest(got), digest(want))
	}
}

func testPresignedGet(t *testing.T, client *minio.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	bucket := makeBucket(t, ctx, client, "presigned", false)
	putText(t, ctx, client, bucket, "report.txt", "presigned content")

	requestURL, err := client.PresignedGetObject(ctx, bucket, "report.txt", time.Minute, nil)
	if err != nil {
		t.Fatalf("create presigned URL: %v", err)
	}
	response, err := http.Get(requestURL.String())
	if err != nil {
		t.Fatalf("GET presigned URL: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read presigned response: %v", err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "presigned content" {
		t.Fatalf("unexpected presigned response: status=%d body=%q", response.StatusCode, string(body))
	}
}

func testBucketPolicy(t *testing.T, client *minio.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	bucket := makeBucket(t, ctx, client, "policy", false)
	object := "public/demo.txt"
	putText(t, ctx, client, bucket, object, "public content")
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/public/*"]}]}`, bucket)
	if err := client.SetBucketPolicy(ctx, bucket, policy); err != nil {
		t.Fatalf("set bucket policy: %v", err)
	}

	objectURL := fmt.Sprintf("http://%s/%s/%s", envOrDefault("S3_ENDPOINT", defaultEndpoint), bucket, object)
	assertAnonymousStatus(t, objectURL, http.StatusOK, "public content")

	if err := client.SetBucketPolicy(ctx, bucket, ""); err != nil {
		t.Fatalf("remove bucket policy: %v", err)
	}
	assertAnonymousStatus(t, objectURL, http.StatusForbidden, "")
}

func testLifecycle(t *testing.T, client *minio.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	bucket := makeBucket(t, ctx, client, "lifecycle", false)
	configuration := lifecycle.NewConfiguration()
	configuration.Rules = []lifecycle.Rule{{
		ID:     "expire-temp-files",
		Status: "Enabled",
		Prefix: "temp/",
		Expiration: lifecycle.Expiration{
			Days: lifecycle.ExpirationDays(30),
		},
	}}
	if err := client.SetBucketLifecycle(ctx, bucket, configuration); err != nil {
		t.Fatalf("set lifecycle configuration: %v", err)
	}

	got, err := client.GetBucketLifecycle(ctx, bucket)
	if err != nil {
		t.Fatalf("get lifecycle configuration: %v", err)
	}
	if len(got.Rules) != 1 || got.Rules[0].ID != "expire-temp-files" || got.Rules[0].Status != "Enabled" || got.Rules[0].Prefix != "temp/" || got.Rules[0].Expiration.Days != lifecycle.ExpirationDays(30) {
		t.Fatalf("unexpected lifecycle configuration: %+v", got.Rules)
	}
}

func testObjectLock(t *testing.T, client *minio.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	bucket := makeBucket(t, ctx, client, "object-lock", true)
	compliance := putText(t, ctx, client, bucket, "compliance.txt", "immutable")
	mode := minio.Compliance
	retainUntil := time.Now().UTC().Add(5 * time.Minute)
	if err := client.PutObjectRetention(ctx, bucket, "compliance.txt", minio.PutObjectRetentionOptions{
		Mode:            &mode,
		RetainUntilDate: &retainUntil,
		VersionID:       compliance.VersionID,
	}); err != nil {
		t.Fatalf("set compliance retention: %v", err)
	}
	assertDeleteDenied(t, client.RemoveObject(ctx, bucket, "compliance.txt", minio.RemoveObjectOptions{VersionID: compliance.VersionID}))
	assertDeleteDenied(t, client.RemoveObject(ctx, bucket, "compliance.txt", minio.RemoveObjectOptions{VersionID: compliance.VersionID, GovernanceBypass: true}))

	held := putText(t, ctx, client, bucket, "legal-hold.txt", "held")
	holdOn := minio.LegalHoldEnabled
	if err := client.PutObjectLegalHold(ctx, bucket, "legal-hold.txt", minio.PutObjectLegalHoldOptions{VersionID: held.VersionID, Status: &holdOn}); err != nil {
		t.Fatalf("enable legal hold: %v", err)
	}
	assertDeleteDenied(t, client.RemoveObject(ctx, bucket, "legal-hold.txt", minio.RemoveObjectOptions{VersionID: held.VersionID}))

	holdOff := minio.LegalHoldDisabled
	if err := client.PutObjectLegalHold(ctx, bucket, "legal-hold.txt", minio.PutObjectLegalHoldOptions{VersionID: held.VersionID, Status: &holdOff}); err != nil {
		t.Fatalf("disable legal hold: %v", err)
	}
	if err := client.RemoveObject(ctx, bucket, "legal-hold.txt", minio.RemoveObjectOptions{VersionID: held.VersionID}); err != nil {
		t.Fatalf("delete object after disabling legal hold: %v", err)
	}
}

func makeBucket(t *testing.T, ctx context.Context, client *minio.Client, purpose string, objectLocking bool) string {
	t.Helper()
	bucket := fmt.Sprintf("kareg-%s-%d", purpose, time.Now().UnixNano())
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{ObjectLocking: objectLocking}); err != nil {
		t.Fatalf("create bucket %s: %v", bucket, err)
	}
	return bucket
}

func putText(t *testing.T, ctx context.Context, client *minio.Client, bucket, object, value string) minio.UploadInfo {
	t.Helper()
	result, err := client.PutObject(ctx, bucket, object, strings.NewReader(value), int64(len(value)), minio.PutObjectOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("put object %s/%s: %v", bucket, object, err)
	}
	return result
}

func assertObjectBody(t *testing.T, ctx context.Context, client *minio.Client, bucket, object, versionID, want string) {
	t.Helper()
	if got := string(readObject(t, ctx, client, bucket, object, versionID)); got != want {
		t.Fatalf("unexpected body for %s/%s: got %q, want %q", bucket, object, got, want)
	}
}

func readObject(t *testing.T, ctx context.Context, client *minio.Client, bucket, object, versionID string) []byte {
	t.Helper()
	reader, err := client.GetObject(ctx, bucket, object, minio.GetObjectOptions{VersionID: versionID})
	if err != nil {
		t.Fatalf("get object %s/%s: %v", bucket, object, err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read object %s/%s: %v", bucket, object, err)
	}
	return body
}

func assertObjectUnavailable(t *testing.T, ctx context.Context, client *minio.Client, bucket, object string) {
	t.Helper()
	reader, err := client.GetObject(ctx, bucket, object, minio.GetObjectOptions{})
	if err == nil {
		defer reader.Close()
		_, err = io.ReadAll(reader)
	}
	if err == nil {
		t.Fatalf("expected %s/%s to be unavailable", bucket, object)
	}
}

func assertAnonymousStatus(t *testing.T, objectURL string, wantStatus int, wantBody string) {
	t.Helper()
	response, err := http.Get(objectURL)
	if err != nil {
		t.Fatalf("anonymous GET %s: %v", objectURL, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read anonymous response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("anonymous GET status: got %d, want %d; body=%q", response.StatusCode, wantStatus, string(body))
	}
	if wantBody != "" && string(body) != wantBody {
		t.Fatalf("anonymous GET body: got %q, want %q", string(body), wantBody)
	}
}

func assertDeleteDenied(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected protected object deletion to be denied")
	}
	response := minio.ToErrorResponse(err)
	if response.Code != "AccessDenied" && response.Code != "InvalidRequest" {
		t.Fatalf("unexpected protected deletion error: code=%q error=%v", response.Code, err)
	}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
