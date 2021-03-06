package bot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type MatrixClient interface {
	JoinRoomByID(id.RoomID)
	SendText(id.RoomID, string)
	SendHTML(id.RoomID, string)
	SendNotice(id.RoomID, string)
	SetPresence(event.Presence)
	SendReaction(roomID id.RoomID, eventID id.EventID, reaction string)
}

func NewAsyncMatrixClient(client *mautrix.Client) *AsyncMatrixClient {
	return &AsyncMatrixClient{
		client: client,
		queue:  make(chan func(context.Context) error, 100),
		logger: log.With().Str("component", "AsyncMatrixClient").Logger(),
	}
}

type AsyncMatrixClient struct {
	client *mautrix.Client
	queue  chan func(context.Context) error
	logger zerolog.Logger
}

func (a *AsyncMatrixClient) Start(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				// TODO persist queue?
				return

			case f := <-a.queue:
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				err := f(ctx)
				if errors.Is(err, mautrix.MLimitExceeded) {
					log.Warn().Msg("limit exceeded, sleeping")
					time.Sleep(2 * time.Second)
					continue
				} else if err != nil {
					log.Error().Err(err).Msg("handling queue")
				}
			}
		}
	}()
	return nil
}

func (a *AsyncMatrixClient) SendNotice(roomID id.RoomID, message string) {
	a.logger.Debug().Str("message", message).Msg("SendNotice")
	a.queue <- func(ctx context.Context) error {
		_, err := a.client.SendNotice(roomID, message)
		return err
	}
}

func (a *AsyncMatrixClient) SendHTML(roomID id.RoomID, message string) {
	a.logger.Debug().Str("message", message).Msg("SendText")
	a.queue <- func(ctx context.Context) error {
		_, err := a.client.SendMessageEvent(roomID, event.EventMessage, event.MessageEventContent{
			MsgType:       event.MsgText,
			FormattedBody: message,
			Format:        event.FormatHTML,
		})
		return err
	}
}
func (a *AsyncMatrixClient) SendText(roomID id.RoomID, message string) {
	a.logger.Debug().Str("message", message).Msg("SendText")
	a.queue <- func(ctx context.Context) error {
		_, err := a.client.SendText(roomID, message)
		return err
	}
}

func (a *AsyncMatrixClient) JoinRoomByID(roomID id.RoomID) {
	a.logger.Debug().Msg("JoinRoomByID")
	a.queue <- func(ctx context.Context) error {
		_, err := a.client.JoinRoomByID(roomID)
		return err
	}
}

func (a *AsyncMatrixClient) SendReaction(roomID id.RoomID, eventID id.EventID, reaction string) {
	a.logger.Debug().Str("eventID", eventID.String()).Str("reaction", reaction).Msg("SendReaction")
	a.queue <- func(ctx context.Context) error {
		x, err := a.client.SendReaction(roomID, eventID, reaction)
		if err != nil {
			a.logger.Error().Err(err).Msg("SendReaction err")
		}
		a.logger.Debug().Msgf("SendReaction resp event id %v", x)
		return err
	}
}

func (a *AsyncMatrixClient) SetPresence(presence event.Presence) {
	panic("not implemented") // TODO: Implement
}

type EventHandler func(context.Context, MatrixClient, mautrix.EventSource, *event.Event) error

type BotConfiguration struct {
	Username      string
	Password      string
	HomeserverURL *url.URL
}

func NewBot(config BotConfiguration, storage BotStorage) (*Bot, error) {
	logger := log.With().Str("component", "bot").Logger()
	client, err := mautrix.NewClient(config.HomeserverURL.String(), "", "")

	if err != nil {
		return nil, fmt.Errorf("could not create client: %w", err)
	}
	client.Store = storage
	return &Bot{
		client:  client,
		config:  config,
		logger:  logger,
		storage: storage,
		matrix:  NewAsyncMatrixClient(client),
	}, nil
}

type Handler struct {
	Func       EventHandler
	Predicates []predicates.EventPredicate
}

type Bot struct {
	client   *mautrix.Client
	config   BotConfiguration
	logger   zerolog.Logger
	storage  BotStorage
	handlers []Handler
	matrix   *AsyncMatrixClient
	UserID   id.UserID
}

func (b *Bot) Client() MatrixClient {
	return b.matrix
}

func (b *Bot) Authenticate(ctx context.Context) error {
	deviceID, err := b.storage.LoadDeviceID()
	if err != nil {
		return fmt.Errorf("could not load device id: %w", err)
	}

	loginResp, err := b.client.Login(&mautrix.ReqLogin{
		Type:             mautrix.AuthTypePassword,
		Identifier:       mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: b.config.Username},
		Password:         b.config.Password,
		StoreCredentials: true,
		DeviceID:         deviceID,
	})
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	b.UserID = loginResp.UserID

	if loginResp.DeviceID != deviceID {
		deviceID = loginResp.DeviceID
		if err := b.storage.StoreDeviceID(deviceID); err != nil {
			return fmt.Errorf("error storing device id: %w", err)
		}
	}

	b.logger.Info().Str("device-id", loginResp.DeviceID.String()).Str("user-id", loginResp.UserID.String()).Msg("login successful")

	return nil
}

func (b *Bot) Run(ctx context.Context) error {
	// TODO do we need a separate cancel context?

	if err := b.respectLimits(b.client.SetPresence(event.PresenceOnline)); err != nil {
		return fmt.Errorf("setting presence failed: %w", err)
	}

	syncer := b.client.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEvent(func(source mautrix.EventSource, evt *event.Event) {
		// TODO ignore events that happened _before_ we joined the room
		ignoredTypes := []event.Type{event.EphemeralEventReceipt, event.EphemeralEventPresence, event.EphemeralEventTyping}
		for _, t := range ignoredTypes {
			if evt.Type == t {
				return
			}
		}

		if evt.Sender == b.UserID {
			return
		}

		log.Info().Str("source", source.String()).Str("sender", evt.Sender.String()).Str("type", evt.Type.Type).Msg("event")

		for _, handler := range b.handlers {
			canHandle := true
			for _, p := range handler.Predicates {
				if !p(source, evt) {
					canHandle = false
					break
				}
			}
			if !canHandle {
				continue
			}

			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()

			err := handler.Func(ctx, b.matrix, source, evt)
			if err != nil {
				log.Error().Err(err).Msgf("handler failed")
			}
		}

		b.client.MarkRead(evt.RoomID, evt.ID)
	})

	b.matrix.Start(ctx)
	go func() {
		b.logger.Info().Msg("beginning sync")
		err := b.client.Sync()
		if err != nil {
			log.Error().Err(err).Msg("sync failed")
		}
	}()

	for {
		select {
		case <-ctx.Done():
			b.logger.Info().Msg("shutting down")
			b.client.StopSync()
			if err := b.respectLimits(b.client.SetPresence(event.PresenceOffline)); err != nil {
				return fmt.Errorf("setting presence failed: %w", err)
			}
			return nil
		}
	}

	return nil
}

func (b *Bot) On(handler EventHandler, predicates ...predicates.EventPredicate) {
	b.handlers = append(b.handlers, Handler{
		Func:       handler,
		Predicates: predicates,
	})
}

func (b *Bot) respectLimits(err error) error {
	if errors.Is(err, mautrix.MLimitExceeded) {
		b.logger.Warn().Err(err).Msg("request exceeded limit")
		return nil
	}

	return err
}
