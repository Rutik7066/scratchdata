package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scratchdata/scratchdata/config"
	"github.com/scratchdata/scratchdata/models"
	"github.com/scratchdata/scratchdata/pkg/destinations"
	models2 "github.com/scratchdata/scratchdata/pkg/storage/queue/models"
)

type ScratchDataWorker struct {
	Config             config.Workers
	StorageServices    *models.StorageServices
	destinationManager *destinations.DestinationManager
}

func (w *ScratchDataWorker) Start(ctx context.Context, threadId int) {
	log.Debug().Int("thread", threadId).Msg("Starting worker")

	for {
		item, ok := w.StorageServices.Queue.Dequeue()

		if !ok {
			time.Sleep(1 * time.Second)
		} else {
			message, err := w.messageToStruct(item)
			if err != nil {
				log.Error().Err(err).Int("thread", threadId).Bytes("message_bytes", item).Msg("Unable to decode message")
			}

			err = w.processMessage(threadId, message)
			if err != nil {
				log.Error().Err(err).Int("thread", threadId).Interface("message", message).Msg("Unable to process message")
			}
		}

		select {
		case <-ctx.Done():
			log.Debug().Int("thread", threadId).Msg("Stopping worker")
			return
		default:
		}
	}
}

func (w *ScratchDataWorker) processMessage(threadId int, message models2.FileUploadMessage) error {
	destination, err := w.destinationManager.Destination(message.DatabaseID)
	if err != nil {
		return err
	}

	fileIdent := filepath.Base(message.Key)
	fileName := fmt.Sprintf("%d_%s_%s.ndjson", message.DatabaseID, message.Table, fileIdent)
	filePath := filepath.Join(w.Config.DataDirectory, fileName)

	err = w.downloadFile(filePath, message.Key)
	if err != nil {
		return err
	}

	err = destination.CreateEmptyTable(message.Table)
	if err != nil {
		return err
	}

	file, err := os.Open(filePath)

	if err != nil {
		return err
	}

	err = destination.CreateColumns(message.Table, filePath)
	if err != nil {
		return err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	err = destination.InsertFromNDJsonFile(message.Table, filePath)
	if err != nil {
		return err
	}

	err = file.Close()
	if err != nil {
		log.Error().Err(err).Int("thread", threadId).Str("filename", filePath).Msg("Unable to close temp file")
	}

	err = os.Remove(filePath)
	if err != nil {
		log.Error().Err(err).Int("thread", threadId).Str("filename", filePath).Msg("Unable to remove temp file")
	}

	return nil
}

func (w *ScratchDataWorker) messageToStruct(item []byte) (models2.FileUploadMessage, error) {
	message := models2.FileUploadMessage{}
	err := json.Unmarshal(item, &message)
	return message, err
}

func (w *ScratchDataWorker) downloadFile(path string, key string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	err = w.StorageServices.BlobStore.Download(key, file)
	if err != nil {
		return err
	}

	return file.Close()
}

func RunWorkers(ctx context.Context, config config.Workers, storageServices *models.StorageServices, destinationManager *destinations.DestinationManager) {
	err := os.MkdirAll(config.DataDirectory, os.ModePerm)
	if err != nil {
		log.Error().Err(err).Str("directory", config.DataDirectory).Msg("Unable to create folder for workers")
		return
	}

	workers := &ScratchDataWorker{
		Config:             config,
		StorageServices:    storageServices,
		destinationManager: destinationManager,
	}

	log.Debug().Msg("Starting Workers")
	var wg sync.WaitGroup
	i := 0
	for i = 0; i < config.Count; i++ {
		wg.Add(1)
		go func(threadId int) {
			defer wg.Done()
			workers.Start(ctx, threadId)
			log.Print("worker done")
		}(i)
	}
	wg.Wait()

	// Clean up resources and gracefully shut down the web server
}
