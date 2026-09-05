package channel

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	basecommon "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func TestApplyUpstreamBodyMetadataPreservesNativeFactory(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("native"))
	require.NoError(t, err)
	require.NotNil(t, req.GetBody)

	ApplyUpstreamBodyMetadata(req, &relaycommon.RelayInfo{
		UpstreamRequestBodySize: 99,
		UpstreamRequestBodyFactory: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("replacement")), nil
		},
	})

	require.EqualValues(t, len("native"), req.ContentLength)
	replay, err := req.GetBody()
	require.NoError(t, err)
	defer replay.Close()
	got, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.Equal(t, "native", string(got))
}

func TestApplyUpstreamBodyMetadataBindsStorageFactory(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"model":"mapped","input":"final"}`)
	body, size, factory, owner, err := relaycommon.NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer owner.Close()

	req, err := http.NewRequest(http.MethodPost, "https://example.com", body)
	require.NoError(t, err)
	require.Nil(t, req.GetBody)
	ApplyUpstreamBodyMetadata(req, &relaycommon.RelayInfo{
		UpstreamRequestBodySize:    size,
		UpstreamRequestBodyFactory: factory,
	})

	require.EqualValues(t, len(payload), req.ContentLength)
	require.NotNil(t, req.GetBody)
	first, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, payload, first)
	replay, err := req.GetBody()
	require.NoError(t, err)
	got, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.NoError(t, replay.Close())
	require.Equal(t, payload, got)
}

func TestInitChannelMetaResetsClosedAttemptBody(t *testing.T) {
	t.Parallel()

	_, size, factory, owner, err := relaycommon.NewOutboundJSONBody([]byte("first-attempt"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		UpstreamRequestBodySize:    size,
		UpstreamRequestBodyFactory: factory,
	}
	require.NoError(t, owner.Close())
	_, err = factory()
	require.ErrorIs(t, err, basecommon.ErrStorageClosed)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info.InitChannelMeta(c)
	require.Zero(t, info.UpstreamRequestBodySize)
	require.Nil(t, info.UpstreamRequestBodyFactory)
}

type refusedStreamServerResult struct {
	err    error
	bodies [][]byte
}

func acceptHTTP2Connection(listener net.Listener) (net.Conn, *http2.Framer, error) {
	conn, err := listener.Accept()
	if err != nil {
		return nil, nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(conn, preface); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("read HTTP/2 preface: %w", err)
	}
	if !bytes.Equal(preface, []byte(http2.ClientPreface)) {
		conn.Close()
		return nil, nil, fmt.Errorf("unexpected HTTP/2 preface")
	}
	framer := http2.NewFramer(conn, conn)
	framer.ReadMetaHeaders = hpack.NewDecoder(4096, nil)
	if err := framer.WriteSettings(); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, framer, nil
}

func readHTTP2RequestBody(framer *http2.Framer) (uint32, []byte, error) {
	var streamID uint32
	var body []byte
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			return 0, nil, err
		}
		switch frame := frame.(type) {
		case *http2.SettingsFrame:
			if !frame.IsAck() {
				if err := framer.WriteSettingsAck(); err != nil {
					return 0, nil, err
				}
			}
		case *http2.MetaHeadersFrame:
			streamID = frame.Header().StreamID
			if frame.StreamEnded() {
				return streamID, body, nil
			}
		case *http2.DataFrame:
			if streamID == 0 {
				streamID = frame.Header().StreamID
			}
			if frame.Header().StreamID != streamID {
				continue
			}
			body = append(body, frame.Data()...)
			if frame.StreamEnded() {
				return streamID, body, nil
			}
		}
	}
}

func writeHTTP2Success(framer *http2.Framer, streamID uint32) error {
	var header bytes.Buffer
	encoder := hpack.NewEncoder(&header)
	if err := encoder.WriteField(hpack.HeaderField{Name: ":status", Value: "200"}); err != nil {
		return err
	}
	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: header.Bytes(),
		EndHeaders:    true,
	}); err != nil {
		return err
	}
	return framer.WriteData(streamID, true, []byte(`{}`))
}

func serveRefusedStreamAfterFullUpload(listener net.Listener) <-chan refusedStreamServerResult {
	result := make(chan refusedStreamServerResult, 1)
	go func() {
		out := refusedStreamServerResult{}
		defer func() { result <- out }()
		conn, framer, err := acceptHTTP2Connection(listener)
		if err != nil {
			out.err = err
			return
		}
		defer conn.Close()

		for attempt := 0; attempt < 2; attempt++ {
			streamID, body, err := readHTTP2RequestBody(framer)
			if err != nil {
				out.err = err
				return
			}
			out.bodies = append(out.bodies, body)
			if attempt == 0 {
				if err := framer.WriteRSTStream(streamID, http2.ErrCodeRefusedStream); err != nil {
					out.err = err
					return
				}
				continue
			}
			out.err = writeHTTP2Success(framer, streamID)
			return
		}
	}()
	return result
}

func TestHTTP2RefusedStreamAfterFullUploadReplaysExactBody(t *testing.T) {
	payload := bytes.Repeat([]byte(`{"input":"complete-upload-before-reset"}`), 32)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	serverResult := serveRefusedStreamAfterFullUpload(listener)

	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, _ string, _ *tls.Config) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, listener.Addr().String())
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}

	body, size, factory, owner, err := relaycommon.NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer owner.Close()
	req, err := http.NewRequest(http.MethodPost, "http://upstream.test/v1/responses", body)
	require.NoError(t, err)
	ApplyUpstreamBodyMetadata(req, &relaycommon.RelayInfo{
		UpstreamRequestBodySize:    size,
		UpstreamRequestBodyFactory: factory,
	})

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case result := <-serverResult:
		require.NoError(t, result.err)
		require.Len(t, result.bodies, 2)
		require.Equal(t, payload, result.bodies[0])
		require.Equal(t, payload, result.bodies[1])
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for HTTP/2 replay fixture")
	}
}
