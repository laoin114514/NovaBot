package filter

import (
	"encoding/json"
	"testing"

	nova "github.com/laoin114514/NovaBot"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestFilter(t *testing.T) {
	e := map[string]interface{}{
		"post_type": "notice",
		"user_id":   "notice",
	}
	b, _ := json.Marshal(e)
	rawEvent := gjson.ParseBytes(b)
	event := &nova.Event{
		RawEvent: rawEvent,
	}
	result := New(
		func(ctx *nova.Ctx) gjson.Result {
			return ctx.Event.RawEvent
		},
		NewField("post_type").Any(
			Equal("notice"),
			Not(
				In("message"),
			),
		),
		NewField("user_id").All(
			NotEqual("abs"),
		),
	)(&nova.Ctx{Event: event})
	assert.Equal(t, true, result)
}
