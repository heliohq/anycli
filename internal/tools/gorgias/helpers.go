package gorgias

import "strconv"

// parseID converts a string flag value to an integer resource id, returning a
// usageError (exit 2) when it is not a valid integer.
func parseID(flag, raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &usageError{msg: "gorgias: --" + flag + " must be an integer id, got " + strconv.Quote(raw)}
	}
	return n, nil
}

// boolString renders a bool as a lowercase query value ("true"/"false").
func boolString(b bool) string {
	return strconv.FormatBool(b)
}

// messageParams carries the fields shared by ticket-create's initial message and
// message-create's reply. Gorgias requires channel + via on every message, plus
// a source object routing addresses for the email/phone/sms channels.
type messageParams struct {
	channel     string
	via         string
	body        string
	fromAgent   bool
	senderEmail string
	sourceFrom  string
	sourceTo    []string
}

// buildMessage assembles a Gorgias ticket-message object. channel and via are
// always set (via derived from the channel when not explicit) since Gorgias
// rejects a message that omits them; sender and source are included only when
// their inputs are present.
func buildMessage(p messageParams) map[string]any {
	msg := map[string]any{
		"channel":    p.channel,
		"via":        resolveVia(p.via, p.channel),
		"from_agent": p.fromAgent,
		"body_text":  p.body,
	}
	if p.senderEmail != "" {
		msg["sender"] = map[string]any{"email": p.senderEmail}
	}
	if src := buildSource(p.sourceFrom, p.sourceTo); src != nil {
		msg["source"] = src
	}
	return msg
}

// resolveVia returns the explicit via when set, otherwise derives it from the
// channel. Gorgias documents only api|email|internal-note as via values, so the
// email and internal-note channels map to themselves and everything else
// (api/phone/sms/…) falls back to api.
func resolveVia(via, channel string) string {
	if via != "" {
		return via
	}
	switch channel {
	case "email", "internal-note":
		return channel
	default:
		return "api"
	}
}

// buildSource assembles the source routing object required by email/phone/sms
// messages. It returns nil when neither a from address nor any to address is
// set, so api-channel messages omit source entirely.
func buildSource(from string, to []string) map[string]any {
	if from == "" && len(to) == 0 {
		return nil
	}
	src := map[string]any{}
	if from != "" {
		src["from"] = map[string]any{"address": from}
	}
	if len(to) > 0 {
		addrs := make([]any, 0, len(to))
		for _, a := range to {
			addrs = append(addrs, map[string]any{"address": a})
		}
		src["to"] = addrs
	}
	return src
}
