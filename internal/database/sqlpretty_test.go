package database

import "testing"

func TestPrettySQL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "select co join / where / order by",
			in:   `SELECT "conversations"."id","conversations"."name" FROM "conversations" JOIN conversation_members cm ON cm.conversation_id = conversations.id AND cm.left_at IS NULL WHERE cm.user_id = '0b3f' AND (COALESCE(conversations.last_message_at, conversations.created_at), conversations.id) < ('2026-01-01','9f1') ORDER BY COALESCE(conversations.last_message_at, conversations.created_at) DESC, conversations.id DESC LIMIT 11`,
			want: `SELECT
    "conversations"."id",
    "conversations"."name"
FROM "conversations"
JOIN conversation_members cm
  ON cm.conversation_id = conversations.id
  AND cm.left_at IS NULL
WHERE cm.user_id = '0b3f'
  AND (COALESCE(conversations.last_message_at, conversations.created_at), conversations.id) < ('2026-01-01', '9f1')
ORDER BY COALESCE(conversations.last_message_at, conversations.created_at) DESC, conversations.id DESC
LIMIT 11`,
		},
		{
			name: "insert on conflict",
			in:   `INSERT INTO "messages" ("id","body") VALUES ('a','hi, there (x)') ON CONFLICT DO NOTHING RETURNING "id"`,
			want: `INSERT INTO "messages" ("id", "body")
VALUES ('a', 'hi, there (x)')
ON CONFLICT DO NOTHING
RETURNING "id"`,
		},
		{
			name: "update nhieu cot",
			in:   `UPDATE "tasks" SET "status"='done',"updated_at"='2026-01-01' WHERE id = 'x' AND status IN ('open','wip')`,
			want: `UPDATE "tasks"
SET "status" = 'done',
    "updated_at" = '2026-01-01'
WHERE id = 'x'
  AND status IN ('open', 'wip')`,
		},
		{
			name: "giu nguyen keyword trong string literal",
			in:   `SELECT * FROM messages WHERE body = 'select from where and'`,
			want: `SELECT *
FROM messages
WHERE body = 'select from where and'`,
		},
		{
			name: "placeholder $1 va select 1 cot",
			in:   `SELECT count(*) FROM conversations WHERE id = $1`,
			want: `SELECT COUNT(*)
FROM conversations
WHERE id = $1`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrettySQL(tc.in); got != tc.want {
				t.Errorf("PrettySQL mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tc.want)
			}
		})
	}
}

func TestPrettySQLEmpty(t *testing.T) {
	if got := PrettySQL("   "); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}
