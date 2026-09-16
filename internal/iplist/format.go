package iplist

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	Magic      = "IPBLV001"
	Version    = uint32(1)
	HeaderSize = uint32(64)
)

// Metadata is the validated v1 header information.
type Metadata struct {
	Version      uint32 `json:"version"`
	HeaderLength uint32 `json:"header_length"`
	Count        uint64 `json:"count"`
	BuiltAtUnix  uint64 `json:"built_at_unix"`
	BuiltAt      string `json:"built_at"`
	DataSHA256   string `json:"data_sha256"`
	FileSize     int64  `json:"file_size"`
	BinarySHA256 string `json:"binary_sha256"`
}

// Manifest records source provenance and normalization statistics.
type Manifest struct {
	Format          string `json:"format"`
	FormatVersion   uint32 `json:"format_version"`
	Source          string `json:"source"`
	SourceSHA256    string `json:"source_sha256"`
	SourceBytes     int64  `json:"source_bytes"`
	SourceLines     int    `json:"source_lines"`
	ValidLines      int    `json:"valid_lines"`
	UniqueIPs       int    `json:"unique_ips"`
	Duplicates      int    `json:"duplicates_removed"`
	SourceUpdatedAt string `json:"source_updated_at,omitempty"`
	BuiltAt         string `json:"built_at"`
	BuiltAtUnix     uint64 `json:"built_at_unix"`
	DataSHA256      string `json:"data_sha256"`
	BinarySHA256    string `json:"binary_sha256"`
	BinaryBytes     int64  `json:"binary_bytes"`
}

// WriteBinary creates a new v1 binary atomically and refuses to replace a file.
func WriteBinary(path string, ips []uint32, builtAt time.Time) (Metadata, error) {
	if len(ips) == 0 {
		return Metadata{}, fmt.Errorf("refusing to write an empty list")
	}
	for i := 1; i < len(ips); i++ {
		if ips[i] <= ips[i-1] {
			return Metadata{}, fmt.Errorf("addresses must be strictly increasing at index %d", i)
		}
	}
	if _, err := os.Stat(path); err == nil {
		return Metadata{}, fmt.Errorf("output already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return Metadata{}, fmt.Errorf("inspect output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Metadata{}, fmt.Errorf("create output directory: %w", err)
	}

	dataHash := sha256.New()
	var word [4]byte
	for _, ip := range ips {
		binary.LittleEndian.PutUint32(word[:], ip)
		_, _ = dataHash.Write(word[:])
	}
	digest := dataHash.Sum(nil)

	tmp, err := os.CreateTemp(filepath.Dir(path), ".ipbl-*.tmp")
	if err != nil {
		return Metadata{}, fmt.Errorf("create temporary binary: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return Metadata{}, fmt.Errorf("set binary permissions: %w", err)
	}

	header := make([]byte, HeaderSize)
	copy(header[0:8], Magic)
	binary.LittleEndian.PutUint32(header[8:12], Version)
	binary.LittleEndian.PutUint32(header[12:16], HeaderSize)
	binary.LittleEndian.PutUint64(header[16:24], uint64(len(ips)))
	binary.LittleEndian.PutUint64(header[24:32], uint64(builtAt.Unix()))
	copy(header[32:64], digest)

	w := bufio.NewWriterSize(tmp, 64*1024)
	if _, err := w.Write(header); err != nil {
		return Metadata{}, fmt.Errorf("write header: %w", err)
	}
	for _, ip := range ips {
		binary.LittleEndian.PutUint32(word[:], ip)
		if _, err := w.Write(word[:]); err != nil {
			return Metadata{}, fmt.Errorf("write address data: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		return Metadata{}, fmt.Errorf("flush binary: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return Metadata{}, fmt.Errorf("sync binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Metadata{}, fmt.Errorf("close binary: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return Metadata{}, fmt.Errorf("publish binary: %w", err)
	}
	ok = true
	return VerifyFile(path, uint64(len(ips)))
}

// VerifyFile validates every header field, exact size, digest and ordering.
func VerifyFile(path string, maxIPs uint64) (Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("open binary: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Metadata{}, fmt.Errorf("stat binary: %w", err)
	}
	fileHash := sha256.New()
	r := io.TeeReader(f, fileHash)
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return Metadata{}, fmt.Errorf("read header: %w", err)
	}
	if string(header[0:8]) != Magic {
		return Metadata{}, fmt.Errorf("invalid magic")
	}
	version := binary.LittleEndian.Uint32(header[8:12])
	if version != Version {
		return Metadata{}, fmt.Errorf("unsupported format version %d", version)
	}
	headerLength := binary.LittleEndian.Uint32(header[12:16])
	if headerLength != HeaderSize {
		return Metadata{}, fmt.Errorf("invalid header length %d", headerLength)
	}
	count := binary.LittleEndian.Uint64(header[16:24])
	if count == 0 {
		return Metadata{}, fmt.Errorf("empty list is not valid")
	}
	if maxIPs == 0 || count > maxIPs {
		return Metadata{}, fmt.Errorf("IP count %d exceeds limit %d", count, maxIPs)
	}
	if count > uint64((1<<63-1-int64(HeaderSize))/4) {
		return Metadata{}, fmt.Errorf("IP count overflows file length")
	}
	expectedSize := int64(HeaderSize) + int64(count)*4
	if st.Size() != expectedSize {
		return Metadata{}, fmt.Errorf("file length %d, expected %d", st.Size(), expectedSize)
	}

	dataHash := sha256.New()
	dataReader := io.TeeReader(r, dataHash)
	var word [4]byte
	var previous uint32
	for i := uint64(0); i < count; i++ {
		if _, err := io.ReadFull(dataReader, word[:]); err != nil {
			return Metadata{}, fmt.Errorf("read address %d: %w", i, err)
		}
		current := binary.LittleEndian.Uint32(word[:])
		if i > 0 && current <= previous {
			return Metadata{}, fmt.Errorf("addresses are not strictly increasing at index %d", i)
		}
		previous = current
	}
	if !bytes.Equal(dataHash.Sum(nil), header[32:64]) {
		return Metadata{}, fmt.Errorf("address data SHA-256 mismatch")
	}
	builtUnix := binary.LittleEndian.Uint64(header[24:32])
	builtAt := time.Unix(int64(builtUnix), 0).UTC()
	return Metadata{
		Version: Version, HeaderLength: HeaderSize, Count: count,
		BuiltAtUnix: builtUnix, BuiltAt: builtAt.Format(time.RFC3339),
		DataSHA256: hex.EncodeToString(header[32:64]), FileSize: st.Size(),
		BinarySHA256: hex.EncodeToString(fileHash.Sum(nil)),
	}, nil
}

// ReadAll validates and then reads all addresses for the offline lookup command.
func ReadAll(path string, maxIPs uint64) ([]uint32, Metadata, error) {
	meta, err := VerifyFile(path, maxIPs)
	if err != nil {
		return nil, Metadata{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer f.Close()
	if _, err := f.Seek(int64(HeaderSize), io.SeekStart); err != nil {
		return nil, Metadata{}, err
	}
	ips := make([]uint32, int(meta.Count))
	if err := binary.Read(f, binary.LittleEndian, ips); err != nil {
		return nil, Metadata{}, fmt.Errorf("read address array: %w", err)
	}
	return ips, meta, nil
}

// WriteManifest atomically creates a JSON build manifest.
func WriteManifest(path string, manifest Manifest) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("manifest already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
