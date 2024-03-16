package blobstore

import (
	"io"

	"github.com/scratchdata/scratchdata/config"
	"github.com/scratchdata/scratchdata/pkg/storage/blobstore/memory"
	"github.com/scratchdata/scratchdata/pkg/storage/blobstore/s3"
)

type BlobStore interface {
	Upload(path string, r io.ReadSeeker) error
	Download(path string, w io.WriterAt) error
}

func NewBlobStore(conf config.BlobStore) (BlobStore, error) {
	switch conf.Type {
	case "memory":
		return memory.NewStorage(conf.Settings)
	case "s3":
		return s3.NewStorage(conf.Settings)
	}

	return nil, nil
}
