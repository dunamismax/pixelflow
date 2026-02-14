package domain

import "time"

type UsageLog struct {
	UserID          string
	JobID           string
	PixelsProcessed int64
	BytesSaved      int64
	ComputeTimeMS   int64
	CreatedAt       time.Time
}
