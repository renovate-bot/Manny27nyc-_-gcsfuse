// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Provides integration tests for gzip objects in gcsfuse mounts.
package gzip_test

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"testing"

	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/client"
	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/operations"
	"github.com/googlecloudplatform/gcsfuse/v3/tools/integration_tests/util/setup"
)

// Verify that the passed file exists on the GCS test-bucket and in the mounted bucket
// and its size in the mounted directory matches that of the GCS object. Also verify
// that the passed file in the mounted bucket matches the corresponding
// GCS object in content.
// GCS object.
func verifyFileSizeAndFullFileRead(t *testing.T, filename string) {
	mountedFilePath := path.Join(setup.MntDir(), TestBucketPrefixPath, filename)
	gcsObjectPath := path.Join(TestBucketPrefixPath, filename)
	gcsObjectSize, err := client.GetGcsObjectSize(ctx, storageClient, gcsObjectPath)
	if err != nil {
		t.Fatalf("Failed to get size of gcs object %s: %v\n", gcsObjectPath, err)
	}

	fi, err := operations.StatFile(mountedFilePath)
	if err != nil || fi == nil {
		t.Fatalf("Failed to get stat info of mounted file %s: %v\n", mountedFilePath, err)
	}

	if (*fi).Size() != int64(gcsObjectSize) {
		t.Fatalf("Size of file mounted through gcsfuse (%s, %d) doesn't match the size of the file on GCS (%s, %d)",
			mountedFilePath, (*fi).Size(), gcsObjectPath, gcsObjectSize)
	}

	localCopy, err := downloadGzipGcsObjectAsCompressed(t, path.Join(TestBucketPrefixPath, filename))
	if err != nil {
		t.Fatalf("failed to download gcs object (gs:/%s) to local-disk: %v", gcsObjectPath, err)
	}

	defer operations.RemoveFile(localCopy)

	identical, err := operations.AreFilesIdentical(localCopy, mountedFilePath)
	if !identical {
		t.Fatalf("Tempfile (%s, download of GCS object %s) didn't match the Mounted local file (%s): %v", localCopy, gcsObjectPath, mountedFilePath, err)
	}
}

// Verify that the passed file exists on the GCS test-bucket and in the mounted bucket
// and its ranged read returns the same size as the requested read size.
func verifyRangedRead(t *testing.T, filename string) {
	mountedFilePath := path.Join(setup.MntDir(), TestBucketPrefixPath, filename)
	gcsObjectPath := path.Join(TestBucketPrefixPath, filename)
	gcsObjectSize, err := client.GetGcsObjectSize(ctx, storageClient, gcsObjectPath)

	if err != nil {
		t.Fatalf("Failed to get size of gcs object %s: %v\n", gcsObjectPath, err)
	}

	readSize := int64(gcsObjectSize / 10)
	readOffset := int64(readSize / 10)
	f, err := os.Open(mountedFilePath)
	if err != nil || f == nil {
		t.Fatalf("Failed to open local mounted file %s: %v", mountedFilePath, err)
	}

	localCopy, err := downloadGzipGcsObjectAsCompressed(t, path.Join(TestBucketPrefixPath, filename))
	if err != nil {
		t.Fatalf("failed to download gcs object (gs:/%s) to local-disk: %v", gcsObjectPath, err)
	}

	defer operations.RemoveFile(localCopy)

	for _, offsetMultiplier := range []int64{1, 3, 5, 7, 9} {
		buf1, err := operations.ReadChunkFromFile(mountedFilePath, (readSize), offsetMultiplier*(readOffset), os.O_RDONLY)
		if err != nil {
			t.Fatalf("Failed to read mounted file %s: %v", mountedFilePath, err)
		} else if buf1 == nil {
			t.Fatalf("Failed to read mounted file %s: buffer returned as nul", mountedFilePath)
		}

		buf2, err := operations.ReadChunkFromFile(localCopy, (readSize), offsetMultiplier*(readOffset), os.O_RDONLY)
		if err != nil {
			t.Fatalf("Failed to read local file %s: %v", localCopy, err)
		} else if buf2 == nil {
			t.Fatalf("Failed to read local file %s: buffer returned as nul", localCopy)
		}

		if !bytes.Equal(buf1, buf2) {
			t.Fatalf("Read buffer (of size %d from offset %d) of %s doesn't match that of %s", int64(readSize), offsetMultiplier*int64(readOffset), mountedFilePath, localCopy)
		}
	}
}

