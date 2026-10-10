package pcapdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PayloadArtifact points to an atomically published complete binary/JSON file.
// File output complements bounded previews without filling a tool's JSON reply.
type PayloadArtifact struct {
	DatasetID string `json:"dataset_id"`
	RecordID  uint   `json:"record_id"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

func (d *Database) ExportPacket(id uint, output string, options ...QueryOption) (*PayloadArtifact, error) {
	return d.exportPayload(id, "packet", output, options, func(ctx context.Context, w io.Writer) error {
		data, err := d.ReadPacket(id, QueryContext(ctx))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
}
func (d *Database) ExportProtocol(id uint, output string, options ...QueryOption) (*PayloadArtifact, error) {
	return d.exportPayload(id, "protocol", output, options, func(ctx context.Context, w io.Writer) error {
		data, err := d.ReadProtocol(id, QueryContext(ctx))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
}
func (d *Database) ExportProtocolFields(id uint, output string, options ...QueryOption) (*PayloadArtifact, error) {
	return d.exportPayload(id, "protocol_fields", output, options, func(ctx context.Context, w io.Writer) error {
		message, err := d.ProtocolDetails(id, QueryContext(ctx))
		if err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(ProtocolDetail{ID: id, Fields: message.Fields, Session: message.Session})
	})
}
func (d *Database) ExportStream(id uint, output string, options ...QueryOption) (*PayloadArtifact, error) {
	return d.exportPayload(id, "stream", output, options, func(ctx context.Context, w io.Writer) error {
		var after int64
		for {
			page, err := d.ReadStream(id, QueryContext(ctx), QueryAfter(after), QueryMaxBytes(1<<20))
			if err != nil {
				return err
			}
			for _, chunk := range page.Chunks {
				if err = ctx.Err(); err != nil {
					return err
				}
				if _, err = w.Write(chunk.Data); err != nil {
					return err
				}
			}
			if !page.HasMore {
				return nil
			}
			after = page.NextAfter
		}
	})
}
func (d *Database) exportPayload(id uint, kind, output string, options []QueryOption, write func(context.Context, io.Writer) error) (*PayloadArtifact, error) {
	c, err := parseDetailQuery(options)
	if err != nil {
		return nil, err
	}
	m := d.manager
	if err = m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	ctx, cancel := c.operationContext(m)
	defer cancel()
	// Derived content cannot be rebuilt between export pages. Multiple exports
	// and normal readers can run concurrently under the existing shared OS lock.
	lock, err := acquireReadFileLock(ctx, datasetLockPath(&PCAPFileDBMetadata{DatabasePath: d.path}), true)
	if err != nil {
		return nil, err
	}
	defer releaseFileLock(lock)
	path, err := filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	if _, err = os.Lstat(path); err == nil {
		return nil, fmt.Errorf("pcapdb: output already exists: %w", os.ErrExist)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".pcapdb-payload-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(temp.Name())
	hash := sha256.New()
	err = write(ctx, io.MultiWriter(temp, hash))
	var bytes int64
	if info, statErr := temp.Stat(); statErr != nil {
		err = errors.Join(err, statErr)
	} else {
		bytes = info.Size()
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = temp.Sync()
	}
	err = errors.Join(err, temp.Close())
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = os.Link(temp.Name(), path); err != nil {
		return nil, err
	}
	artifact := &PayloadArtifact{DatasetID: d.ID, RecordID: id, Kind: kind, Path: path, Bytes: bytes, SHA256: hex.EncodeToString(hash.Sum(nil))}
	return artifact, syncDirectory(filepath.Dir(path))
}
