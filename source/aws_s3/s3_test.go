package awss3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	st "github.com/golang-migrate/migrate/v4/source/testing"
	"github.com/stretchr/testify/assert"
)

func Test(t *testing.T) {
	s3Client := fakeS3{
		bucket: "some-bucket",
		objects: map[string]string{
			"staging/migrations/1_foobar.up.sql":          "1 up",
			"staging/migrations/1_foobar.down.sql":        "1 down",
			"prod/migrations/1_foobar.up.sql":             "1 up",
			"prod/migrations/1_foobar.down.sql":           "1 down",
			"prod/migrations/3_foobar.up.sql":             "3 up",
			"prod/migrations/4_foobar.up.sql":             "4 up",
			"prod/migrations/4_foobar.down.sql":           "4 down",
			"prod/migrations/5_foobar.down.sql":           "5 down",
			"prod/migrations/7_foobar.up.sql":             "7 up",
			"prod/migrations/7_foobar.down.sql":           "7 down",
			"prod/migrations/not-a-migration.txt":         "",
			"prod/migrations/0-random-stuff/whatever.txt": "",
		},
	}
	driver, err := WithInstance(&s3Client, &Config{
		Bucket: "some-bucket",
		Prefix: "prod/migrations/",
	})
	if err != nil {
		t.Fatal(err)
	}
	st.Test(t, driver)
}

func TestLoadMigrationsPaginates(t *testing.T) {
	// A single ListObjects response is capped at 1000 keys by S3. Spread the
	// migrations across several pages (via pageSize) to ensure loadMigrations
	// walks every page instead of silently stopping after the first one.
	const migrationCount = 300
	objects := make(map[string]string, migrationCount*2)
	for i := 1; i <= migrationCount; i++ {
		objects[fmt.Sprintf("prod/migrations/%d_foobar.up.sql", i)] = fmt.Sprintf("%d up", i)
		objects[fmt.Sprintf("prod/migrations/%d_foobar.down.sql", i)] = fmt.Sprintf("%d down", i)
	}
	s3Client := fakeS3{
		bucket:   "some-bucket",
		pageSize: 50,
		objects:  objects,
	}
	driver, err := WithInstance(&s3Client, &Config{
		Bucket: "some-bucket",
		Prefix: "prod/migrations/",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := driver.First()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, uint(1), first)

	version := first
	count := 1
	for {
		next, err := driver.Next(version)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		version = next
		count++
	}
	assert.Equal(t, migrationCount, count, "every migration across all pages should be loaded")
	assert.Equal(t, uint(migrationCount), version, "the highest-numbered migration should be loaded")

	r, identifier, err := driver.ReadUp(uint(migrationCount))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	assert.Equal(t, "foobar", identifier)
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, fmt.Sprintf("%d up", migrationCount), string(body))
}

func TestParseURI(t *testing.T) {
	tests := []struct {
		name   string
		uri    string
		config *Config
	}{
		{
			"with prefix, no trailing slash",
			"s3://migration-bucket/production",
			&Config{
				Bucket: "migration-bucket",
				Prefix: "production/",
			},
		},
		{
			"without prefix, no trailing slash",
			"s3://migration-bucket",
			&Config{
				Bucket: "migration-bucket",
			},
		},
		{
			"with prefix, trailing slash",
			"s3://migration-bucket/production/",
			&Config{
				Bucket: "migration-bucket",
				Prefix: "production/",
			},
		},
		{
			"without prefix, trailing slash",
			"s3://migration-bucket/",
			&Config{
				Bucket: "migration-bucket",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := parseURI(test.uri)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.config, actual)
		})
	}
}

type fakeS3 struct {
	bucket string
	// pageSize caps how many objects each ListObjectsV2 page returns so tests
	// can exercise the multi-page path; 0 means a single page.
	pageSize int
	objects  map[string]string
}

func (s *fakeS3) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	bucket := aws.ToString(input.Bucket)
	if bucket != s.bucket {
		return nil, errors.New("bucket not found")
	}
	prefix := aws.ToString(input.Prefix)
	delimiter := aws.ToString(input.Delimiter)

	var keys []string
	for name := range s.objects {
		if strings.HasPrefix(name, prefix) {
			if delimiter == "" || !strings.Contains(strings.Replace(name, prefix, "", 1), delimiter) {
				keys = append(keys, name)
			}
		}
	}
	// The paginator issues one call per page, so the key order has to be stable
	// across calls; ranging over a map alone would shuffle the page boundaries
	// and drop or repeat keys between pages.
	sort.Strings(keys)

	start := 0
	if token := aws.ToString(input.ContinuationToken); token != "" {
		var err error
		start, err = strconv.Atoi(token)
		if err != nil {
			return nil, fmt.Errorf("invalid continuation token %q", token)
		}
	}
	if start > len(keys) {
		start = len(keys)
	}
	end := len(keys)
	if s.pageSize > 0 && start+s.pageSize < end {
		end = start + s.pageSize
	}

	var output s3.ListObjectsV2Output
	for _, name := range keys[start:end] {
		output.Contents = append(output.Contents, types.Object{
			Key: aws.String(name),
		})
	}
	if end < len(keys) {
		output.IsTruncated = aws.Bool(true)
		output.NextContinuationToken = aws.String(strconv.Itoa(end))
	}
	return &output, nil
}

func (s *fakeS3) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	bucket := aws.ToString(input.Bucket)
	if bucket != s.bucket {
		return nil, errors.New("bucket not found")
	}
	if data, ok := s.objects[aws.ToString(input.Key)]; ok {
		body := io.NopCloser(strings.NewReader(data))
		return &s3.GetObjectOutput{Body: body}, nil
	}
	return nil, errors.New("object not found")
}
