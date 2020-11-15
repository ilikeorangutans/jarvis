package main

import (
	"github.com/rs/zerolog/log"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

func main() {
	homeserverURL := "https://matrix.ilikeorangutans.me"
	userID := "test-bot"
	password := "secret"
	deviceID := "KCPNDLBQUO"

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
}
