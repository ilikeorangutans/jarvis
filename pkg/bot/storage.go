package bot

import (
	"fmt"

	"github.com/rs/zerolog/log"
	bolt "go.etcd.io/bbolt"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

type BotStorage interface {
	mautrix.Storer
	LoadDeviceID() (id.DeviceID, error)
	StoreDeviceID(id.DeviceID) error
}

func NewBoltBotStorage(name string, db *bolt.DB) (*BoltBotStorage, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("could not create bot storage bucket: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.Bucket([]byte(name)).CreateBucketIfNotExists([]byte("matrix"))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("could not create bot storage bucket: %w", err)
	}

	return &BoltBotStorage{
		name: name,
		db:   db,
	}, nil
}

type BoltBotStorage struct {
	name string
	db   *bolt.DB
}

func (b *BoltBotStorage) SaveFilterID(userID id.UserID, filterID string) {
	log.Debug().Str("filter-id", filterID).Msg("SaveFilterID")
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := b.matrixBucket(tx)
		return bucket.Put([]byte("filter"), []byte(filterID))
	})
	if err != nil {
		log.Error().Err(err).Msg("SaveFilterID")
	}
}

func (b *BoltBotStorage) LoadFilterID(userID id.UserID) string {
	log.Debug().Msg("LoadFilterID")
	filterID := ""

	b.db.View(func(tx *bolt.Tx) error {
		filterID = string(b.matrixBucket(tx).Get([]byte("filter")))
		return nil
	})

	return filterID
}

func (b *BoltBotStorage) SaveNextBatch(userID id.UserID, nextBatchToken string) {
	log.Debug().Str("next-batch-token", nextBatchToken).Msg("SaveNextBatch")
	err := b.db.Update(func(tx *bolt.Tx) error {
		return b.matrixBucket(tx).Put([]byte("batch"), []byte(nextBatchToken))
	})
	if err != nil {
		log.Error().Err(err).Msg("SaveNextBatch")
	}
}

func (b *BoltBotStorage) LoadNextBatch(userID id.UserID) string {
	log.Debug().Msg("LoadNextBatch")
	result := ""
	err := b.db.View(func(tx *bolt.Tx) error {
		result = string(b.matrixBucket(tx).Get([]byte("batch")))
		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("LoadNextBatch")
	}

	return result
}

func (b *BoltBotStorage) SaveRoom(room *mautrix.Room) {
	panic("not implemented") // TODO: Implement
}

func (b *BoltBotStorage) LoadRoom(roomID id.RoomID) *mautrix.Room {
	panic("not implemented") // TODO: Implement
}

func (b *BoltBotStorage) bucket(tx *bolt.Tx) *bolt.Bucket {
	bucket := tx.Bucket([]byte(b.name))
	return bucket
}

func (b *BoltBotStorage) matrixBucket(tx *bolt.Tx) *bolt.Bucket {
	return b.bucket(tx).Bucket([]byte("matrix"))
}

func (b *BoltBotStorage) LoadDeviceID() (id.DeviceID, error) {
	deviceID := ""
	err := b.db.View(func(tx *bolt.Tx) error {
		deviceID = string(b.bucket(tx).Get([]byte("device-id")))
		return nil
	})
	if err != nil {
		return "", err
	}
	return id.DeviceID(deviceID), nil
}

func (b *BoltBotStorage) StoreDeviceID(deviceID id.DeviceID) error {
	err := b.db.Update(func(tx *bolt.Tx) error {
		return b.bucket(tx).Put([]byte("device-id"), []byte(deviceID.String()))
	})
	if err != nil {
		return err
	}
	return nil
}
