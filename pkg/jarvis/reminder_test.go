package jarvis

import (
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestReminderFromParts(t *testing.T) {

	data := []struct {
		input string
	}{
		{"at 8am to do foo"},
		{"today at 8am to do foo"},
		{"tomorrow at 8am to do foo"},
		{"every day at 8am to do foo"},
		{"monday at 8am to do foo"},
		{"every weekday at 8am to do foo"},
		{"every saturday at 8am to do foo"},
	}

	for _, d := range data {
		t.Logf(">>> input: %s", d.input)
		parts := timeSpecifierRegex.FindStringSubmatch(d.input)

		reminder, err := ReminderFromParts(parts)
		assert.NilError(t, err)
		t.Logf("reminder: %s", reminder)
		t.Logf("spec: %s", reminder.ToSpec())
	}
}

func TestResolveRelativeDay(t *testing.T) {

	now := time.Date(2021, time.January, 8, 14, 30, 0, 0, time.Local)

	data := []struct {
		day      string
		hour     string
		minute   string
		expected string
	}{
		{"today", "8", "0", strings.ToLower(now.Weekday().String())},
		{"tomorrow", "8", "0", strings.ToLower(now.AddDate(0, 0, 1).Weekday().String())},
		{"monday", "8", "0", "monday"},
		{"weekday", "8", "0", "weekday"},
		{"", "8", "0", "saturday"},
		{"", "14", "31", "friday"},
		{"day", "8", "0", "every day"},
	}

	for i, d := range data {
		r := &Reminder{
			Day:    d.day,
			Hour:   d.hour,
			Minute: d.minute,
		}
		expected := d.expected
		assert.Equal(t, r.ResolveRelativeDay(now), expected, "for %d", i)
	}

}
