package domain

import (
	"sync/atomic"
	"time"
)

// Vi sao can "het han": cot users.presence duoc ghi khi WebSocket mo/dong.
// Neu process bi kill (restart, crash, OOM) thi hook dong ket noi khong chay,
// cot do mac ket o "online" mai mai — FE thay nguoi da tat may van sang den.
//
// Cach chong: moi instance 30 giay mot lan cham vao last_seen_at cua nhung
// nguoi dang noi VAO INSTANCE DO (presence.Service.Run). Luc doc, ai
// presence="online" ma last_seen_at qua cu thi coi la offline.
//
// Nho vay dung ca khi chay nhieu pod: pod nao co ket noi thi pod do lam tuoi,
// khong can pod nao biet pod khac.
//
// 90s = 3 lan chu ky 30s: mat 1-2 nhip vi mang/GC thi chua bi coi la offline.
const (
	DefaultPresenceHeartbeat = 30 * time.Second
	DefaultPresenceTTL       = 90 * time.Second
)

// presenceTTL doc rat nhieu (moi lan dung view) nhung chi ghi 1 lan luc wiring,
// nen dung atomic cho gon thay vi mutex.
var presenceTTL atomic.Int64

func init() { presenceTTL.Store(int64(DefaultPresenceTTL)) }

// SetPresenceTTL chi goi luc khoi dong (app.New), truoc khi phuc vu request.
// ttl <= 0 => tat viec coi la het han (luon tin cot presence).
func SetPresenceTTL(ttl time.Duration) { presenceTTL.Store(int64(ttl)) }

// PresenceTTL tra nguong dang dung.
func PresenceTTL() time.Duration { return time.Duration(presenceTTL.Load()) }

// LivePresence la trang thai NEN hien cho nguoi dung — dung cai nay thay cho
// u.Presence o moi cho tra ra ngoai (REST, WebSocket, Bot API).
//
//	online + last_seen_at con moi  -> online
//	online + last_seen_at qua cu   -> offline   (instance giu ket noi da chet)
//	offline / off_duty             -> giu nguyen
//
// off_duty KHONG bi het han: do CMS dat theo lich nghi, khong phu thuoc ket noi.
func (u *User) LivePresence() Presence {
	if u == nil {
		return PresenceOffline
	}
	if u.Presence != PresenceOnline {
		return u.Presence
	}
	ttl := PresenceTTL()
	if ttl <= 0 {
		return PresenceOnline
	}
	if u.LastSeenAt == nil || time.Since(*u.LastSeenAt) > ttl {
		return PresenceOffline
	}
	return PresenceOnline
}

// IsOnline: tien cho view, khoi so chuoi.
func (u *User) IsOnline() bool { return u.LivePresence() == PresenceOnline }
