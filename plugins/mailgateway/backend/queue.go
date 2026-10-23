package mailgateway

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// queueMessage is one Postfix queue entry shown in the dashboard.
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

// parsePostqueueJSON reads the JSON lines from `postqueue -j`.
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
		msg := queueMessage{
			ID:         item.QueueID,
			Sender:     item.Sender,
			Status:     item.QueueName,
			Size:       item.MessageSize,
			Recipients: []string{},
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
	return messages, nil
}

// parseMailq reads the classic `mailq` table.
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
			id := fields[0]
			if strings.HasSuffix(id, "*") || strings.HasSuffix(id, "!") {
				status = "active"
				id = id[:len(id)-1]
			}
			size, _ := strconv.ParseInt(fields[1], 10, 64)
			current = &queueMessage{
				ID:         id,
				Size:       size,
				Sender:     fields[len(fields)-1],
				Arrival:    strings.Join(fields[2:len(fields)-1], " "),
				Status:     status,
				Recipients: []string{},
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
	return messages
}
