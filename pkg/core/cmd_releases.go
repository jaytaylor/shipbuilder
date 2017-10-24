package core

import (
	"fmt"
	"net"

	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/s3"
)

var awsAuth aws.Auth = aws.Auth{
	AccessKey: DefaultAWSKey,
	SecretKey: DefaultAWSSecret,
}

func getS3Bucket() *s3.Bucket {
	return s3.New(awsAuth, aws.Regions[DefaultAWSRegion]).Bucket(DefaultS3BucketName)
}

func (server *Server) Releases_List(conn net.Conn, applicationName string) error {
	releases, err := server.ReleasesProvider.List(applicationName)
	if err != nil {
		Logf(conn, "%v", err)
		return err
	}
	for _, r := range releases {
		Logf(conn, "%v %v %v\n", r.Version, r.Revision, r.Date)
	}
	return nil
}
func (*Server) Releases_Info(conn net.Conn, applicationName, version string) error {
	return fmt.Errorf("not implemented")
}
