package mail

import (
	"strings"
	"testing"
)

func TestNormalizeQueueID(t *testing.T) {
	ok, err := normalizeQueueID("ABC123*")
	if err != nil || ok != "ABC123" {
		t.Fatalf("%q %v", ok, err)
	}
	if _, err := normalizeQueueID("../etc"); err == nil {
		t.Fatal("path rejected")
	}
	if _, err := normalizeQueueID("short"); err == nil {
		t.Fatal("too short")
	}
	if _, err := normalizeQueueID("id;rm"); err == nil {
		t.Fatal("metachar")
	}
}

func TestParsePostqueueJSON(t *testing.T) {
	line := `{"queue_name":"deferred","queue_id":"4ABCDE1234","arrival_time":1760380000,"message_size":1200,"sender":"from@example.org","recipients":[{"address":"to@example.org","delay_reason":"connect refused"}]}`
	msgs, err := parsePostqueueJSON(line + "\n")
	if err != nil || len(msgs) != 1 {
		t.Fatalf("%v %v", msgs, err)
	}
	m := msgs[0]
	if m.ID != "4ABCDE1234" || m.Sender != "from@example.org" || m.Reason != "connect refused" {
		t.Fatalf("%+v", m)
	}
	if len(m.Recipients) != 1 || m.Recipients[0] != "to@example.org" {
		t.Fatalf("%v", m.Recipients)
	}
}

func TestParseMailq(t *testing.T) {
	out := `-Queue ID-  --Size-- ----Arrival Time---- -Sender/Recipient-------
4ABCDE1234*    1234 Tue Oct 28 10:00:00  from@example.org
                                         to@example.org
                                         (connect refused)

-- 0 Kbytes in 1 Request.
`
	msgs := parseMailq(out)
	if len(msgs) != 1 {
		t.Fatalf("%d", len(msgs))
	}
	if msgs[0].ID != "4ABCDE1234" || msgs[0].Status != "active" {
		t.Fatalf("%+v", msgs[0])
	}
	if !strings.Contains(msgs[0].Reason, "connect refused") {
		t.Fatalf("%q", msgs[0].Reason)
	}
	if len(parseMailq("Mail queue is empty\n")) != 0 {
		t.Fatal("empty")
	}
}
