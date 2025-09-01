package events

import "context"

// Enqueue accepts a CloudEvent payload for processing.
func Enqueue(ctx context.Context, payload []byte) error {
	// TODO: implement event enqueue
	return nil
}
