package domain

import (
	"time"
)

// Release encapsulates the notion of an App "release".
type Release struct {
	Version          string
	Revision         string
	ImageFingerprint string
	Date             time.Time
	Config           map[string]string
}
