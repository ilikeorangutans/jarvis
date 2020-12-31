package bot

import (
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
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

func NewMultiplexStorage(storers ...BotStorage) BotStorage {
	return &MultiplexStorage{
		storers: storers,
	}
}

type MultiplexStorage struct {
	storers []BotStorage
}

func (m *MultiplexStorage) SaveFilterID(userID id.UserID, filterID string) {
	for _, s := range m.storers {
		s.SaveFilterID(userID, filterID)
	}
}

func (m *MultiplexStorage) LoadFilterID(userID id.UserID) string {
	return m.storers[0].LoadFilterID(userID)
}

func (m *MultiplexStorage) SaveNextBatch(userID id.UserID, nextBatchToken string) {
	for _, s := range m.storers {
		s.SaveNextBatch(userID, nextBatchToken)
	}
}

func (m *MultiplexStorage) LoadNextBatch(userID id.UserID) string {
	return m.storers[0].LoadNextBatch(userID)
}

func (m *MultiplexStorage) SaveRoom(room *mautrix.Room) {
	for _, s := range m.storers {
		s.SaveRoom(room)
	}
}

func (m *MultiplexStorage) LoadRoom(roomID id.RoomID) *mautrix.Room {
	return m.storers[0].LoadRoom(roomID)
}

func (m *MultiplexStorage) LoadDeviceID() (id.DeviceID, error) {
	return m.storers[0].LoadDeviceID()
}

func (m *MultiplexStorage) StoreDeviceID(deviceID id.DeviceID) error {
	for _, s := range m.storers {
		if err := s.StoreDeviceID(deviceID); err != nil {
			return err
		}
	}
	return nil
}

func NewSQLBotStorage(db *sqlx.DB, log zerolog.Logger) (BotStorage, error) {
	return &sqlBotStorage{
		log: log,
		db:  db,
	}, nil
}

type sqlBotStorage struct {
	mautrix.Storer
	log zerolog.Logger
	db  *sqlx.DB
}

func (s *sqlBotStorage) SaveFilterID(userID id.UserID, filterID string) {
	s.log.Debug().Str("method", "SaveFilterID").Stringer("userID", userID).Str("filterID", filterID).Send()
	_, err := sq.
		Insert("bot_batch").
		Columns("user_id", "batch_token").
		Values(userID, filterID).
		Suffix("on conflict (user_id) do update set filter_id = ?", filterID).
		RunWith(s.db).
		Exec()

	if err != nil {
		s.log.Error().Err(err).Msg("could not save filter id")
	}
}

func (s *sqlBotStorage) LoadFilterID(userID id.UserID) string {
	s.log.Debug().Str("method", "LoadFilterID").Stringer("userID", userID).Send()

	var filterID string
	err := sqlx.Get(s.db, &filterID, "select filter_id from bot_filters where user_id = ?", userID)
	if err != nil {
		s.log.Error().Err(err).Msg("could not load filter id")
	}

	return filterID
}

func (s *sqlBotStorage) SaveNextBatch(userID id.UserID, nextBatchToken string) {
	s.log.Debug().Str("method", "SaveNextBatch").Stringer("userID", userID).Str("nextBatchToken", nextBatchToken).Send()

	_, err := sq.
		Insert("bot_batch").
		Columns("user_id", "batch_token").
		Values(userID, nextBatchToken).
		Suffix("on conflict (user_id) do update set batch_token = ?", nextBatchToken).
		RunWith(s.db).
		Exec()
	if err != nil {
		s.log.Error().Err(err).Msg("could not build query")
	}
}

func (s *sqlBotStorage) LoadNextBatch(userID id.UserID) string {
	s.log.Debug().Str("method", "LoadNextBatch").Stringer("userID", userID).Send()

	var batchToken string
	err := sqlx.Get(s.db, &batchToken, "select batch_token from bot_batch where user_id = ?", userID)
	if err != nil {
		s.log.Error().Err(err).Msg("could not load filter id")
	}

	return batchToken
}

func (s *sqlBotStorage) LoadDeviceID() (id.DeviceID, error) {
	s.log.Debug().Str("method", "LoadDeviceID").Send()
	var deviceID id.DeviceID
	err := sqlx.Get(s.db, &deviceID, "select device_id from device_ids order by created_at desc limit 1 ")
	if err != nil {
		s.log.Error().Err(err).Msg("could not load device id")
	}

	return deviceID, nil
}

func (s *sqlBotStorage) StoreDeviceID(deviceID id.DeviceID) error {
	s.log.Debug().Str("method", "StoreDeviceID").Stringer("deviceID", deviceID).Send()
	_, err := sq.
		Insert("device_ids").
		Columns("device_id", "created_at").
		Values(deviceID, time.Now()).
		Suffix("on conflict (device_id) do update set device_id = ?", deviceID).
		RunWith(s.db).
		Exec()
	if err != nil {
		return fmt.Errorf("could not save device id: %w", err)
	}

	return nil
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
