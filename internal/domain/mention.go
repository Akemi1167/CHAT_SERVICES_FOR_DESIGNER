package domain

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// MaxMentions: so nguoi toi da duoc nhac trong mot tin (khong tinh @all).
const MaxMentions = 100

// MentionAllKeyword: chuoi FE dung cho "nhac ca nhom".
const MentionAllKeyword = "all"

// MentionKey la khoa dinh danh nguoi duoc nhac @ trong Message.Mentions.
// Uu tien actorId (u_<cmsUserId>) — dung hop dong voi CMS/FE. User chua co
// actorId (token dev, nhom noi bo) thi dung UUID noi bo de van nhac duoc.
func (u *User) MentionKey() string {
	if u == nil {
		return ""
	}
	if u.ActorID != "" {
		return u.ActorID
	}
	return u.ID.String()
}

// BotAdminOf tra danh sach botId ma user la admin (TL) phia CMS.
func (u *User) BotAdminOf() []int64 {
	out := make([]int64, 0)
	for _, v := range u.cmsList("botAdminOf") {
		if id := toInt64(v); id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// CanMentionAll: chi admin / tro ly cua nhom duoc nhac @all. Xet theo thu tu:
//   - role CMS = "admin"
//   - admin bot (botAdminOf) so huu nhom (cms_info.botId)
//   - tro ly nhom (assistantOf co chatId cua nhom)
//   - nhom tao trong Chat: owner / admin cua nhom
//
// member co the nil (chua tra) — khi do bo qua dieu kien cuoi.
func (u *User) CanMentionAll(conv *Conversation, member *ConversationMember) bool {
	if u == nil || conv == nil {
		return false
	}
	if u.Role == "admin" {
		return true
	}
	if botID := conv.BotID(); botID > 0 && slices.Contains(u.BotAdminOf(), botID) {
		return true
	}
	if conv.ExternalID != nil && *conv.ExternalID != "" && slices.Contains(u.AssistantOf(), *conv.ExternalID) {
		return true
	}
	if member != nil {
		// Thanh vien co nguon "assistant" (CMS tinh) cung la tro ly cua nhom.
		if member.Sources.Has("assistant") {
			return true
		}
		if !conv.IsCMS() && (member.Role == MemberOwner || member.Role == MemberAdmin) {
			return true
		}
	}
	return false
}

// NormalizeMentionInput bo khoang trang, dau "@" o dau va chuoi rong; giu thu tu,
// bo trung. Tra them allRequested = true neu co "all" (FE gui nham vao mentions).
func NormalizeMentionInput(in []string) (keys []string, allRequested bool) {
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		k := strings.TrimPrefix(strings.TrimSpace(raw), "@")
		if k == "" {
			continue
		}
		if strings.EqualFold(k, MentionAllKeyword) {
			allRequested = true
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	return keys, allRequested
}

// toInt64 doc so tu gia tri JSON (float64 sau khi doc jsonb, int64 khi tao trong bo nho, ...).
func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimPrefix(t, "b_"), 10, 64)
		return n
	case map[string]any:
		// botAdminOf co the la [{ "botId": 1, ... }]
		if id, ok := t["botId"]; ok {
			return toInt64(id)
		}
		if id, ok := t["id"]; ok {
			return toInt64(id)
		}
	}
	return 0
}
