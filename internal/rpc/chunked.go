package rpc

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
)

// ChunkSize is the maximum size of each chunk (14KB to leave room for headers).
const ChunkSize = 14 * 1024

// ChunkedPayload represents a large payload split into chunks.
type ChunkedPayload struct {
	TotalSize   uint32 // Total uncompressed size
	TotalChunks uint16 // Total number of chunks
	ChunkIndex  uint16 // Current chunk index (0-based)
	Compressed  bool   // Whether the data is gzip compressed
	Data        []byte // Chunk data
}

// EncodeChunkedPayload serializes a chunked payload.
func EncodeChunkedPayload(cp *ChunkedPayload) []byte {
	// Format: [4 bytes total_size][2 bytes total_chunks][2 bytes chunk_index][1 byte compressed][data]
	buf := make([]byte, 9+len(cp.Data))
	binary.BigEndian.PutUint32(buf[0:4], cp.TotalSize)
	binary.BigEndian.PutUint16(buf[4:6], cp.TotalChunks)
	binary.BigEndian.PutUint16(buf[6:8], cp.ChunkIndex)
	if cp.Compressed {
		buf[8] = 1
	}
	copy(buf[9:], cp.Data)
	return buf
}

// DecodeChunkedPayload deserializes a chunked payload.
func DecodeChunkedPayload(data []byte) (*ChunkedPayload, error) {
	if len(data) < 9 {
		return nil, fmt.Errorf("chunked payload too short")
	}

	cp := &ChunkedPayload{
		TotalSize:   binary.BigEndian.Uint32(data[0:4]),
		TotalChunks: binary.BigEndian.Uint16(data[4:6]),
		ChunkIndex:  binary.BigEndian.Uint16(data[6:8]),
		Compressed:  data[8] == 1,
		Data:        make([]byte, len(data)-9),
	}
	copy(cp.Data, data[9:])
	return cp, nil
}

// SplitIntoChunks splits data into chunks, optionally compressing it.
func SplitIntoChunks(data []byte, compress bool) ([]*ChunkedPayload, error) {
	var processedData []byte
	compressed := false

	if compress && len(data) > ChunkSize {
		// Try to compress
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write(data); err != nil {
			return nil, fmt.Errorf("compression failed: %w", err)
		}
		if err := gw.Close(); err != nil {
			return nil, fmt.Errorf("compression close failed: %w", err)
		}

		// Only use compression if it actually helps
		if buf.Len() < len(data) {
			processedData = buf.Bytes()
			compressed = true
		} else {
			processedData = data
		}
	} else {
		processedData = data
	}

	totalSize := uint32(len(data)) // Original uncompressed size
	numChunks := (len(processedData) + ChunkSize - 1) / ChunkSize
	if numChunks == 0 {
		numChunks = 1
	}

	chunks := make([]*ChunkedPayload, numChunks)
	for i := 0; i < numChunks; i++ {
		start := i * ChunkSize
		end := start + ChunkSize
		if end > len(processedData) {
			end = len(processedData)
		}

		chunks[i] = &ChunkedPayload{
			TotalSize:   totalSize,
			TotalChunks: uint16(numChunks),
			ChunkIndex:  uint16(i),
			Compressed:  compressed,
			Data:        processedData[start:end],
		}
	}

	return chunks, nil
}

// ReassembleChunks reassembles chunks back into the original data.
func ReassembleChunks(chunks []*ChunkedPayload) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks provided")
	}

	// Sort chunks by index (they should already be sorted, but just in case)
	totalChunks := int(chunks[0].TotalChunks)
	if len(chunks) != totalChunks {
		return nil, fmt.Errorf("expected %d chunks, got %d", totalChunks, len(chunks))
	}

	// Concatenate chunk data
	var buf bytes.Buffer
	for i := 0; i < totalChunks; i++ {
		found := false
		for _, chunk := range chunks {
			if int(chunk.ChunkIndex) == i {
				buf.Write(chunk.Data)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("missing chunk %d", i)
		}
	}

	data := buf.Bytes()

	// Decompress if needed
	if chunks[0].Compressed {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decompression failed: %w", err)
		}
		defer gr.Close()

		decompressed, err := io.ReadAll(gr)
		if err != nil {
			return nil, fmt.Errorf("decompression read failed: %w", err)
		}
		data = decompressed
	}

	return data, nil
}

// EncodeBase64 encodes data to base64.
func EncodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64 decodes base64 data.
func DecodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