// downloadGzipGcsObjectAsCompressed downloads given gzipped GCS object (with path without 'gs://') to local disk.
// Fails if the object doesn't exist or permission to read object is not
// available.
// Uses go storage client library to download object. Use of gsutil/gcloud is not
// possible as they both always read back objects with content-encoding: gzip as
// uncompressed/decompressed irrespective of any argument passed.
func downloadGzipGcsObjectAsCompressed(t *testing.T, objPathInBucket string) (tempfile string, err error) {
	gcsObjectSize, err := client.GetGcsObjectSize(ctx, storageClient, objPathInBucket)

	if err != nil {
		return "", fmt.Errorf("failed to get size of gcs object %s: %w", objPathInBucket, err)
	}

	content, err := createContentOfSize(1)
	if err != nil {
		return "", fmt.Errorf("failed to create data: %w", err)
	}
	if tempfile, err = operations.CreateLocalTempFile(content, false); err != nil {
		return "", fmt.Errorf("failed to create tempfile for downloading gcs object: %w", err)
	}
	defer func() {
		if err != nil {
			removeErr := os.Remove(tempfile)
			if removeErr != nil {
				t.Logf("Error removing temporary file %s: %v", tempfile, removeErr)
			}
		}
	}()

	client, err := client.CreateStorageClient(ctx)
	if err != nil || client == nil {
		return "", fmt.Errorf("failed to create storage client: %w", err)
	}
	defer client.Close()

	bktName := setup.TestBucket()
	bkt := client.Bucket(bktName)
	if bkt == nil {
		return "", fmt.Errorf("failed to access bucket %s: %w", bktName, err)
	}

	obj := bkt.Object(objPathInBucket)
	if obj == nil {
		return "", fmt.Errorf("failed to access object %s from bucket %s: %w", objPathInBucket, bktName, err)
	}

	obj = obj.ReadCompressed(true)
	if obj == nil {
		return "", fmt.Errorf("failed to access object %s from bucket %s as compressed: %w", objPathInBucket, bktName, err)
	}

	r, err := obj.NewReader(ctx)
	if r == nil || err != nil {
		return "", fmt.Errorf("failed to read object %s from bucket %s: %w", objPathInBucket, bktName, err)
	}
	defer r.Close()

	gcsObjectData, err := io.ReadAll(r)
	if len(gcsObjectData) < int(gcsObjectSize) || err != nil {
		return "", fmt.Errorf("failed to read object %s from bucket %s (expected read-size: %d, actual read-size: %d): %w", objPathInBucket, bktName, gcsObjectSize, len(gcsObjectData), err)
	}

	err = os.WriteFile(tempfile, gcsObjectData, fs.FileMode(os.O_CREATE|os.O_WRONLY|os.O_TRUNC))
	if err != nil || client == nil {
		return "", fmt.Errorf("failed to write to tempfile %s: %w", tempfile, err)
	}

	return tempfile, nil
}

func TestGzipEncodedTextFileWithNoTransformSizeAndFullFileRead(t *testing.T) {
	verifyFileSizeAndFullFileRead(t, TextContentWithContentEncodingWithNoTransformFilename)
}

func TestGzipEncodedTextFileWithNoTransformRangedRead(t *testing.T) {
	verifyRangedRead(t, TextContentWithContentEncodingWithNoTransformFilename)
}

func TestGzipEncodedTextFileWithoutNoTransformSizeAndFullFileRead(t *testing.T) {
	verifyFileSizeAndFullFileRead(t, TextContentWithContentEncodingWithoutNoTransformFilename)
}

func TestGzipEncodedTextFileWithoutNoTransformRangedRead(t *testing.T) {
	verifyRangedRead(t, TextContentWithContentEncodingWithoutNoTransformFilename)
}

func TestGzipUnencodedGzipFileSizeAndFullFileRead(t *testing.T) {
	verifyFileSizeAndFullFileRead(t, GzipContentWithoutContentEncodingFilename)
}

func TestGzipUnencodedGzipFileRangedRead(t *testing.T) {
	verifyRangedRead(t, GzipContentWithoutContentEncodingFilename)
}

func TestGzipEncodedGzipFileWithNoTransformSizeAndFullFileRead(t *testing.T) {
	verifyFileSizeAndFullFileRead(t, GzipContentWithContentEncodingWithNoTransformFilename)
}

func TestGzipEncodedGzipFileWithNoTransformRangedRead(t *testing.T) {
	verifyRangedRead(t, GzipContentWithContentEncodingWithNoTransformFilename)
}

func TestGzipEncodedGzipFileWithoutNoTransformSizeAndFullFileRead(t *testing.T) {
	verifyFileSizeAndFullFileRead(t, GzipContentWithContentEncodingWithoutNoTransformFilename)
}

func TestGzipEncodedGzipFileWithoutNoTransformRangedRead(t *testing.T) {
	verifyRangedRead(t, GzipContentWithContentEncodingWithoutNoTransformFilename)
}
