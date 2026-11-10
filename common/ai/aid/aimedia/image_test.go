package aimedia

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 7, 5))
	img.Set(2, 3, color.RGBA{R: 200, G: 80, B: 20, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestImageValidationAndOwnership(t *testing.T) {
	ctx := context.Background()
	raw := testPNG(t)
	original := bytes.Clone(raw)
	img, err := FromBytes(ctx, raw, "image/png")
	require.NoError(t, err)
	require.Equal(t, 7, img.Width)
	require.Equal(t, 5, img.Height)
	raw[0] = 0
	owned, err := img.Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, original, owned)
	owned[0] = 0
	again, err := img.Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, original, again)

	encoded := base64.StdEncoding.EncodeToString(original)
	fromBase64, err := FromBase64(ctx, encoded, "image/png")
	require.NoError(t, err)
	require.Equal(t, img.ID, fromBase64.ID)
	serialized, err := json.Marshal(img)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), encoded)
	require.NotContains(t, fmt.Sprintf("%#v", img), encoded)
	require.NotContains(t, string(serialized), `"data"`)
}

func TestImageRejectsMalformedMismatchedAndOversizedInputs(t *testing.T) {
	ctx := context.Background()
	raw := testPNG(t)
	_, err := FromBytes(ctx, raw, "image/jpeg")
	require.ErrorContains(t, err, "does not match")
	_, err = FromBytes(ctx, []byte("not an image"), "image/png")
	require.Error(t, err)
	_, err = FromBytes(ctx, raw[:len(raw)-10], "image/png")
	require.ErrorContains(t, err, "invalid image data")
	_, err = FromBase64(ctx, "secret-invalid-payload!", "image/png")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-invalid-payload")
	_, err = FromBase64(ctx, strings.Repeat("A", base64.StdEncoding.EncodedLen(MaxImageBytes)+1), "image/png")
	require.ErrorContains(t, err, "limit")
	_, err = FromBytes(ctx, make([]byte, MaxImageBytes+1), "image/png")
	require.ErrorContains(t, err, "bytes")

	// Advertise a huge PNG in an otherwise tiny buffer. Reject its dimensions
	// before attempting the full pixel allocation.
	huge := bytes.Clone(raw)
	binary.BigEndian.PutUint32(huge[16:20], 100_000)
	binary.BigEndian.PutUint32(huge[20:24], 100_000)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	_, err = FromBytes(ctx, huge, "image/png")
	require.ErrorContains(t, err, "pixels")
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = FromBytes(ctx, raw, "image/png")
	require.ErrorIs(t, err, context.Canceled)
}

func TestStoreReferencesRestoreAndVerifyOriginalContent(t *testing.T) {
	ctx := context.Background()
	raw := testPNG(t)
	img, err := FromBytes(ctx, raw, "image/png")
	require.NoError(t, err)
	img.Source = Source{Kind: "tool", ToolCallID: "first", ObservedAt: time.Now()}
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	ref, err := store.Save(ctx, img)
	require.NoError(t, err)
	require.Empty(t, img.Path, "saving must not mutate a caller-owned image")
	require.Nil(t, ref.data)
	serialized, err := json.Marshal(ref)
	require.NoError(t, err)
	var restored Image
	require.NoError(t, json.Unmarshal(serialized, &restored))
	data, err := restored.Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, raw, data)

	img.Source.ToolCallID = "second"
	second, err := store.Save(ctx, img)
	require.NoError(t, err)
	require.Equal(t, ref.Path, second.Path)
	require.Equal(t, "first", ref.Source.ToolCallID)
	require.Equal(t, "second", second.Source.ToolCallID)
	files, err := os.ReadDir(filepath.Dir(ref.Path))
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NoError(t, os.WriteFile(ref.Path, []byte("changed"), 0o600))
	_, err = restored.Bytes(ctx)
	require.ErrorContains(t, err, "does not match")
	_, err = store.Save(ctx, img)
	require.ErrorContains(t, err, "damaged")
	require.NoError(t, os.Remove(ref.Path))
	_, err = restored.Bytes(ctx)
	require.Error(t, err, "a missing resource must not be treated as visible")
}

func TestStoreConcurrentCopiesAreAtomicAndCancellationLeavesNoFile(t *testing.T) {
	ctx := context.Background()
	img, err := FromBytes(ctx, testPNG(t), "image/png")
	require.NoError(t, err)
	root := t.TempDir()
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	for n := 0; n < 10; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store, err := NewStore(root)
			if err == nil {
				_, err = store.Save(ctx, img)
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	files, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, files, 1)
	otherRoot := filepath.Join(t.TempDir(), "not-created")
	store, err := NewStore(otherRoot)
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.Save(canceled, img)
	require.ErrorIs(t, err, context.Canceled)
	require.NoDirExists(t, otherRoot)
	_, err = FromFile(ctx, root)
	require.ErrorContains(t, err, "regular file")
}

func TestStoreRetentionLimitDoesNotDiscardExistingResources(t *testing.T) {
	root := t.TempDir()
	large := filepath.Join(root, "retained.png")
	file, err := os.Create(large)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(MaxStoreBytes))
	require.NoError(t, file.Close())
	store, err := NewStore(root)
	require.NoError(t, err)
	img, err := FromBytes(context.Background(), testPNG(t), "image/png")
	require.NoError(t, err)
	_, err = store.Save(context.Background(), img)
	require.ErrorContains(t, err, "retention limit")
	info, err := os.Stat(large)
	require.NoError(t, err)
	require.Equal(t, MaxStoreBytes, info.Size())
}
