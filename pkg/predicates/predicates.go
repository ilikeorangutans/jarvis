package predicates

import (
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type EventPredicate func(source mautrix.EventSource, evt *event.Event) bool

func All(predicates ...EventPredicate) EventPredicate {
	return func(source mautrix.EventSource, evt *event.Event) bool {
		for _, p := range predicates {
			if !p(source, evt) {
				return false
			}
		}

		return true
	}
}

func MessageMatching(r *regexp.Regexp) EventPredicate {
	return func(source mautrix.EventSource, evt *event.Event) bool {
		if evt.Type != event.EventMessage {
			return false
		}

		msg := evt.Content.AsMessage()

		return r.MatchString(msg.Body)
	}
}

func NotFromUser(userID id.UserID) EventPredicate {
	return func(source mautrix.EventSource, evt *event.Event) bool {
		log.Info().Str("evt.Sender", evt.Sender.String()).Str("userID", userID.String()).Msg("comparing")
		return evt.Sender != userID
	}
}

func AtUser(userID id.UserID) EventPredicate {
	return func(source mautrix.EventSource, evt *event.Event) bool {
		if evt.Type != event.EventMessage {
			return false
		}

		msg := evt.Content.AsMessage()

		user, _, _ := userID.Parse()
		return strings.HasPrefix(strings.TrimSpace(msg.Body), user)
	}
}

func InvitedToRoom() EventPredicate {
	return func(source mautrix.EventSource, evt *event.Event) bool {
		return evt.Type == event.StateMember && evt.Content.AsMember().Membership == event.MembershipInvite
	}
}
