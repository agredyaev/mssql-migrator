package migrator

import (
	"context"
	"time"
)

const metadataWriteTimeout = 10 * time.Second

func metadataContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, metadataWriteTimeout)
}
