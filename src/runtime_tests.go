package main

import (
	"fmt"
	"net"
	"strings"
)

func (server *Server) LocalRuntimeTests(conn net.Conn) error {
	tests := map[string]func() error{
		"S3 write/read test": TestS3Config,
	}

	failedTests := []string{}

	logger := server.getLogger(conn)

	fmt.Fprintf(logger, "== Runtime tests ==\n")
	for testName, testFn := range tests {
		err := testFn()
		if err != nil {
			fmt.Fprintf(logger, "    %v: failed, reason: %v\n", testName, err)
			failedTests = append(failedTests, testName)
		} else {
			fmt.Fprintf(logger, "    %v: succeeeded\n", testName)
		}
	}

	fmt.Fprintf(logger, "\n")

	if len(failedTests) > 0 {
		return fmt.Errorf("the following tests failed:\n    - %v", strings.Join(failedTests, "\n    - "))
	} else {
		fmt.Fprintf(logger, "All tests passed.\n")
		return nil
	}
}

/**
 * Verifies that the specified S3 bucket is able to be written and read from.
 */
func TestS3Config() error {
	s3TestFilePath := "/sbTest"
	data := []byte("data")
	bucket := getS3Bucket()

	err := bucket.Put(s3TestFilePath, data, "text", "private")
	if err != nil {
		return err
	}
	remoteContents, err := bucket.Get(s3TestFilePath)
	if err != nil {
		return err
	}
	// Verify that the contents match.
	if string(remoteContents) != string(data) {
		return fmt.Errorf("S3 integrity error: Downloaded file contents did not match, sent \"%v\" but got back \"%v\"", string(data), string(remoteContents))
	}
	return nil
}
