package gateways

import "errors"

type channelCreationFailure struct {
	err      error
	rejected bool
}

func (e *channelCreationFailure) Error() string { return e.err.Error() }
func (e *channelCreationFailure) Unwrap() error { return e.err }

func rejectedChannelCreation(err error) error {
	return &channelCreationFailure{err: err, rejected: true}
}

// ChannelCreationRejected distinguishes a proven rejection (or a failure
// before submitting the create request) from an unknown provider outcome.
func ChannelCreationRejected(err error) bool {
	var failure *channelCreationFailure
	return errors.As(err, &failure) && failure.rejected
}
