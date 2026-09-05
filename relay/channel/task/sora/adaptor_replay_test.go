package sora

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type noMaterializeBodyStorage struct {
	payload     []byte
	reader      *bytes.Reader
	bytesCalled bool
}

func newNoMaterializeBodyStorage(payload []byte) *noMaterializeBodyStorage {
	return &noMaterializeBodyStorage{payload: payload, reader: bytes.NewReader(payload)}
}

func (s *noMaterializeBodyStorage) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *noMaterializeBodyStorage) Seek(offset int64, whence int) (int64, error) {
	return s.reader.Seek(offset, whence)
}

func (s *noMaterializeBodyStorage) Close() error { return nil }

func (s *noMaterializeBodyStorage) Bytes() ([]byte, error) {
	s.bytesCalled = true
	return bytes.Clone(s.payload), nil
}

func (s *noMaterializeBodyStorage) Size() int64 { return int64(len(s.payload)) }

func (s *noMaterializeBodyStorage) IsDisk() bool { return true }

func (s *noMaterializeBodyStorage) NewReader() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.payload)), nil
}

func TestBuildRequestBodyBindsReplayForOpaquePassThrough(t *testing.T) {
	payload := []byte("opaque-sora-upload")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/octet-stream")
	defer common.CleanupBodyStorage(c)

	info := &relaycommon.RelayInfo{}
	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, payload, got)
	require.EqualValues(t, len(payload), info.UpstreamRequestBodySize)
	require.NotNil(t, info.UpstreamRequestBodyFactory)

	replay, err := info.UpstreamRequestBodyFactory()
	require.NoError(t, err)
	defer replay.Close()
	replayed, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, payload, replayed)
}

func TestBuildRequestBodyDoesNotMaterializeOpaqueDiskStorage(t *testing.T) {
	payload := []byte("disk-backed-opaque-sora-upload")
	storage := newNoMaterializeBodyStorage(payload)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	c.Request.Header.Set("Content-Type", "application/octet-stream")
	c.Set(common.KeyBodyStorage, storage)

	info := &relaycommon.RelayInfo{}
	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	require.False(t, storage.bytesCalled)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}
