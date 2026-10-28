package mail

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"godeb/backend/core"
)

// Postfix queue IDs are alphanumeric (sometimes with a trailing * / ! in mailq).
var queueIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{6,25}$`)

// queueMessage is one entry from postqueue -j / mailq.
type queueMessage struct {
	ID         string   `json:"id"`
	Sender     string   `json:"sender"`
	Recipients []string `json:"recipients"`
	Arrival    string   `json:"arrival"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason,omitempty"`
	Size       int64    `json:"size"`
}

type postqueueItem struct {
	QueueName   string `json:"queue_name"`
	QueueID     string `json:"queue_id"`
	ArrivalTime int64  `json:"arrival_time"`
	MessageSize int64  `json:"message_size"`
	Sender      string `json:"sender"`
	Recipients  []struct {
		Address     string `json:"address"`
		DelayReason string `json:"delay_reason"`
	} `json:"recipients"`
}

func normalizeQueueID(id string) (string, error) {
	id = strings.TrimSpace(id)
	id = strings.TrimRight(id, "*!")
	if !queueIDPattern.MatchString(id) {
		return "", errors.New("Queue-ID ist ungültig")
	}
	return id, nil
}

func readQueue(ctx context.Context) ([]queueMessage, bool) {
	out, env := core.ReadCommand(ctx, "", "postqueue", "-j")
	if !env.Mocked {
		if msgs, err := parsePostqueueJSON(out); err == nil {
			return msgs, false
		}
	}
	text, textEnv := core.ReadCommand(ctx, "Mail queue is empty\n", "mailq")
	return parseMailq(text), env.Mocked || textEnv.Mocked
}

func parsePostqueueJSON(out string) ([]queueMessage, error) {
	var messages []queueMessage
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var item postqueueItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, err
		}
		id, err := normalizeQueueID(item.QueueID)
		if err != nil {
			continue
		}
		msg := queueMessage{
			ID: id, Sender: item.Sender, Status: item.QueueName,
			Size: item.MessageSize, Recipients: []string{},
		}
		if item.ArrivalTime > 0 {
			msg.Arrival = time.Unix(item.ArrivalTime, 0).UTC().Format(time.RFC3339)
		}
		for _, recipient := range item.Recipients {
			if recipient.Address != "" {
				msg.Recipients = append(msg.Recipients, recipient.Address)
			}
			if msg.Reason == "" && recipient.DelayReason != "" {
				msg.Reason = recipient.DelayReason
			}
		}
		messages = append(messages, msg)
	}
	if messages == nil {
		messages = []queueMessage{}
	}
	return messages, nil
}

func parseMailq(out string) []queueMessage {
	var messages []queueMessage
	var current *queueMessage
	flush := func() {
		if current == nil {
			return
		}
		messages = append(messages, *current)
		current = nil
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "-Queue ID-") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") || trimmed == "Mail queue is empty" {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' && line[0] != '(' {
			flush()
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			status := "deferred"
			raw := fields[0]
			if strings.HasSuffix(raw, "*") || strings.HasSuffix(raw, "!") {
				status = "active"
			}
			id, err := normalizeQueueID(raw)
			if err != nil {
				continue
			}
			size, _ := strconv.ParseInt(fields[1], 10, 64)
			current = &queueMessage{
				ID: id, Size: size, Sender: fields[len(fields)-1],
				Arrival: strings.Join(fields[2:len(fields)-1], " "),
				Status:  status, Recipients: []string{},
			}
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(trimmed, "(") {
			current.Reason = strings.TrimSuffix(strings.TrimPrefix(trimmed, "("), ")")
			continue
		}
		current.Recipients = append(current.Recipients, trimmed)
	}
	flush()
	if messages == nil {
		messages = []queueMessage{}
	}
	return messages
}
