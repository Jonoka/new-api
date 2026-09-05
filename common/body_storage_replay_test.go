package common

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func assertIndependentStorageReaders(t *testing.T, storage BodyStorage, payload []byte) {
	t.Helper()

	first, err := storage.NewReader()
	require.NoError(t, err)
	second, err := storage.NewReader()
	require.NoError(t, err)

	head := make([]byte, len(payload)/2)
	_, err = io.ReadFull(first, head)
	require.NoError(t, err)
	require.Equal(t, payload[:len(head)], head)

	all, err := io.ReadAll(second)
	require.NoError(t, err)
	require.Equal(t, payload, all)
	require.NoError(t, second.Close())

	tail, err := io.ReadAll(first)
	require.NoError(t, err)
	require.Equal(t, payload[len(head):], tail)
	require.NoError(t, first.Close())
}

func TestMemoryStorageNewReaderIndependentAndOwnerLifetime(t *testing.T) {
	t.Parallel()

	payload := []byte("memory-replay-payload")
	storage := newMemoryStorage(payload)
	t.Cleanup(func() { _ = storage.Close() })
	assertIndependentStorageReaders(t, storage, payload)

	reader, err := storage.NewReader()
	require.NoError(t, err)
	require.NoError(t, storage.Close())
	_, err = storage.NewReader()
	require.ErrorIs(t, err, ErrStorageClosed)

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, payload, got)
	require.NoError(t, reader.Close())
}

func TestMemoryStorageOwnsImmutableSnapshot(t *testing.T) {
	t.Parallel()

	payload := []byte("immutable-memory-body")
	want := bytes.Clone(payload)
	storage := newMemoryStorage(payload)
	defer storage.Close()
	payload[0] = 'X'

	exposed, err := storage.Bytes()
	require.NoError(t, err)
	exposed[1] = 'Y'
	reader, err := storage.NewReader()
	require.NoError(t, err)
	defer reader.Close()
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestMemoryStorageNewReaderConcurrent(t *testing.T) {
	t.Parallel()

	payload := bytes.Repeat([]byte("concurrent-memory-body"), 128)
	storage := newMemoryStorage(payload)
	defer storage.Close()

	const readers = 16
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader, err := storage.NewReader()
			if err != nil {
				errs <- err
				return
			}
			defer reader.Close()
			got, err := io.ReadAll(reader)
			if err == nil && !bytes.Equal(got, payload) {
				err = io.ErrUnexpectedEOF
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestDiskStorageNewReaderIndependentAndOwnerLifetime(t *testing.T) {
	payload := []byte("disk-replay-payload")
	file, err := os.CreateTemp(t.TempDir(), "body-*.tmp")
	require.NoError(t, err)
	_, err = file.Write(payload)
	require.NoError(t, err)
	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)

	storage := &diskStorage{file: file, filePath: file.Name(), size: int64(len(payload))}
	IncrementDiskFiles(storage.size)
	t.Cleanup(func() { _ = storage.Close() })
	assertIndependentStorageReaders(t, storage, payload)

	reader, err := storage.NewReader()
	require.NoError(t, err)
	diskReader, isDiskReader := reader.(*diskStorageReader)
	require.True(t, isDiskReader, "disk replay must use an independent file descriptor")
	require.NotSame(t, storage.file, diskReader.file)
	require.NoError(t, storage.Close())
	_, err = storage.NewReader()
	require.ErrorIs(t, err, ErrStorageClosed)
	_, err = os.Stat(file.Name())
	require.NoError(t, err, "cache file remains owned while a child reader is open")

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, payload, got)
	require.NoError(t, reader.Close())
	_, err = os.Stat(file.Name())
	require.True(t, os.IsNotExist(err), "owner close must remove the cache file")
}

func TestDiskStorageNewReaderConcurrent(t *testing.T) {
	payload := bytes.Repeat([]byte("concurrent-disk-body"), 128)
	file, err := os.CreateTemp(t.TempDir(), "body-*.tmp")
	require.NoError(t, err)
	_, err = file.Write(payload)
	require.NoError(t, err)
	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)

	storage := &diskStorage{file: file, filePath: file.Name(), size: int64(len(payload))}
	IncrementDiskFiles(storage.size)
	defer storage.Close()

	const readers = 16
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader, err := storage.NewReader()
			if err != nil {
				errs <- err
				return
			}
			defer reader.Close()
			got, err := io.ReadAll(reader)
			if err == nil && !bytes.Equal(got, payload) {
				err = io.ErrUnexpectedEOF
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
