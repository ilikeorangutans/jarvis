package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	bolt "go.etcd.io/bbolt"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func NewBotStore(db *bolt.DB) (*BotStore, error) {
	db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists([]byte("bot"))
		return nil
	})
	return &BotStore{
		db: db,
	}, nil
}

type BotStore struct {
	db *bolt.DB
}

func (b *BotStore) SaveFilterID(userID id.UserID, filterID string) {
	log.Info().Str("method", "SaveFilterID").Str("userID", userID.String()).Str("filterID", filterID).Send()
	b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("bot"))
		return bucket.Put([]byte("filter"), []byte(filterID))
	})
}

func (b *BotStore) LoadFilterID(userID id.UserID) string {
	log.Info().Str("method", "LoadFilterID").Str("userID", userID.String()).Send()
	return ""
}

func (b *BotStore) SaveNextBatch(userID id.UserID, nextBatchToken string) {
	log.Info().Str("method", "SaveNextBatch").Str("nextBatchToken", nextBatchToken).Send()
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("bot"))
		return bucket.Put([]byte("batch"), []byte(nextBatchToken))
	})
	if err != nil {
		log.Error().Err(err).Msg("SaveNextBatch")
	}
}

func (b *BotStore) LoadNextBatch(userID id.UserID) string {
	log.Info().Str("method", "LoadNextBatch").Str("userID", userID.String()).Send()
	result := ""
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("bot"))
		result = string(bucket.Get([]byte("batch")))
		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("LoadNextBatch")
	}

	log.Info().Str("method", "LoadNextBatch").Str("result", result).Send()
	return result
}

func (b *BotStore) SaveRoom(room *mautrix.Room) {
	panic("not implemented") // TODO: Implement
}

func (b *BotStore) LoadRoom(roomID id.RoomID) *mautrix.Room {
	panic("not implemented") // TODO: Implement
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	homeserverURL := "https://matrix.ilikeorangutans.me"
	userID := "test-bot"
	password := "secret"

	deviceID := "KCPNDLBQUO"
	lastWatermark := time.Now().Unix() * 1000

	client, err := mautrix.NewClient(homeserverURL, "", "")
	if err != nil {
		log.Fatal().Err(err).Msgf("connecting to homeserver %s failed", homeserverURL)
	}

	loginResp, err := client.Login(&mautrix.ReqLogin{
		Type:             mautrix.AuthTypePassword,
		Identifier:       mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: userID},
		Password:         password,
		StoreCredentials: true,
		DeviceID:         id.DeviceID(deviceID),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("authentication failed")
	}

	log.Info().Str("device-id", loginResp.DeviceID.String()).Str("user-id", loginResp.UserID.String()).Msg("login successful")

	db, err := bolt.Open("my.db", 0666, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("authentication failed")
	}
	defer db.Close()

	store, err := NewBotStore(db)
	if err != nil {
		log.Fatal().Err(err).Msg("creating bot store")
	}
	client.Store = store
	client.SyncPresence = event.PresenceOnline

	joinedRooms := make(map[id.RoomID]struct{})
	if rooms, err := client.JoinedRooms(); err != nil {
		log.Fatal().Err(err).Msg("failed listing joined rooms")
	} else {
		for _, roomID := range rooms.JoinedRooms {
			log.Info().Str("room-id", roomID.String()).Msg("joined room")
			joinedRooms[roomID] = struct{}{}
		}
	}

	// TODO can we just filter out past messages?
	syncer := client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(source mautrix.EventSource, evt *event.Event) {
		log.Info().Int64("ts", evt.Timestamp).Int64("wm", lastWatermark).Msg("message")
		if evt.Timestamp < lastWatermark {
			log.Info().Int64("ts", evt.Timestamp).Int64("wm", lastWatermark).Msg("igoring")
			return
		}
		client.MarkRead(evt.RoomID, evt.ID)
		message := evt.Content.AsMessage()

		log.Info().Str("body", message.Body).Msg("message")
		if !strings.HasPrefix(strings.ToLower(message.Body), "remind me") {
			return
		}

		client.SendText(evt.RoomID, fmt.Sprintf("you want me to remind you about %s", message.Body))
	})
	syncer.OnEventType(event.StateMember, func(source mautrix.EventSource, evt *event.Event) {
		if evt.Timestamp < lastWatermark {
			return
		}
		membership := evt.Content.AsMember()
		switch membership.Membership {
		case event.MembershipInvite:
			if _, ok := joinedRooms[evt.RoomID]; ok {
				return
			}
			log.Info().Str("room-id", evt.RoomID.String()).Bool("invite", true).Str("sender", evt.Sender.String()).Msgf("invite to room")
			if _, err := client.JoinRoomByID(evt.RoomID); err != nil {
				log.Error().Err(err)
				return
			}
			joinedRooms[evt.RoomID] = struct{}{}
			if _, err := client.SendNotice(evt.RoomID, "test-bot has joined this room"); err != nil {
				log.Error().Err(err)
				return
			}

		case event.MembershipLeave:
			delete(joinedRooms, evt.RoomID)
			log.Info().Str("room-id", evt.RoomID.String()).Msg("leaving room")
		default:
			// ignore
		}
	})

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt)

	go func() {
		log.Info().Msg("starting sync")
		client.Sync()
	}()

	<-shutdown
	log.Info().Msg("shutting down")
}
