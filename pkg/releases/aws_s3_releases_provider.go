package releases

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/jaytaylor/shipbuilder/pkg/domain"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/sirupsen/logrus"
)

// AWSS3ReleasesProvider is an Amazon AWS S3 based releases provider.
type AWSS3ReleasesProvider struct {
	baseReleasesProvider

	svc    *s3.S3
	bucket string
	region string
}

// NewAWSS3ReleasesProvider creates a new instance of *AWSS3ReleasesProvider.
func NewAWSS3ReleasesProvider(accessKey string, secretKey string, bucket string, region string) (*AWSS3ReleasesProvider, error) {
	creds := credentials.NewStaticCredentials(accessKey, secretKey, "")
	if _, err := creds.Get(); err != nil {
		return nil, err
	}
	cfg := aws.NewConfig().WithRegion(region).WithCredentials(creds)
	provider := &AWSS3ReleasesProvider{
		svc:    s3.New(session.New(), cfg),
		bucket: bucket,
		region: region,
	}
	return provider, nil
}

// List returns the list of releases for an application.
func (provider *AWSS3ReleasesProvider) List(applicationName string) ([]domain.Release, error) {
	var (
		key      = fmt.Sprintf("/releases/%v/manifest.json", applicationName)
		releases []domain.Release
	)

	input := &s3.GetObjectInput{
		Bucket: aws.String(provider.bucket),
		Key:    aws.String(key),
	}

	resp, err := provider.svc.GetObject(input)
	if err != nil {
		return releases, fmt.Errorf("retrieving key=%q: %s", key, err)
	}

	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return releases, fmt.Errorf("reading response body for key=%q: %s", key, err)
	}
	resp.Body.Close()

	if err := json.Unmarshal(bs, &releases); err != nil {
		return nil, fmt.Errorf("unmarshalling key=%q: %s", key, err)
	}
	return releases, nil
}

// Set sets the list of releases for an application.
func (provider *AWSS3ReleasesProvider) Set(applicationName string, releases []domain.Release) error {
	bs, err := json.Marshal(releases)
	if err != nil {
		return fmt.Errorf("marshalling releases: %s", err)
	}

	key := fmt.Sprintf("/releases/%v/manifest.json", applicationName)

	putInput := &s3.PutObjectInput{
		Bucket:        aws.String(provider.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(bs),
		ContentLength: aws.Int64(int64(len(bs))),
		ContentType:   aws.String("application/json"),
	}

	resp, err := provider.svc.PutObject(putInput)
	if err != nil {
		return fmt.Errorf("uploading manifest.json: %s", err)
	}
	log.WithField("app", applicationName).Debugf("Upload releases manifest response: %s", awsutil.StringValue(resp))
	return nil
}

// Delete removes all releases for an application.
func (provider *AWSS3ReleasesProvider) Delete(applicationName string, logger io.Writer) error {
	prefix := fmt.Sprintf("/releases/%v", applicationName)

	listInput := &s3.ListObjectsInput{
		Bucket: aws.String(provider.bucket),
		Prefix: aws.String(prefix),
	}
	resp, err := provider.svc.ListObjects(listInput)
	if err != nil {
		return fmt.Errorf("listing contents of prefix=%q: %s", prefix, err)
	}
	log.WithField("app", applicationName).WithField("prefix", prefix).Debugf("Delete list objects response: %s", awsutil.StringValue(resp))

	fmt.Fprintf(logger, "Purging application %q from S3..\n", applicationName)

	delIter := s3manager.NewDeleteListIterator(provider.svc, listInput)

	batcher := s3manager.NewBatchDelete(session.New(&provider.svc.Config))

	if err := batcher.Delete(aws.BackgroundContext(), delIter); err != nil {
		return fmt.Errorf("batch deleting prefix %q: %s", prefix, err)
	}
	return nil
}

// Store uploads release content.
func (provider *AWSS3ReleasesProvider) Store(applicationName string, version string, rs io.ReadSeeker, length int64) error {
	key := fmt.Sprintf("/releases/%v/%v.tar.gz", applicationName, version)

	putInput := &s3.PutObjectInput{
		Bucket:        aws.String(provider.bucket),
		Key:           aws.String(key),
		Body:          rs,
		ContentLength: aws.Int64(length),
		ContentType:   aws.String("application/x-tar-gz"),
	}

	resp, err := provider.svc.PutObject(putInput)
	if err != nil {
		return fmt.Errorf("uploading release content: %s", err)
	}
	log.WithField("app", applicationName).Debugf("Upload release content response: %s", awsutil.StringValue(resp))
	return nil
}

// Get retrieves a specific release.
func (provider *AWSS3ReleasesProvider) Get(applicationName string, version string) (*domain.Release, error) {
	releases, err := provider.List(applicationName)
	if err != nil {
		return nil, err
	}
	return provider.find(version, releases)
}
