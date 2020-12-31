package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/ilikeorangutans/jarvis/pkg/bot"
	"github.com/ilikeorangutans/jarvis/pkg/jarvis"
	"github.com/ilikeorangutans/jarvis/pkg/observability"
	"github.com/ilikeorangutans/jarvis/pkg/predicates"
	"github.com/ilikeorangutans/jarvis/pkg/version"
	"github.com/jmoiron/sqlx"
	"github.com/kelseyhightower/envconfig"
	_ "github.com/mattn/go-sqlite3"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	bolt "go.etcd.io/bbolt"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	FancyLogs     bool `split_words:"true"`
	Debug         bool
	HomeserverURL *url.URL `split_words:"true" required:"true"`
	UserID        string   `split_words:"true" required:"true"`
	Password      string   `split_words:"true" required:"true"`
	DataPath      string   `split_words:"true" required:"true"`
}

func (c Config) DatabasePath() string {
	return ""
}

func (c Config) setupLogging() {
	if c.FancyLogs {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if c.Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
}

func setupDatabase(path string) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("could not open database file: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("could not establish connection to database: %w", err)
	}

	driver, err := sqlite3.WithInstance(db.DB, &sqlite3.Config{})
	if err != nil {
		return nil, fmt.Errorf("could not create migration driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://db/migrations", "sqlite3", driver)
	if err != nil {
		return nil, fmt.Errorf("could not create migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return nil, fmt.Errorf("could not migrate: %w", err)
	}

	return db, nil
}

func main() {
	var config Config
	if err := envconfig.Process("jarvis", &config); err != nil {
		log.Fatal().Err(err).Send()
	}

	config.setupLogging()

	go observability.MakeObservable()

	startTime := time.Now()
	log.
		Info().
		Str("log-level", zerolog.GlobalLevel().String()).
		Str("sha", version.SHA).
		Str("build-time", version.BuildTime).
		Str("go-version", version.GoVersion).
		Str("data-path", config.DataPath).
		Str("homeserverURL", config.HomeserverURL.String()).
		Str("userID", config.UserID).
		Msg("Jarvis starting up")

	filestore, err := bolt.Open(filepath.Join(config.DataPath, "reminder-bot.db"), 0666, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("opening database failed")
	}
	defer filestore.Close()

	botConfig := bot.BotConfiguration{
		Password:      config.Password,
		HomeserverURL: config.HomeserverURL,
		Username:      config.UserID,
	}

	db, err := setupDatabase(filepath.Join(config.DataPath, "jarvis.db"))
	if err != nil {
		log.Fatal().Err(err).Msg("could not set up database")
	}
	defer db.Close()

	boltBotStorage, err := bot.NewBoltBotStorage("jarvis", filestore)
	sqlBotStorage, err := bot.NewSQLBotStorage(db, log.With().Str("component", "sql-bot-storage").Logger())
	if err != nil {
		log.Fatal().Err(err).Msg("could not set up database")
	}

	botStorage := bot.NewMultiplexStorage(boltBotStorage, sqlBotStorage)
	b, err := bot.NewBot(botConfig, botStorage)
	ctx, cancel := context.WithCancel(context.Background())

	// TODO this timezone hack is kinda ugly. We should just translate the times into UTC when we schedule them
	location, _ := time.LoadLocation("EST")
	c := cron.New(cron.WithLocation(location))
	c.Start()

	if err := b.Authenticate(ctx); err != nil {
		log.Fatal().Err(err).Msg("authentication failed")
	}

	jarvis.AddWeatherHandler(ctx, b)
	jarvis.AddSunriseHandlers(ctx, b)
	//jarvis.AddAgendaHandlers(ctx, b)
	reminders, err := jarvis.NewReminders(ctx, b, c, db)
	if err != nil {
		log.Fatal().Err(err).Msg("creating reminders")
	}
	reminders.Start(ctx)
	jarvis.AddReminderHandlers(ctx, b, reminders)
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			client.JoinRoomByID(evt.RoomID)
			client.SendText(evt.RoomID, "👋")
			return nil
		},
		predicates.InvitedToRoom(),
	)
	b.On(
		func(ctx context.Context, client bot.MatrixClient, source mautrix.EventSource, evt *event.Event) error {
			t, err := time.Parse("2006-01-02T15:04:05-0700", version.BuildTime)
			if err != nil {
				log.Error().Err(err).Send()
			}

			client.SendHTML(
				evt.RoomID,
				fmt.Sprintf("🤖 running since <strong>%s</strong>, sha <code>%s</code>, build time <strong>%s</strong> (<code>%s</code>)", humanize.Time(startTime), version.SHA, humanize.Time(t), version.BuildTime),
			)
			return nil
		},
		predicates.All(
			predicates.MessageMatching(regexp.MustCompile(`(?i)\A\s*status`)),
			predicates.AtUser(id.UserID(config.UserID)),
		),
	)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		for {
			select {
			case <-signals:
				log.Info().Msg("received interrupt signal")
				cancel()
			}
		}
	}()

	err = b.Run(ctx)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
}

func agenda(ctx context.Context, client bot.MatrixClient) func() {
	return func() {
		log.Info().Msg("agenda")

		forecast, err := jarvis.WeatherForecast(ctx, "on-143", jarvis.FormatFeed)
		if err != nil {
			log.Error().Err(err).Msg("could not get weather forecast")
		}
		roomID := id.RoomID("!xpfpdJfdQocOPCHqsc:matrix.ilikeorangutans.me")
		client.SendText(roomID, "Weather for today: ")
		client.SendText(roomID, forecast)
	}

}
