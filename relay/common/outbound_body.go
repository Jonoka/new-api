package common

import (
	"io"

	"github.com/QuantumNous/new-api/common"
)

// NewOutboundJSONBody wraps the already-marshaled upstream request body into a
// BodyStorage. When disk cache is enabled and the payload exceeds the configured
// threshold, the data is written to a temp file and the original []byte can be
// GC'd, significantly reducing the heap residency while waiting for the
// upstream provider to respond (the dominant cost for large base64 payloads).
//
// In memory mode the storage owns one immutable snapshot shared by all replay
// readers. Each reader has an independent cursor.
//
// The caller MUST invoke closer.Close() once the upstream call has finished
// (typically via defer) to release the disk file / memory accounting.
//
// The returned reader is wrapped with common.ReaderOnly to prevent the HTTP
// transport from prematurely closing the underlying BodyStorage. The returned
// size is meant to be propagated to http.Request.ContentLength because the
// type-erased io.Reader prevents net/http from auto-detecting it.
func NewOutboundJSONBody(data []byte) (body io.Reader, size int64, factory func() (io.ReadCloser, error), closer io.Closer, err error) {
	storage, err := common.CreateBodyStorage(data)
	if err != nil {
		return nil, 0, nil, nil, err
	}
	return common.ReaderOnly(storage), storage.Size(), storage.NewReader, storage, nil
}
