package releases

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jaytaylor/shipbuilder/pkg/domain"

	log "github.com/sirupsen/logrus"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/s3"
)

// AWSS3ReleasesProvider is an Amazon AWS S3 based releases provider.
type AWSS3ReleasesProvider struct {
	baseReleasesProvider

	auth   aws.Auth
	bucket string
	region string
}

// NewAWSS3ReleasesProvider creates a new instance of *AWSS3ReleasesProvider.
func NewAWSS3ReleasesProvider(accessKey string, secretKey string, bucket string, region string) *AWSS3ReleasesProvider {
	provider := &AWSS3ReleasesProvider{
		auth: aws.Auth{
			AccessKey: accessKey,
			SecretKey: secretKey,
		},
		bucket: bucket,
		region: region,
	}
	return provider
}

// List returns the list of releases for an application.
func (provider *AWSS3ReleasesProvider) List(applicationName string) ([]domain.Release, error) {
	var releases []domain.Release
	bs, err := provider.s3Bucket().Get(fmt.Sprintf("/releases/%v/manifest.json", applicationName))
	if err != nil {
		if err.Error() == "The specified key does not exist." {
			// The manifest.json file for this app was missing, fill in an empty releases list and continue on our way.
			if err := provider.Set(applicationName, []domain.Release{}); err != nil {
				return nil, err
			}
			log.Warnf("Manifest.json S3 key was missing for application %q, so an empty releases list was set", applicationName)
			return []domain.Release{}, nil
		}
		return releases, err
	}
	if err := json.Unmarshal(bs, &releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// Set sets the list of releases for an application.
func (provider *AWSS3ReleasesProvider) Set(applicationName string, releases []domain.Release) error {
	bs, err := json.Marshal(releases)
	if err != nil {
		return err
	}
	if err := provider.s3Bucket().Put(fmt.Sprintf("/releases/%v/manifest.json", applicationName), bs, "application/json", "private"); err != nil {
		return err
	}
	return nil
}

// Delete removes all releases for an application.
func (provider *AWSS3ReleasesProvider) Delete(applicationName string, logger io.Writer) error {
	bucket := provider.s3Bucket()
	keys, err := bucket.List("releases/"+applicationName, "/releases/"+applicationName, "", 999999)
	if err != nil {
		return err
	}
	fmt.Fprint(logger, "Purging application from S3..\n")
	for _, key := range keys.Contents {
		fmt.Fprintf(logger, "    Deleting key %v\n", key.Key)
		bucket.Del(key.Key)
	}
	return nil
}

// Store adds a new release to the set of releases.
func (provider *AWSS3ReleasesProvider) Store(applicationName string, version string, r io.Reader, length int64) error {
	if err := provider.s3Bucket().PutReader(fmt.Sprintf("/releases/%v/%v.tar.gz", applicationName, version), r, length, "application/x-tar-gz", "private"); err != nil {
		return err
	}
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

func (provider *AWSS3ReleasesProvider) s3Bucket() *s3.Bucket {
	return s3.New(provider.auth, aws.Regions[provider.region]).Bucket(provider.bucket)
}
